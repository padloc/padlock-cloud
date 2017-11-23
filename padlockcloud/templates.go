package padlockcloud

import (
	"encoding/json"
	"errors"
	t "html/template"
	fp "path/filepath"
)

// Wrapper for holding references to template instances used for rendering emails, webpages etc.
type Templates struct {
	BasePage  *t.Template
	BaseEmail *t.Template
	// Email template for api key activation email
	ActivateAuthTokenEmail *t.Template
	// Email template for clients using an outdated api version
	DeprecatedVersionEmail *t.Template
	ErrorPage              *t.Template
	LoginPage              *t.Template
	Dashboard              *t.Template
}

func ExtendTemplate(base *t.Template, path string) (*t.Template, error) {
	if base == nil {
		return nil, errors.New("Base page is nil")
	}

	b, err := base.Clone()
	if err != nil {
		return nil, err
	}

	return b.ParseFiles(path)
}

// Loads templates from given directory
func LoadTemplates(tt *Templates, p string) error {
	var err error

	if tt.BaseEmail, err = t.ParseFiles(fp.Join(p, "email/base.txt.tmpl")); err != nil {
		return err
	}
	if tt.BasePage, err = t.ParseFiles(fp.Join(p, "page/base.html.tmpl")); err != nil {
		return err
	}
	tt.BasePage = tt.BasePage.Funcs(t.FuncMap{
		"jsonify": func(obj interface{}) string {
			val, _ := json.Marshal(obj)
			return string(val)
		},
	})
	if tt.ActivateAuthTokenEmail, err = ExtendTemplate(tt.BaseEmail, fp.Join(p, "email/activate-auth-token.txt.tmpl")); err != nil {
		return err
	}
	if tt.DeprecatedVersionEmail, err = ExtendTemplate(tt.BaseEmail, fp.Join(p, "email/deprecated-version.txt.tmpl")); err != nil {
		return err
	}
	if tt.ErrorPage, err = ExtendTemplate(tt.BasePage, fp.Join(p, "page/error.html.tmpl")); err != nil {
		return err
	}
	if tt.LoginPage, err = ExtendTemplate(tt.BasePage, fp.Join(p, "page/login.html.tmpl")); err != nil {
		return err
	}
	if tt.Dashboard, err = ExtendTemplate(tt.BasePage, fp.Join(p, "page/dashboard.html.tmpl")); err != nil {
		return err
	}

	return nil
}
