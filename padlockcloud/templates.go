package padlockcloud

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
	ErrorPage              *ht.Template
}

// Loads templates from given directory
func LoadTemplates(path string) (*Templates, error) {
	var err error

	tt := &Templates{}

	tp := func(filename string) string {
		return filepath.Join(path, filename)
	}

	if tt.ActivateAuthTokenEmail, err = t.ParseFiles(
		tp("email/base.txt"),
		tp("email/activate-auth-token.txt"),
	); err != nil {
		return nil, err
	}
	if tt.DeleteStoreEmail, err = t.ParseFiles(
		tp("email/base.txt"),
		tp("email/delete-store.txt"),
	); err != nil {
		return nil, err
	}
	if tt.DeprecatedVersionEmail, err = t.ParseFiles(
		tp("email/base.txt"),
		tp("email/deprecated-version.txt"),
	); err != nil {
		return nil, err
	}
	if tt.ActivateAuthTokenSuccess, err = ht.ParseFiles(
		tp("page/base.html"),
		tp("page/activate-auth-token-success.html"),
	); err != nil {
		return nil, err
	}
	if tt.DeleteStoreSuccess, err = ht.ParseFiles(
		tp("page/base.html"),
		tp("page/delete-store-success.html"),
	); err != nil {
		return nil, err
	}
	if tt.ErrorPage, err = ht.ParseFiles(tp("page/base.html"), tp("page/error.html")); err != nil {
		return nil, err
	}

	return tt, nil
}
