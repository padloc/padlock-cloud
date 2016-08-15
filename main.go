package main

import "os"
import "log"
import "fmt"
import "net/http"
import "os/signal"
import "path/filepath"

var gopath = os.Getenv("GOPATH")

func main() {
	appConfig := AppConfig{
		RequireTLS: true,
		AssetsPath: filepath.Join(gopath, "src/github.com/maklesoft/padlock-cloud/assets"),
		Port:       3000,
	}
	levelDBConfig := LevelDBConfig{
		Path: "db",
	}
	emailConfig := EmailConfig{}

	LoadConfig(&appConfig, &levelDBConfig, &emailConfig)

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

	// Add rate limiting middleWare
	handler := RateLimit(app, map[Route]RateQuota{
		Route{"POST", "/auth/"}:    RateQuota{PerMin(1), 0},
		Route{"PUT", "/auth/"}:     RateQuota{PerMin(1), 0},
		Route{"DELETE", "/store/"}: RateQuota{PerMin(1), 0},
	})

	// Add CORS middleware
	handler = Cors(handler)

	// Start server
	log.Printf("Starting server on port %v", appConfig.Port)
	err = http.ListenAndServe(fmt.Sprintf(":%d", appConfig.Port), handler)
	if err != nil {
		log.Fatal(err)
	}
}
