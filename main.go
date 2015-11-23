package main

import "os"
import "flag"
import "text/template"
import htmlTemplate "html/template"
import "log"
import "fmt"
import "path/filepath"
import "os/signal"

const defaultDbPath = "./db"
const defaultAssetsPath = "./assets"
const defaultPort = 3000

// Loads settings from environment variables
func loadEnv(storage *LevelDBStorage, emailSender *EmailSender, assetsPath *string, notifyEmail *string) {
	emailSender.User = os.Getenv("PADLOCK_EMAIL_USERNAME")
	emailSender.Server = os.Getenv("PADLOCK_EMAIL_SERVER")
	emailSender.Port = os.Getenv("PADLOCK_EMAIL_PORT")
	emailSender.Password = os.Getenv("PADLOCK_EMAIL_PASSWORD")
	*assetsPath = os.Getenv("PADLOCK_ASSETS_PATH")
	*notifyEmail = os.Getenv("PADLOCK_NOTIFY_EMAIL")
	if *assetsPath == "" {
		*assetsPath = defaultAssetsPath
	}
	storage.Path = os.Getenv("PADLOCK_DB_PATH")
	if storage.Path == "" {
		storage.Path = defaultDbPath
	}
}

// Loads templates from given directory
func loadTemplates(path string) *Templates {
	return &Templates{
		template.Must(template.ParseFiles(filepath.Join(path, "activate.txt"))),
		template.Must(template.ParseFiles(filepath.Join(path, "delete.txt"))),
		htmlTemplate.Must(htmlTemplate.ParseFiles(filepath.Join(path, "connected.html"))),
		htmlTemplate.Must(htmlTemplate.ParseFiles(filepath.Join(path, "deleted.html"))),
	}
}

func main() {
	// Initialize dependencies and load config
	storage := &LevelDBStorage{}
	sender := &EmailSender{}
	var assetsPath, notifyEmail string
	loadEnv(storage, sender, &assetsPath, &notifyEmail)
	templates := loadTemplates(filepath.Join(assetsPath, "templates"))

	// Parse options from command line flags
	port := flag.Int("p", defaultPort, "Port to listen on")
	requireTLS := flag.Bool("https-only", false, "Set to true to only allow requests via https")
	flag.Parse()

	// Initialize app instance
	app := NewApp(storage, sender, templates, Config{RequireTLS: *requireTLS, NotifyEmail: notifyEmail})

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
