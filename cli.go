package main

import "os"
import "log"
import "fmt"
import "net/http"
import "os/signal"
import "path/filepath"
import "io/ioutil"
import "gopkg.in/yaml.v2"
import "gopkg.in/urfave/cli.v1"

var gopath = os.Getenv("GOPATH")

func loadConfigFromFile(path string, appConfig *AppConfig, levelDBConfig *LevelDBConfig, emailConfig *EmailConfig) error {
	// load config file
	yamlData, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	cfg := struct {
		*AppConfig     `yaml:"app"`
		*LevelDBConfig `yaml:"leveldb"`
		*EmailConfig   `yaml:"email"`
	}{
		appConfig,
		levelDBConfig,
		emailConfig,
	}

	err = yaml.Unmarshal(yamlData, &cfg)
	if err != nil {
		return err
	}

	return nil
}

func NewCliApp() *cli.App {
	var configPath string

	appConfig := AppConfig{}
	levelDBConfig := LevelDBConfig{}
	emailConfig := EmailConfig{}

	cliApp := cli.NewApp()
	cliApp.Name = "padlock-cloud"
	cliApp.Usage = "A command line interface for Padlock Cloud"

	cliApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "config, c",
			Value:       "",
			Usage:       "Path to configuration file",
			EnvVar:      "PC_CONFIG_PATH",
			Destination: &configPath,
		},
		cli.StringFlag{
			Name:        "db-path",
			Value:       "",
			Usage:       "Path to LevelDB database",
			EnvVar:      "PC_LEVELDB_PATH",
			Destination: &(levelDBConfig.Path),
		},
		cli.StringFlag{
			Name:        "email-server",
			Value:       "",
			Usage:       "Mail server for sending emails",
			EnvVar:      "PC_EMAIL_SERVER",
			Destination: &(emailConfig.Server),
		},
		cli.StringFlag{
			Name:        "email-port",
			Value:       "",
			Usage:       "Port to use with mail server",
			EnvVar:      "PC_EMAIL_PORT",
			Destination: &(emailConfig.Port),
		},
		cli.StringFlag{
			Name:        "email-user",
			Value:       "",
			Usage:       "Username for authentication with mail server",
			EnvVar:      "PC_EMAIL_USER",
			Destination: &(emailConfig.User),
		},
		cli.StringFlag{
			Name:        "email-password",
			Value:       "",
			Usage:       "Password for authentication with mail server",
			EnvVar:      "PC_EMAIL_PASSWORD",
			Destination: &(emailConfig.Password),
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
					Destination: &(appConfig.Port),
				},
				cli.StringFlag{
					Name:        "assets-path",
					Usage:       "Path to assets directory",
					Value:       filepath.Join(gopath, "src/github.com/maklesoft/padlock-cloud/assets"),
					EnvVar:      "PC_ASSETS_PATH",
					Destination: &(appConfig.AssetsPath),
				},
				cli.BoolFlag{
					Name:        "require-tls",
					Usage:       "Reject insecure connections",
					EnvVar:      "PC_REQUIRE_TLS",
					Destination: &(appConfig.RequireTLS),
				},
				cli.StringFlag{
					Name:        "notify-email",
					Usage:       "Email address to send error reports to",
					Value:       "",
					EnvVar:      "PC_NOTIFY_EMAIL",
					Destination: &(appConfig.NotifyEmail),
				},
			},
			Action: func(context *cli.Context) error {
				var err error

				if configPath != "" {
					err = loadConfigFromFile(configPath, &appConfig, &levelDBConfig, &emailConfig)
					if err != nil {
						log.Fatal(err)
					}
				}

				// Load templates from assets directory
				templates, err := LoadTemplates(filepath.Join(appConfig.AssetsPath, "templates"))

				if err != nil {
					log.Fatalf("%s\nFailed to load Template! Did you specify the correct assets path? (Currently \"%s\")",
						err, appConfig.AssetsPath)
				}

				// Initialize app instance
				app, err := NewApp(
					&LevelDBStorage{LevelDBConfig: levelDBConfig},
					&EmailSender{emailConfig},
					templates,
					appConfig,
				)

				if err != nil {
					log.Fatal(err)
				}

				// Add rate limiting middleWare
				handler := RateLimit(app, map[Route]RateQuota{
					Route{"POST", "/auth/"}:    RateQuota{PerMin(1), 0},
					Route{"PUT", "/auth/"}:     RateQuota{PerMin(1), 0},
					Route{"DELETE", "/store/"}: RateQuota{PerMin(1), 0},
				})

				// Add CORS middleware
				handler = Cors(handler)

				// Clean up after method returns (should never happen under normal circumstances but you never know)
				defer app.CleanUp()

				// Handle INTERRUPT and KILL signals
				c := make(chan os.Signal, 1)
				signal.Notify(c, os.Interrupt, os.Kill)
				go func() {
					s := <-c
					log.Printf("Received %v signal. Exiting...", s)
					app.CleanUp()
					os.Exit(0)
				}()

				// Start server
				log.Printf("Starting server on port %v", appConfig.Port)
				err = http.ListenAndServe(fmt.Sprintf(":%d", appConfig.Port), handler)
				if err != nil {
					log.Fatal(err)
				}

				return nil
			},
		},
	}

	return cliApp
}
