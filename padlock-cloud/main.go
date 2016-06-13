package main

import "os"
import "flag"
import "log"
import "fmt"
import "net/http"
import "os/signal"
import pc "github.com/maklesoft/padlock-cloud"

const defaultPort = 3000

func main() {
	// Parse options from command line flags
	port := flag.Int("p", defaultPort, "Port to listen on")
	requireTLS := flag.Bool("https-only", false, "Set to true to only allow requests via https")
	flag.Parse()

	// Initialize app instance
	app := pc.NewApp(
		&pc.LevelDBStorage{},
		&pc.EmailSender{},
		nil,
		pc.Config{
			RequireTLS: *requireTLS,
		},
	)

	app.LoadTemplatesFromAssets()

	// Open storage
	err := app.Storage.Open()
	if err != nil {
		log.Fatal(err)
	}

	// Close database connection when the method returns
	defer app.Storage.Close()

	handler := pc.RateLimit(pc.Cors(app), map[pc.Route]pc.RateQuota{
		pc.Route{"POST", "/auth/"}:    pc.RateQuota{pc.PerMin(1), 0},
		pc.Route{"PUT", "/auth/"}:     pc.RateQuota{pc.PerMin(1), 0},
		pc.Route{"DELETE", "/store/"}: pc.RateQuota{pc.PerMin(1), 0},
	})

	// Start server
	log.Printf("Starting server on port %v", *port)
	err = http.ListenAndServe(fmt.Sprintf(":%d", *port), handler)
	if err != nil {
		log.Fatal(err)
	}

	// Handle INTERRUPT and KILL signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		s := <-c
		log.Printf("Received %v signal. Exiting...", s)
		app.Storage.Close()
		os.Exit(0)
	}()
}
