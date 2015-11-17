package main

import "os"
import "flag"
import "text/template"
import htmlTemplate "html/template"
import "log"
import "fmt"

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

func loadTemplates(path string) *Templates {
	return &Templates{
		template.Must(template.ParseFiles(path + "activate.txt")),
		template.Must(template.ParseFiles(path + "delete.txt")),
		htmlTemplate.Must(htmlTemplate.ParseFiles(path + "connected.html")),
		htmlTemplate.Must(htmlTemplate.ParseFiles(path + "deleted.html")),
	}
}

func main() {
	storage := &LevelDBStorage{}
	sender := &EmailSender{}
	var assetsPath, notifyEmail string
	loadEnv(storage, sender, &assetsPath, &notifyEmail)
	templates := loadTemplates(assetsPath + "/templates/")

	app := NewApp(storage, sender, templates, Config{RequireTLS: true, NotifyEmail: "martin@padlock.io"})

	port := flag.Int("p", defaultPort, "Port to listen on")
	flag.Parse()

	log.Printf("Starting server on port %v", *port)
	app.Start(fmt.Sprintf(":%d", *port))
}
