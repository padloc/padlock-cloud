package main

import "os"
import "log"
import "fmt"
import "net/http"
import "os/signal"
import pc "github.com/maklesoft/padlock-cloud"

func main() {
	config := struct {
		Port int `env:"PC_PORT" cli:"port" yaml:"port"`
	}{
		3000,
	}
	appConfig := pc.AppConfig{
		RequireTLS: true,
		AssetsPath: "assets",
	}
	levelDBConfig := pc.LevelDBConfig{
		Path: "db",
	}
	emailConfig := pc.EmailConfig{}

	pc.LoadConfig(&config, &appConfig, &levelDBConfig, &emailConfig)

	// Initialize app instance
	app := pc.NewApp(
		&pc.LevelDBStorage{LevelDBConfig: levelDBConfig},
		&pc.EmailSender{emailConfig},
		nil,
		appConfig,
	)

	app.LoadTemplatesFromAssets()

	// Open storage
	err := app.Storage.Open()
	if err != nil {
		log.Fatal(err)
	}

	// Close database connection when the method returns
	defer app.Storage.Close()

	// Handle INTERRUPT and KILL signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		s := <-c
		log.Printf("Received %v signal. Exiting...", s)
		app.Storage.Close()
		os.Exit(0)
	}()

	// Add rate limiting middleWare
	handler := pc.RateLimit(app, map[pc.Route]pc.RateQuota{
		pc.Route{"POST", "/auth/"}:    pc.RateQuota{pc.PerMin(1), 0},
		pc.Route{"PUT", "/auth/"}:     pc.RateQuota{pc.PerMin(1), 0},
		pc.Route{"DELETE", "/store/"}: pc.RateQuota{pc.PerMin(1), 0},
	})

	// Add CORS middleware
	handler = pc.Cors(handler)

	// Start server
	log.Printf("Starting server on port %v", config.Port)
	err = http.ListenAndServe(fmt.Sprintf(":%d", config.Port), handler)
	if err != nil {
		log.Fatal(err)
	}
}
