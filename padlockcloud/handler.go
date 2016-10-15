package padlockcloud

import "net/http"
import "io/ioutil"
import "fmt"
import "encoding/json"
import "errors"
import "bytes"
import "time"
import "github.com/gorilla/csrf"

type Handler interface {
	Handle(http.ResponseWriter, *http.Request, *AuthToken) error
}

type VoidHandler struct {
}

func (h *VoidHandler) Handle(w http.ResponseWriter, r *http.Request, a *AuthToken) error {
	return nil
}

type RequestAuthToken struct {
	*Server
}

// Handler function for requesting an api key. Generates a key-token pair and stores them.
// The token can later be used to activate the api key. An email is sent to the corresponding
// email address with an activation url. Expects `email` and `device_name` parameters through either
// multipart/form-data or application/x-www-urlencoded parameters
func (h *RequestAuthToken) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	create := r.Method == "POST"
	email := r.PostFormValue("email")
	tType := r.PostFormValue("type")
	redirect := r.PostFormValue("redirect")
	if tType == "" {
		tType = "api"
	}

	// Make sure email field is set
	if email == "" {
		return &BadRequest{"no email provided"}
	}

	if tType != "api" && tType != "web" {
		return &BadRequest{"unsupported auth token type"}
	}

	if redirect != "" && h.Endpoints[redirect] == nil {
		return &BadRequest{"invalid redirect path"}
	}

	// If the client does not explicitly state that the server should create a new account for this email
	// address in case it does not exist, we have to check if an account exists first
	if !create {
		acc := &Account{Email: email}
		if err := h.Storage.Get(acc); err != nil {
			if err == ErrNotFound {
				// See if there exists a data store for this account
				if err := h.Storage.Get(&DataStore{Account: acc}); err != nil {
					if err == ErrNotFound {
						return &AccountNotFound{email}
					} else {
						return err
					}
				}
			} else {
				return err
			}
		}
	}

	authRequest, err := NewAuthRequest(email, tType)
	if err != nil {
		return err
	}

	authRequest.Redirect = redirect

	// Save key-token pair to database for activating it later in a separate request
	err = h.Storage.Put(authRequest)
	if err != nil {
		return err
	}

	var response []byte
	var emailBody bytes.Buffer
	var emailSubj string

	switch tType {
	case "api":
		if response, err = json.Marshal(map[string]string{
			"id":    authRequest.AuthToken.Id,
			"token": authRequest.AuthToken.Token,
			"email": authRequest.AuthToken.Email,
		}); err != nil {
			return err
		}
		emailSubj = "Connect to Padlock Cloud"

		w.Header().Set("Content-Type", "application/json")
	case "web":
		var buff bytes.Buffer
		if err := h.Templates.LoginPage.Execute(&buff, map[string]interface{}{
			"submitted": true,
			"email":     email,
		}); err != nil {
			return err
		}

		response = buff.Bytes()
		emailSubj = "Log in to Padlock Cloud"

		w.Header().Set("Content-Type", "text/html")
	}

	// Render activation email
	if err := h.Templates.ActivateAuthTokenEmail.Execute(&emailBody, map[string]interface{}{
		"activation_link": fmt.Sprintf("%s/activate/?t=%s", h.BaseUrl(r), authRequest.Token),
		"token":           authRequest.AuthToken,
	}); err != nil {
		return err
	}

	if !h.emailRateLimiter.RateLimit(getIp(r), email) {
		// Send email with activation link
		go func() {
			if err := h.Sender.Send(email, emailSubj, emailBody.String()); err != nil {
				h.LogError(&ServerError{err}, r)
			}
		}()
	} else {
		return &RateLimitExceeded{}
	}

	h.Info.Printf("%s - auth_token:request - %s:%s:%s\n", formatRequest(r), email, tType, authRequest.AuthToken.Id)

	w.WriteHeader(http.StatusAccepted)
	w.Write(response)

	return nil
}

type ActivateAuthToken struct {
	*Server
}

// Hander function for activating a given api key
func (h *ActivateAuthToken) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	token := r.URL.Query().Get("t")

	if token == "" {
		return &BadRequest{"no activation token provided"}
	}

	// Let's check if an unactivate api key exists for this token. If not,
	// the token is not valid
	authRequest := &AuthRequest{Token: token}
	if err := h.Storage.Get(authRequest); err != nil {
		if err == ErrNotFound {
			return &BadRequest{"invalid activation token"}
		} else {
			return err
		}
	}

	authToken := authRequest.AuthToken

	// Create account instance with the given email address.
	acc := &Account{Email: authToken.Email}

	// Fetch existing account data. It's fine if no existing data is found. In that case we'll create
	// a new entry in the database
	if err := h.Storage.Get(acc); err != nil && err != ErrNotFound {
		return err
	}

	// Add the new key to the account
	acc.AddAuthToken(authToken)

	// Save the changes
	if err := h.Storage.Put(acc); err != nil {
		return err
	}

	// Delete the authentication request from the database
	if err := h.Storage.Delete(authRequest); err != nil {
		return err
	}

	redirect := authRequest.Redirect

	if authToken.Type == "web" {
		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    authToken.String(),
			HttpOnly: true,
			Path:     "/",
			Secure:   h.Secure,
		})

		if redirect == "" {
			redirect = "/dashboard/"
		}
	}

	if redirect != "" {
		http.Redirect(w, r, redirect, http.StatusFound)
	} else {
		var b bytes.Buffer
		if err := h.Templates.ActivateAuthTokenSuccess.Execute(&b, map[string]interface{}{
			"token": authToken,
		}); err != nil {
			return err
		}
		b.WriteTo(w)
	}

	h.Info.Printf("%s - auth_token:activate - %s:%s:%s\n", formatRequest(r), acc.Email, authToken.Type, authToken.Id)

	return nil
}

type ReadStore struct {
	*Server
}

// Handler function for retrieving the data associated with a given account
func (h *ReadStore) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	// Retrieve data from database. If not database entry is found, the `Content` field simply stays empty.
	// This is not considered an error. Instead we simply return an empty response body. Clients should
	// know how to deal with this.
	data := &DataStore{Account: acc}
	if err := h.Storage.Get(data); err != nil && err != ErrNotFound {
		return err
	}

	h.Info.Printf("%s - data_store:read - %s\n", formatRequest(r), acc.Email)

	// Return raw data in response body
	w.Write(data.Content)

	return nil
}

type WriteStore struct {
	*Server
}

// Handler function for updating the data associated with a given account. This does NOT implement a
// diffing algorith of any kind since Padlock Cloud is completely ignorant of the data structures involved.
// Instead, clients should retrieve existing data through the `ReadStore` endpoint first, perform any necessary
// decryption/parsing, consolidate the data with any existing local data and then reupload the full,
// encrypted data set
func (h *WriteStore) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	// Read data from request body into `DataStore` instance
	data := &DataStore{Account: acc}
	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	data.Content = content

	// Update database entry
	if err := h.Storage.Put(data); err != nil {
		return err
	}

	h.Info.Printf("%s - data_store:write - %s\n", formatRequest(r), acc.Email)

	// Return with NO CONTENT status code
	w.WriteHeader(http.StatusNoContent)

	return nil
}

type DeleteStore struct {
	*Server
}

func (h *DeleteStore) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	deleted := false

	if r.Method == "POST" {
		if err := h.Storage.Delete(&DataStore{Account: acc}); err != nil {
			return err
		}
		deleted = true
	}

	var b bytes.Buffer
	if err := h.Templates.DeleteStore.Execute(&b, map[string]interface{}{
		"account":        acc,
		"deleted":        deleted,
		csrf.TemplateTag: csrf.TemplateField(r),
	}); err != nil {
		return err
	}

	b.WriteTo(w)
	return nil
}

type RequestDeleteStore struct {
	*Server
}

// Handler function for requesting a data reset for a given account
func (h *RequestDeleteStore) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	// Create AuthRequest
	authRequest, err := NewAuthRequest(acc.Email, "web")
	if err != nil {
		return err
	}

	// After logging in, redirect to delete store page
	authRequest.Redirect = "/deletestore/"

	// Save authrequest
	if err := h.Storage.Put(authRequest); err != nil {
		return err
	}

	// Render confirmation email
	var buff bytes.Buffer
	if err := h.Templates.ActivateAuthTokenEmail.Execute(&buff, map[string]interface{}{
		"token":           authRequest.AuthToken,
		"activation_link": fmt.Sprintf("%s/activate/?t=%s", h.BaseUrl(r), authRequest.Token),
	}); err != nil {
		return err
	}

	body := buff.String()

	if !h.emailRateLimiter.RateLimit(getIp(r), acc.Email) {
		// Send email with activation link
		go func() {
			if err := h.Sender.Send(acc.Email, "Padlock Cloud Delete Request", body); err != nil {
				h.LogError(&ServerError{err}, r)
			}
		}()
	} else {
		return &RateLimitExceeded{}
	}

	h.Info.Printf("%s - data_store:request_delete - %s", formatRequest(r), acc.Email)

	// Send ACCEPTED status code
	w.WriteHeader(http.StatusAccepted)

	return nil
}

type LoginPage struct {
	*Server
}

func (h *LoginPage) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	var b bytes.Buffer
	if err := h.Templates.LoginPage.Execute(&b, nil); err != nil {
		return err
	}

	b.WriteTo(w)

	return nil
}

type Dashboard struct {
	*Server
}

func (h *Dashboard) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	var b bytes.Buffer
	if err := h.Templates.Dashboard.Execute(&b, map[string]interface{}{
		"account":        acc,
		csrf.TemplateTag: csrf.TemplateField(r),
	}); err != nil {
		return err
	}

	b.WriteTo(w)
	return nil
}

type Logout struct {
	*Server
}

func (h *Logout) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	acc.RemoveAuthToken(auth)
	if err := h.Storage.Put(acc); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.Secure,
	})
	http.Redirect(w, r, "/login/", http.StatusFound)
	return nil
}

type Revoke struct {
	*Server
}

func (h *Revoke) Handle(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	token := r.PostFormValue("token")
	id := r.PostFormValue("id")
	if token == "" && id == "" {
		return &BadRequest{"No token or id provided"}
	}

	acc := auth.Account()

	t := &AuthToken{Token: token, Id: id}
	if _, t = acc.findAuthToken(t); t == nil {
		return &BadRequest{"No such token"}
	}

	t.Expires = time.Now().Add(-time.Minute)

	acc.UpdateAuthToken(t)

	if err := h.Storage.Put(acc); err != nil {
		return err
	}

	http.Redirect(w, r, "/dashboard/", http.StatusFound)

	return nil
}

type StaticHandler struct {
	fh http.Handler
}

func (h *StaticHandler) Handle(w http.ResponseWriter, r *http.Request, a *AuthToken) error {
	h.fh.ServeHTTP(w, r)
	return nil
}

func NewStaticHandler(dir string, path string) *StaticHandler {
	// Serve up static files
	fh := http.StripPrefix(path, http.FileServer(http.Dir(dir)))
	return &StaticHandler{fh}
}

type RootHandler struct {
	*Server
}

func (h *RootHandler) Handle(w http.ResponseWriter, r *http.Request, a *AuthToken) error {
	if r.URL.Path != "/" {
		return &UnsupportedEndpoint{r.URL.Path}
	}

	http.Redirect(w, r, "/dashboard/", http.StatusFound)
	return nil
}

type HandlerFunc func(http.ResponseWriter, *http.Request, *AuthToken) error

func (f HandlerFunc) Handle(w http.ResponseWriter, r *http.Request, a *AuthToken) error {
	return f(w, r, a)
}

type Endpoint struct {
	Handlers map[string]Handler
	Version  int
	AuthType string
}

func (endpoint *Endpoint) Handle(w http.ResponseWriter, r *http.Request, a *AuthToken) error {
	if h := endpoint.Handlers[r.Method]; h != nil {
		return h.Handle(w, r, a)
	} else {
		return errors.New("Unexpected method: " + r.Method)
	}
}

func HttpHandler(h Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Handle(w, r, nil)
	})
}
