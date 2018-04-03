package padlockcloud

import "testing"
import "fmt"
import "io/ioutil"
import "os"
import "path/filepath"
import "time"
import "reflect"
import "gopkg.in/yaml.v2"

func NewSampleConfig(dir string) CliConfig {
	logfile := filepath.Join(dir, "LOG.txt")
	errfile := filepath.Join(dir, "ERR.txt")
	dbpath := filepath.Join(dir, "db")
	secret, _ := genSecret()

	return CliConfig{
		LogConfig{
			LogFile:      logfile,
			ErrFile:      errfile,
			NotifyErrors: "notify@padlock.io",
		},
		ServerConfig{
			AssetsPath: "../assets",
			Port:       5555,
			TLSCert:    "",
			TLSKey:     "",
			BaseUrl:    "http://example.com",
			Secret:     secret,
		},
		LevelDBConfig{
			Path: dbpath,
		},
		EmailConfig{
			User:     "emailuser",
			Password: "emailpassword",
			Server:   "myemailserver.com",
			Port:     "4321",
			From:     "emailfrom",
		},
	}
}

func TestCliFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cfg := NewSampleConfig(dir)

	app := NewCliApp()

	go func() {
		if err := app.Run([]string{"padlock-cloud",
			"--log-file", cfg.Log.LogFile,
			"--db-path", cfg.LevelDB.Path,
			"--email-user", cfg.Email.User,
			"runserver",
			"--port", fmt.Sprintf("%d", cfg.Server.Port),
		}); err != nil {
			t.Fatal(err)
		}
	}()

	time.Sleep(time.Millisecond * 100)

	// t.Logf("%v, %v, %v, %v", app.Log, app.Storage, app.Sender, app.Server)
	t.Logf("teting %+v", app)

	if app.Server.Log.Config.LogFile != cfg.Log.LogFile ||
		app.Storage.(*LevelDBStorage).Config.Path != cfg.LevelDB.Path ||
		app.Server.Sender.(*EmailSender).Config.User != cfg.Email.User ||
		app.Server.Config.Port != cfg.Server.Port {
		t.Fatal("Values provided via flags should be carried over into corresponding configs")
	}

	app.Server.Stop(time.Second)
}

func TestCliConfigFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cfg := NewSampleConfig(dir)
	cfgPath := filepath.Join(dir, "config.yaml")

	yamlData, _ := yaml.Marshal(cfg)
	if err = ioutil.WriteFile(cfgPath, yamlData, 0644); err != nil {
		t.Fatal(err)
	}

	app := NewCliApp()

	go func() {
		if err := app.Run([]string{"padlock-cloud",
			"--config", cfgPath,
			"runserver",
		}); err != nil {
			t.Fatal(err)
		}
	}()

	time.Sleep(time.Millisecond * 100)

	if !reflect.DeepEqual(*app.Server.Log.Config, cfg.Log) ||
		!reflect.DeepEqual(*app.Storage.(*LevelDBStorage).Config, cfg.LevelDB) ||
		!reflect.DeepEqual(*app.Server.Config, cfg.Server) ||
		!reflect.DeepEqual(*app.Server.Sender.(*EmailSender).Config, cfg.Email) {
		yamlData2, _ := yaml.Marshal(app.Config)
		t.Fatalf("Config file not loaded correctly. \n\nExpected: \n\n%s\n\n Got: \n\n%s\n", yamlData, yamlData2)
	}

	app.Server.Stop(time.Second)
}
