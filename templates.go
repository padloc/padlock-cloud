package main

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
func LoadTemplates(path string) (Templates, error) {
	var err error

	activateAuthTokenEmail, err := template.ParseFiles(filepath.Join(path, "activate-auth-token-email.txt"))
	deleteStoreEmail, err := template.ParseFiles(filepath.Join(path, "delete-store-email.txt"))
	activateAuthTokenSuccess, err := htmlTemplate.ParseFiles(filepath.Join(path, "activate-auth-token-success.html"))
	deleteStoreSuccess, err := htmlTemplate.ParseFiles(filepath.Join(path, "delete-store-success.html"))
	deprecatedVersionEmail, err := template.ParseFiles(filepath.Join(path, "deprecated-version-email.txt"))

	templates := Templates{
		activateAuthTokenEmail,
		deleteStoreEmail,
		activateAuthTokenSuccess,
		deleteStoreSuccess,
		deprecatedVersionEmail,
	}

	return templates, err
}
