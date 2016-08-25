package main

import "io/ioutil"
import "errors"
import "time"
import "fmt"

import "gopkg.in/yaml.v2"
import "gopkg.in/urfave/cli.v1"

import pc "github.com/maklesoft/padlock-cloud/padlockcloud"

type CliConfig struct {
	Itunes ItunesConfig `yaml:"itunes"`
}

func (c *CliConfig) LoadFromFile(path string) error {
	// load config file
	yamlData, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(yamlData, c)
	if err != nil {
		return err
	}

	return nil
}

type CliApp struct {
	*pc.CliApp
	SubscriptionServer *Server
	Itunes             *ItunesServer
	Config             *CliConfig
}

func (cliApp *CliApp) InitConfig() {
	cliApp.Config = &CliConfig{}
	cliApp.Itunes.Config = &cliApp.Config.Itunes
}

func (cliApp *CliApp) RunServer(context *cli.Context) error {
	var err error

	if err = cliApp.Server.Init(); err != nil {
		return err
	}
	// Clean up after method returns (should never happen under normal circumstances but you never know)
	defer cliApp.Server.CleanUp()

	pc.HandleInterrupt(cliApp.Server.CleanUp)

	return cliApp.ServeHandler(cliApp.SubscriptionServer)
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

	if err := cliApp.Storage.Open(); err != nil {
		return err
	}
	defer cliApp.Storage.Close()

	if err := cliApp.Storage.Put(acc); err != nil {
		return err
	}

	return nil
}

func (cliApp *CliApp) DisplaySubscriptionAccount(context *cli.Context) error {
	email := context.Args().Get(0)
	if email == "" {
		return errors.New("Please provide an email address!")
	}

	if err := cliApp.Storage.Open(); err != nil {
		return err
	}
	defer cliApp.Storage.Close()

	acc := &SubscriptionAccount{
		Email: email,
	}

	if err := cliApp.Storage.Get(acc); err != nil {
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
	pcCli := pc.NewCliApp()
	itunes := &ItunesServer{}
	server := NewServer(pcCli.Server, itunes)
	app := &CliApp{
		pcCli,
		server,
		itunes,
		nil,
	}
	app.InitConfig()
	config := app.Config

	app.Flags = append(app.Flags, []cli.Flag{
		cli.StringFlag{
			Name:        "itunes-shared-secret",
			Usage:       "'Shared Secret' used for authenticating with itunes",
			Value:       "",
			EnvVar:      "PC_ITUNES_SHARED_SECRET",
			Destination: &config.Itunes.SharedSecret,
		},
		cli.StringFlag{
			Name:        "itunes-environment",
			Usage:       "Determines which itunes server to send requests to. Can be 'sandbox' (default) or 'production'.",
			Value:       "sandbox",
			EnvVar:      "PC_ITUNES_ENVIRONMENT",
			Destination: &config.Itunes.Environment,
		},
	}...)

	runserverCmd := &app.Commands[0]
	runserverCmd.Action = app.RunServer

	app.Commands = append(app.Commands, []cli.Command{
		{
			Name:  "subscriptions",
			Usage: "Commands for managing subscriptions",
			Subcommands: []cli.Command{
				{
					Name:   "update",
					Usage:  "Create subscription for a given account",
					Action: app.CreateSubscription,
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
					Action: app.DisplaySubscriptionAccount,
				},
			},
		},
	}...)

	before := app.Before
	app.Before = func(context *cli.Context) error {
		before(context)

		if app.ConfigPath != "" {
			// Replace original config object to prevent flags from being applied
			app.InitConfig()
			return app.Config.LoadFromFile(app.ConfigPath)
		}
		return nil
	}

	return app
}
