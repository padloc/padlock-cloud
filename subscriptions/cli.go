package main

import "gopkg.in/yaml.v2"
import "gopkg.in/urfave/cli.v1"

import pc "github.com/maklesoft/padlock-cloud/padlockcloud"

type CliConfig struct {
	*SubscriptionsServerConfig `yaml:"subscriptions"`
}

type CliApp struct {
	*pc.CliApp
	*SubscriptionsServerConfig
}

func (cliApp *CliApp) LoadConfigFromFile() error {
	// load config file
	yamlData, err := ioutil.ReadFile(cliApp.ConfigPath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(yamlData, struct{
		Subscriptions *SubscriptionsServerConfig `yaml:"subscriptions"`
	}{
		cliApp.SubscriptionsServerConfig
	})
	if err != nil {
		return err
	}

	return nil
}

func (cliApp *CliApp) CreateSubscription(context *cli.Context) error {
	var (
		email      string
		subType    string
		expiresStr string
	)

	if email = context.String("account"); email == "" {
		return errors.New("Please provide an email address!")
	}
	if expiresStr = context.String("expires"); expiresStr == "" {
		return errors.New("Please provide an expiration date!")
	}
	if subType = context.String("type"); subType == "" {
		return errors.New("Please provide a subscription type!")
	}

	expires, err := time.Parse("2006/01/02", expiresStr)
	if err != nil {
		return errors.New("Failed to parse expiration date!")
	}

	acc := &SubscriptionAccount{
		Email: email,
	}

	switch subType {
	case "free":
		acc.FreeSubscription = &FreeSubscription{
			Expires: expires,
		}
	default:
		return errors.New("Invalid subscription type")
	}

	storage := &pc.LevelDBStorage{LevelDBConfig: cliApp.Config.LevelDB}
	if err := storage.Open(); err != nil {
		return err
	}
	defer storage.Close()

	if err := storage.Put(acc); err != nil {
		return err
	}

	return nil
}

func (cliApp *CliApp) DisplaySubscriptionAccount(context *cli.Context) error {
	email := context.Args().Get(0)
	if email == "" {
		return errors.New("Please provide an email address!")
	}

	storage := &pc.LevelDBStorage{LevelDBConfig: cliApp.Config.LevelDB}
	if err := storage.Open(); err != nil {
		return err
	}
	defer storage.Close()

	acc := &SubscriptionAccount{
		Email: email,
	}

	if err := storage.Get(acc); err != nil {
		return err
	}

	yamlData, err := yaml.Marshal(acc)
	if err != nil {
		return err
	}

	fmt.Println(string(yamlData))

	return nil
}

func NewCliApp() *CliApp {
	config := &SubscriptionsServerConfig{}
	app := &CliApp{
		pc.NewCliApp(),
		config,
	}

	runserverCmd := app.Commands[0]

	append(runserverCmd.Flags, []cli.Flag {
		cli.StringFlag{
			Name:        "itunes-shared-secret",
			Usage:       "'Shared Secret' used for authenticating with itunes",
			Value:       "",
			EnvVar:      "PC_ITUNES_SHARED_SECRET",
			Destination: &config.ItunesSharedSecret,
		},
		cli.StringFlag{
			Name:        "itunes-environment",
			Usage:       "Determines which itunes server to send requests to. Can be 'sandbox' (default) or 'production'.",
			Value:       "sandbox",
			EnvVar:      "PC_ITUNES_ENVIRONMENT",
			Destination: &config.ItunesEnvironment,
		},
	}

	append(app.Commands, []cli.Command {
		{
			Name:  "subscriptions",
			Usage: "Commands for managing subscriptions",
			Subcommands: []cli.Command{
				{
					Name:   "update",
					Usage:  "Create subscription for a given account",
					Action: cliApp.CreateSubscription,
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "account",
							Value: "",
							Usage: "Email address of the account to create the subscription for",
						},
						cli.StringFlag{
							Name:  "type",
							Value: "free",
							Usage: "Subscription type; Currently only 'free' is supported (default)",
						},
						cli.StringFlag{
							Name:  "expires",
							Value: "",
							Usage: "Expiration date; Must be in the form 'YYYY/MM/DD'",
						},
					},
				},
				{
					Name:   "displayaccount",
					Usage:  "Display a given subscription account",
					Action: cliApp.DisplaySubscriptionAccount,
				},
			},
		},
	}

	before := app.Before
	app.Before = func(context *cli.Context) error {
		before(context)

		if cliApp.ConfigPath != "" {
			// Replace original config object to prevent other flags from being applied
			cliApp.SubscriptionServerConfig = &SubscriptionServerConfig{}
			return app.LoadConfigFromFile()
		}
		return nil
	}
}
