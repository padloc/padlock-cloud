package main

import "os"
import "flag"
import "log"
import "fmt"
import "os/signal"
import "github.com/maklesoft/padlock-cloud"

const defaultPort = 3000

func main() {
	// Parse options from command line flags
	port := flag.Int("p", defaultPort, "Port to listen on")
	requireTLS := flag.Bool("https-only", false, "Set to true to only allow requests via https")
	flag.Parse()

	// Initialize app instance
	app := padlockcloud.NewApp(
		&padlockcloud.LevelDBStorage{},
		&padlockcloud.EmailSender{},
		nil,
		padlockcloud.Config{
			RequireTLS: *requireTLS,
		},
	)

	app.LoadTemplatesFromAssets()

	// Handle INTERRUPT and KILL signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		s := <-c
		log.Printf("Received %v signal. Exiting...", s)
		app.Stop()
		os.Exit(0)
	}()

	// Start server
	log.Printf("Starting server on port %v", *port)
	app.Start(fmt.Sprintf(":%d", *port))
}
