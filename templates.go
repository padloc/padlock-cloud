package main

import "path/filepath"
import t "text/template"
import ht "html/template"

// Wrapper for holding references to template instances used for rendering emails, webpages etc.
type Templates struct {
	// Email template for api key activation email
	ActivateAuthTokenEmail *t.Template
	// Email template for deletion confirmation email
	DeleteStoreEmail *t.Template
	// Template for success page for activating an auth token
	ActivateAuthTokenSuccess *ht.Template
	// Template for success page for deleting account data
	DeleteStoreSuccess *ht.Template
	// Email template for clients using an outdated api version
	DeprecatedVersionEmail *t.Template
}

// Loads templates from given directory
func LoadTemplates(path string) (*Templates, error) {
	var err error

	tt := &Templates{}

	tPath := func(filename string) string {
		return filepath.Join(path, filename)
	}

	if tt.ActivateAuthTokenEmail, err = t.ParseFiles(tPath("activate-auth-token-email.txt")); err != nil {
		return nil, err
	}
	if tt.DeleteStoreEmail, err = t.ParseFiles(tPath("delete-store-email.txt")); err != nil {
		return nil, err
	}
	if tt.ActivateAuthTokenSuccess, err = ht.ParseFiles(tPath("activate-auth-token-success.html")); err != nil {
		return nil, err
	}
	if tt.DeleteStoreSuccess, err = ht.ParseFiles(tPath("delete-store-success.html")); err != nil {
		return nil, err
	}
	if tt.DeprecatedVersionEmail, err = t.ParseFiles(tPath("deprecated-version-email.txt")); err != nil {
		return nil, err
	}

	return tt, nil
}
