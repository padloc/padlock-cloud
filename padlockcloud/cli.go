package padlockcloud

import "fmt"
import "path/filepath"
import "io/ioutil"
import "errors"
import "encoding/base64"
import "gopkg.in/yaml.v2"
import "gopkg.in/urfave/cli.v1"

type CliConfig struct {
	Log     LogConfig     `yaml:"log"`
	Server  ServerConfig  `yaml:"server"`
	LevelDB LevelDBConfig `yaml:"leveldb"`
	Email   EmailConfig   `yaml:"email"`
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
	*cli.App
	Storage    Storage
	Server     *Server
	Config     *CliConfig
	ConfigPath string
}

func (cliApp *CliApp) InitWithConfig(config *CliConfig) error {
	cliApp.Config = config
	cliApp.Storage = &LevelDBStorage{
		Config: &config.LevelDB,
	}

	return nil
}

func (cliApp *CliApp) InitServer() error {
	var storage Storage
	var sender Sender
	logger := NewLog(&cliApp.Config.Log, nil)

	if cliApp.Config.Server.Test {
		storage = &MemoryStorage{}
		sender = &RecordSender{}
		// Also use CORS when in test mode
		cliApp.Config.Server.Cors = true
	} else {
		storage = cliApp.Storage
		sender = NewEmailSender(&cliApp.Config.Email)
		logger.Sender = sender
	}

	cliApp.Server = NewServer(
		logger,
		storage,
		sender,
		&cliApp.Config.Server,
	)

	return cliApp.Server.Init()
}

func (cliApp *CliApp) RunServer(context *cli.Context) error {
	if err := cliApp.InitServer(); err != nil {
		return err
	}

	cfg, _ := yaml.Marshal(cliApp.Config)
	cliApp.Server.Info.Printf("Running server with the following configuration:\n%s", cfg)

	if cliApp.Server.Config.Test {
		fmt.Println("*** TEST MODE ***")
	} else if cliApp.Server.Config.BaseUrl == "" {
		fmt.Printf("\nWARNING: No --base-url option provided for constructing urls. The 'Host' header\n" +
			"from incoming requests will be used instead which makes the server vulnerable to URL\n" +
			"spoofing attacks! See the README for details.\n\n")
	}

	return cliApp.Server.Start()
}

func (cliApp *CliApp) ListAccounts(context *cli.Context) error {
	if err := cliApp.Storage.Open(); err != nil {
		return err
	}
	defer cliApp.Storage.Close()

	acc := &Account{}
	iter, err := cliApp.Storage.Iterator(acc)
	if err != nil {
		return err
	}
	defer iter.(*LevelDBIterator).Release()

	output := ""
	for iter.Next() {
		iter.Get(acc)
		output = output + acc.Email + "\n"
	}
	fmt.Print(output)

	return nil
}

func (cliApp *CliApp) CreateAccount(context *cli.Context) error {
	email := context.Args().Get(0)
	if email == "" {
		return errors.New("Please provide an email address!")
	}
	acc := &Account{
		Email: email,
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

func (cliApp *CliApp) DisplayAccount(context *cli.Context) error {
	email := context.Args().Get(0)
	if email == "" {
		return errors.New("Please provide an email address!")
	}
	acc := &Account{
		Email: email,
	}

	if err := cliApp.Storage.Open(); err != nil {
		return err
	}
	defer cliApp.Storage.Close()

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

func (cliApp *CliApp) DeleteAccount(context *cli.Context) error {
	email := context.Args().Get(0)
	if email == "" {
		return errors.New("Please provide an email address!")
	}
	acc := &Account{Email: email}

	if err := cliApp.Storage.Open(); err != nil {
		return err
	}
	defer cliApp.Storage.Close()

	return cliApp.Storage.Delete(acc)
}

func genSecret() (string, error) {
	b, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (cliApp *CliApp) GenSecret(context *cli.Context) error {
	s, err := genSecret()
	if err != nil {
		return err
	}
	fmt.Println(s)
	return nil
}

func NewCliApp() *CliApp {
	config := &CliConfig{}
	cliApp := &CliApp{
		App: cli.NewApp(),
	}

	cliApp.Name = "padlock-cloud"
	cliApp.Version = Version
	cliApp.Usage = "A command line interface for Padlock Cloud"

	cliApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "config, c",
			Value:       "",
			Usage:       "Path to configuration file. WARNING: If provided, all other flags will be ingored",
			EnvVar:      "PC_CONFIG_PATH",
			Destination: &cliApp.ConfigPath,
		},
		cli.StringFlag{
			Name:        "log-file",
			Value:       "",
			Usage:       "Path to log file",
			EnvVar:      "PC_LOG_FILE",
			Destination: &config.Log.LogFile,
		},
		cli.StringFlag{
			Name:        "err-file",
			Value:       "",
			Usage:       "Path to error log file",
			EnvVar:      "PC_ERR_FILE",
			Destination: &config.Log.ErrFile,
		},
		cli.StringFlag{
			Name:        "notify-errors",
			Usage:       "Email address to send unexpected errors to",
			Value:       "",
			EnvVar:      "PC_NOTIFY_ERRORS",
			Destination: &config.Log.NotifyErrors,
		},
		cli.StringFlag{
			Name:        "db-path",
			Value:       "db",
			Usage:       "Path to LevelDB database",
			EnvVar:      "PC_LEVELDB_PATH",
			Destination: &config.LevelDB.Path,
		},
		cli.StringFlag{
			Name:        "email-server",
			Value:       "",
			Usage:       "Mail server for sending emails",
			EnvVar:      "PC_EMAIL_SERVER",
			Destination: &config.Email.Server,
		},
		cli.StringFlag{
			Name:        "email-port",
			Value:       "",
			Usage:       "Port to use with mail server",
			EnvVar:      "PC_EMAIL_PORT",
			Destination: &config.Email.Port,
		},
		cli.StringFlag{
			Name:        "email-user",
			Value:       "",
			Usage:       "Username for authentication with mail server",
			EnvVar:      "PC_EMAIL_USER",
			Destination: &config.Email.User,
		},
		cli.StringFlag{
			Name:        "email-password",
			Value:       "",
			Usage:       "Password for authentication with mail server",
			EnvVar:      "PC_EMAIL_PASSWORD",
			Destination: &config.Email.Password,
		},
		cli.StringFlag{
			Name:        "email-from",
			Value:       "",
			Usage:       "Mail address to use as sender of outgoing mails. If empty, email-user is used instead.",
			EnvVar:      "PC_EMAIL_FROM",
			Destination: &config.Email.From,
		},
		cli.StringFlag{
			Name:        "whitelist-path",
			Value:       "",
			Usage:       "File containing line-separated email whitelist",
			EnvVar:      "PC_WHITELIST_PATH",
			Destination: &config.Server.WhitelistPath,
		},
	}

	cliApp.Commands = []cli.Command{
		{
			Name:  "runserver",
			Usage: "Starts a Padlock Cloud server instance",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:        "port, p",
					Usage:       "Port to listen on",
					Value:       3000,
					EnvVar:      "PC_PORT",
					Destination: &config.Server.Port,
				},
				cli.StringFlag{
					Name:        "assets-path",
					Usage:       "Path to assets directory",
					Value:       DefaultAssetsPath,
					EnvVar:      "PC_ASSETS_PATH",
					Destination: &config.Server.AssetsPath,
				},
				cli.StringFlag{
					Name:        "tls-cert",
					Usage:       "Path to TLS certification file",
					Value:       "",
					EnvVar:      "PC_TLS_CERT",
					Destination: &config.Server.TLSCert,
				},
				cli.StringFlag{
					Name:        "tls-key",
					Usage:       "Path to TLS key file",
					Value:       "",
					EnvVar:      "PC_TLS_KEY",
					Destination: &config.Server.TLSKey,
				},
				cli.StringFlag{
					Name:        "base-url",
					Usage:       "Base url for constructing urls",
					Value:       "",
					EnvVar:      "PC_BASE_URL",
					Destination: &config.Server.BaseUrl,
				},
				cli.BoolFlag{
					Name:        "cors",
					Usage:       "Enable Cross-Origin Resource Sharing",
					EnvVar:      "PC_CORS",
					Destination: &config.Server.Cors,
				},
				cli.BoolFlag{
					Name:        "test",
					Usage:       "Enable test mode",
					EnvVar:      "PC_TEST",
					Destination: &config.Server.Test,
				},
			},
			Action: cliApp.RunServer,
		},
		{
			Name:  "accounts",
			Usage: "Commands for managing accounts",
			Subcommands: []cli.Command{
				{
					Name:   "list",
					Usage:  "List existing accounts",
					Action: cliApp.ListAccounts,
				},
				{
					Name:   "create",
					Usage:  "Create new account",
					Action: cliApp.CreateAccount,
				},
				{
					Name:   "display",
					Usage:  "Display account",
					Action: cliApp.DisplayAccount,
				},
				{
					Name:   "delete",
					Usage:  "Delete account",
					Action: cliApp.DeleteAccount,
				},
			},
		},
		{
			Name:   "gensecret",
			Usage:  "Generate random 32 byte secret",
			Action: cliApp.GenSecret,
		},
	}

	cliApp.Before = func(context *cli.Context) error {
		if cliApp.ConfigPath != "" {
			absPath, _ := filepath.Abs(cliApp.ConfigPath)

			fmt.Printf("Loading config from %s - all other flags and environment variables will be ignored!\n", absPath)
			// Replace original config object to prevent flags from being applied
			config = &CliConfig{}
			if err := config.LoadFromFile(cliApp.ConfigPath); err != nil {
				return err
			}
		}

		if err := cliApp.InitWithConfig(config); err != nil {
			fmt.Println(err)
			return err
		}

		return nil
	}

	return cliApp
}
