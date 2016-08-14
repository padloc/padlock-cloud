package main

import "os"
import "log"
import "fmt"
import "net/http"
import "os/signal"
import "path/filepath"
import pc "github.com/maklesoft/padlock-cloud"

func main() {
	appConfig := pc.AppConfig{
		RequireTLS: true,
		AssetsPath: "assets",
		Port:       3000,
	}
	levelDBConfig := pc.LevelDBConfig{
		Path: "db",
	}
	emailConfig := pc.EmailConfig{}

	pc.LoadConfig(&appConfig, &levelDBConfig, &emailConfig)

	// Load templates from assets directory
	templates := pc.LoadTemplates(filepath.Join(appConfig.AssetsPath, "templates"))

	// Initialize app instance
	app, err := pc.NewApp(
		&pc.LevelDBStorage{LevelDBConfig: levelDBConfig},
		&pc.EmailSender{emailConfig},
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
	handler := pc.RateLimit(app, map[pc.Route]pc.RateQuota{
		pc.Route{"POST", "/auth/"}:    pc.RateQuota{pc.PerMin(1), 0},
		pc.Route{"PUT", "/auth/"}:     pc.RateQuota{pc.PerMin(1), 0},
		pc.Route{"DELETE", "/store/"}: pc.RateQuota{pc.PerMin(1), 0},
	})

	// Add CORS middleware
	handler = pc.Cors(handler)

	// Start server
	log.Printf("Starting server on port %v", appConfig.Port)
	err = http.ListenAndServe(fmt.Sprintf(":%d", appConfig.Port), handler)
	if err != nil {
		log.Fatal(err)
	}
}
