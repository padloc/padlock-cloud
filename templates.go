package padlockcloud

import "path/filepath"
import "text/template"
import htmlTemplate "html/template"

// Wrapper for holding references to template instances used for rendering emails, webpages etc.
type Templates struct {
	// Email template for api key activation email
	ActivateAuthTokenEmail *template.Template
	// Email template for deletion confirmation email
	DeleteStoreEmail *template.Template
	// Template for success page for activating an auth token
	ActivateAuthTokenSuccess *htmlTemplate.Template
	// Template for success page for deleting account data
	DeleteStoreSuccess *htmlTemplate.Template
	// Email template for clients using an outdated api version
	DeprecatedVersionEmail *template.Template
}

// Loads templates from given directory
func LoadTemplates(path string) Templates {
	return Templates{
		template.Must(template.ParseFiles(filepath.Join(path, "activate.txt"))),
		template.Must(template.ParseFiles(filepath.Join(path, "delete.txt"))),
		htmlTemplate.Must(htmlTemplate.ParseFiles(filepath.Join(path, "connected.html"))),
		htmlTemplate.Must(htmlTemplate.ParseFiles(filepath.Join(path, "deleted.html"))),
		template.Must(template.ParseFiles(filepath.Join(path, "deprecated.txt"))),
	}
}
