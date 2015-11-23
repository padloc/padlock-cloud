package main

import "net/http"
import "io/ioutil"
import "crypto/rand"
import "fmt"
import "log"
import "net/smtp"
import "encoding/json"
import "regexp"
import "errors"
import "bytes"
import "text/template"
import htmlTemplate "html/template"

// Error singletons
var (
	// No or invalid token provided
	ErrInvalidToken = errors.New("padlock: invalid token")
	// No valid authentication credentials provided
	ErrNotAuthenticated = errors.New("padlock: not authenticated")
	// Received request for a HTTP method not supported for a given endpoint
	ErrWrongMethod = errors.New("padlock: wrong http method")
	// Received request via http:// protocol when https:// is explicitly required
	ErrInsecureConnection = errors.New("padlock: insecure connection")
	// Recovered from a panic
	ErrPanic = errors.New("padlock: panic")
)

// Regex pattern for checking for RFC4122-compliant uuids
const uuidPattern = "[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89aAbB][a-f0-9]{3}-[a-f0-9]{12}"

// RFC4122-compliant uuid generator
func uuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Extracts a uuid-formated token from a given url
func tokenFromUrl(url string, baseUrl string) (string, error) {
	re := regexp.MustCompile("^" + baseUrl + "(?P<token>" + uuidPattern + ")$")

	if !re.MatchString(url) {
		return "", ErrInvalidToken
	}

	return re.FindStringSubmatch(url)[1], nil
}

// Returns the appropriate protocol based on whether a request was made via https or not
func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	} else {
		return "http"
	}
}

// Sender is a interface that exposes the `Send` method for sending messages with a subject to a given
// receiver.
type Sender interface {
	Send(receiver string, subject string, message string) error
}

// EmailSender implements the `Sender` interface for emails
type EmailSender struct {
	// User name used for authentication with the mail server
	User string
	// Mail server address
	Server string
	// Port on which to contact the mail server
	Port string
	// Password used for authentication with the mail server
	Password string
}

// Attempts to send an email to a given receiver. Through `smpt.SendMail`
func (sender *EmailSender) Send(rec string, subject string, body string) error {
	auth := smtp.PlainAuth(
		"",
		sender.User,
		sender.Password,
		sender.Server,
	)

	message := fmt.Sprintf("Subject: %s\r\nFrom: Padlock Cloud <%s>\r\n\r\n%s", subject, sender.User, body)
	return smtp.SendMail(
		sender.Server+":"+sender.Port,
		auth,
		sender.User,
		[]string{rec},
		[]byte(message),
	)
}

// A wrapper for an api key containing some meta info like the user and device name
type ApiKey struct {
	Email      string `json:"email"`
	DeviceName string `json:"device_name"`
	Key        string `json:"key"`
}

// A struct representing a user with a set of api keys
type Account struct {
	// The email servers as a unique identifier and as a means for
	// requesting/activating api keys
	Email string
	// A set of api keys that can be used to access the data associated with this
	// account
	ApiKeys []ApiKey
}

// Implements the `Key` method of the `Storable` interface
func (acc *Account) Key() []byte {
	return []byte(acc.Email)
}

// Implementation of the `Storable.Deserialize` method
func (acc *Account) Deserialize(data []byte) error {
	return json.Unmarshal(data, acc)
}

// Implementation of the `Storable.Serialize` method
func (acc *Account) Serialize() ([]byte, error) {
	return json.Marshal(acc)
}

// Removes the api key for a given device name
func (a *Account) RemoveKeyForDevice(deviceName string) {
	for i, apiKey := range a.ApiKeys {
		if apiKey.DeviceName == deviceName {
			a.ApiKeys = append(a.ApiKeys[:i], a.ApiKeys[i+1:]...)
			return
		}
	}
}

// Adds an api key to this account. If an api key for the given device
// is already registered, that one will be replaced
func (a *Account) SetKey(apiKey ApiKey) {
	a.RemoveKeyForDevice(apiKey.DeviceName)
	a.ApiKeys = append(a.ApiKeys, apiKey)
}

// Checks if a given api key is valid for this account
func (a *Account) Validate(key string) bool {
	// Check if the account contains any ApiKey with that matches
	// the given key
	for _, apiKey := range a.ApiKeys {
		if apiKey.Key == key {
			return true
		}
	}

	return false
}

// AuthRequest represents an api key - activation token pair used to activate a given api key
// `AuthRequest.Token` is used to activate the ApiKey through a separate channel (e.g. email)
type AuthRequest struct {
	Token  string
	ApiKey ApiKey
}

// Implementation of the `Storable.Key` interface method
func (ar *AuthRequest) Key() []byte {
	return []byte(ar.Token)
}

// Implementation of the `Storable.Deserialize` method
func (ar *AuthRequest) Deserialize(data []byte) error {
	return json.Unmarshal(data, &ar.ApiKey)
}

// Implementation of the `Storable.Serialize` method
func (ar *AuthRequest) Serialize() ([]byte, error) {
	return json.Marshal(&ar.ApiKey)
}

// Represents a request for reseting the data associated with a given account. `RequestReset.Token` is used
// for validating the request through a separate channel (e.g. email)
type ResetRequest struct {
	Token   string
	Account string
}

// Implementation of the `Storable.Key` interface method
func (rr *ResetRequest) Key() []byte {
	return []byte(rr.Token)
}

// Implementation of the `Storable.Deserialize` interface method
func (rr *ResetRequest) Deserialize(data []byte) error {
	rr.Account = string(data)
	return nil
}

// Implementation of the `Storable.Serialize` interface method
func (rr *ResetRequest) Serialize() ([]byte, error) {
	return []byte(rr.Account), nil
}

// Data represents the data associated to a given account
type Data struct {
	Account *Account
	Content []byte
}

// Implementation of the `Storable.Key` interface method
func (d *Data) Key() []byte {
	return []byte(d.Account.Email)
}

// Implementation of the `Storable.Deserialize` interface method
func (d *Data) Deserialize(data []byte) error {
	d.Content = data
	return nil
}

// Implementation of the `Storable.Serialize` interface method
func (d *Data) Serialize() ([]byte, error) {
	return d.Content, nil
}

// Wrapper for holding references to template instances used for rendering emails, webpages etc.
type Templates struct {
	// Email template for api key activation email
	ActivationEmail *template.Template
	// Email template for deletion confirmation email
	DataResetEmail *template.Template
	// Template for connected page
	ConnectionSuccess *htmlTemplate.Template
	// Template for connected page
	DataResetSuccess *htmlTemplate.Template
}

// Config contains various configuration data
type Config struct {
	// If true, all requests via plain http will be rejected. Only https requests are allowed
	RequireTLS bool
	// Email address for sending error reports; Leave empty for no notifications
	NotifyEmail string
}

// The App type holds all the contextual data and logic used for running a Padlock Cloud instances
// Users should use the `NewApp` function to instantiate an `App` instance
type App struct {
	*http.ServeMux
	Sender
	Storage
	*Templates
	Config
}

// Retreives Account object from a http.Request object by evaluating the Authorization header and
// cross-checking it with api keys of existing accounts. Returns an `ErrNotAuthenticated` error
// if no valid Authorization header is provided or if the provided email:api_key pair does not match
// any of the accounts in the database.
func (app *App) accountFromRequest(r *http.Request) (*Account, error) {
	// Extract email and authentication token from Authorization header
	re := regexp.MustCompile("^ApiKey (?P<email>.+):(?P<key>.+)$")
	authHeader := r.Header.Get("Authorization")

	// Check if the Authorization header exists and is well formed
	if !re.MatchString(authHeader) {
		return nil, ErrNotAuthenticated
	}

	// Extract email and api key from Authorization header
	matches := re.FindStringSubmatch(authHeader)
	email, key := matches[1], matches[2]
	acc := &Account{Email: email}

	// Fetch account for the given email address
	err := app.Get(acc)

	if err != nil {
		return nil, ErrNotAuthenticated
	}

	// Check if the provide api key is valid
	if !acc.Validate(key) {
		return nil, ErrNotAuthenticated
	}

	return acc, nil
}

// Global error handler. Writes a appropriate response to the provided `http.ResponseWriter` object and
// logs / notifies of internal server errors
func (app *App) handleError(e error, w http.ResponseWriter, r *http.Request) {
	switch e {
	case ErrInvalidToken:
		{
			http.Error(w, "", http.StatusBadRequest)
		}
	case ErrNotAuthenticated:
		{
			http.Error(w, "", http.StatusUnauthorized)
		}
	case ErrWrongMethod:
		{
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	case ErrNotFound:
		{
			http.Error(w, "", http.StatusNotFound)
		}
	case ErrInsecureConnection:
		{
			http.Error(w, "", http.StatusForbidden)
		}
	default:
		{
			http.Error(w, "", http.StatusInternalServerError)

			log.Printf("Internal Server Error: %v", e)

			if app.NotifyEmail != "" {
				go app.Send(app.NotifyEmail, "Padlock Cloud Error Notification",
					fmt.Sprintf("Internal server error: %v\nRequest: %v", e, r))
			}
		}
	}
}

// Handler function for requesting an api key. Generates a key-token pair and stores them.
// The token can later be used to activate the api key. An email is sent to the corresponding
// email address with an activation url. Expects `email` and `device_name` parameters through either
// multipart/form-data or application/x-www-urlencoded parameters
func (app *App) RequestApiKey(w http.ResponseWriter, r *http.Request) {
	// TODO: Add validation
	email, deviceName := r.PostFormValue("email"), r.PostFormValue("device_name")

	// Generate key-token pair
	key := uuid()
	token := uuid()
	apiKey := ApiKey{
		email,
		deviceName,
		key,
	}

	// Save key-token pair to database for activating it later in a separate request
	err := app.Put(&AuthRequest{token, apiKey})
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Render activation email
	var buff bytes.Buffer
	err = app.Templates.ActivationEmail.Execute(&buff, map[string]string{
		"email":           apiKey.Email,
		"device_name":     apiKey.DeviceName,
		"activation_link": fmt.Sprintf("%s://%s/activate/%s", schemeFromRequest(r), r.Host, token),
	})
	if err != nil {
		app.handleError(err, w, r)
		return
	}
	body := buff.String()

	// Send email with activation link
	go app.Send(email, "Connect to Padlock Cloud", body)

	// Serialize api key
	data, err := json.Marshal(apiKey)
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Send JSON representation of api key along with CREATED status code
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}

// Hander function for activating a given api key
func (app *App) ActivateApiKey(w http.ResponseWriter, r *http.Request) {
	// Extract activation token from url
	token, err := tokenFromUrl(r.URL.Path, "/activate/")
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Let's check if an unactivate api key exists for this token. If not,
	// the token is not valid
	authRequest := &AuthRequest{Token: token}
	err = app.Get(authRequest)
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Create account instance with the given email address.
	acc := &Account{Email: authRequest.ApiKey.Email}

	// Fetch existing account data. It's fine if no existing data is found. In that case we'll create
	// a new entry in the database
	err = app.Get(acc)
	if err != nil && err != ErrNotFound {
		app.handleError(err, w, r)
		return
	}

	// Add the new key to the account (keys with the same device name will be replaced)
	acc.SetKey(authRequest.ApiKey)

	// Save the changes
	err = app.Put(acc)
	if err != nil {
		app.handleError(err, w, r)
	}

	// Delete the authentication request from the database
	app.Delete(authRequest)

	// Render success page
	var buff bytes.Buffer
	err = app.Templates.ConnectionSuccess.Execute(&buff, map[string]string{
		"device_name": authRequest.ApiKey.DeviceName,
	})
	if err != nil {
		app.handleError(err, w, r)
	}
	buff.WriteTo(w)
}

// Handler function for retrieving the data associated with a given account
func (app *App) GetData(w http.ResponseWriter, r *http.Request) {
	// Fetch account based on provided credentials
	acc, err := app.accountFromRequest(r)
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Retrieve data from database. If not database entry is found, the `Content` field simply stays empty.
	// This is not considered an error. Instead we simply return an empty response body. Clients should
	// know how to deal with this.
	data := &Data{Account: acc}
	err = app.Get(data)
	if err != nil && err != ErrNotFound {
		app.handleError(err, w, r)
		return
	}

	// Return raw data in response body
	w.Write(data.Content)
}

// Handler function for updating the data associated with a given account. This does NOT implement a
// diffing algorith of any kind since Padlock Cloud is completely ignorant of the data structures involved.
// Instead, clients should retrieve existing data through the `GetData` endpoint first, perform any necessary
// decryption/parsing, consolidate the data with any existing local data and then reupload the full,
// encrypted data set
func (app *App) PutData(w http.ResponseWriter, r *http.Request) {
	// Fetch account based on provided credentials
	acc, err := app.accountFromRequest(r)
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Read data from request body into `Data` instance
	data := &Data{Account: acc}
	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		app.handleError(err, w, r)
		return
	}
	data.Content = content

	// Update database entry
	err = app.Put(data)
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Return with NO CONTENT status code
	w.WriteHeader(http.StatusNoContent)
}

// Handler function for requesting a data reset for a given account
func (app *App) RequestDataReset(w http.ResponseWriter, r *http.Request) {
	// Extract email from URL
	email := r.URL.Path[1:]

	// Fetch the account for the given email address if there is one.
	err := app.Get(&Account{Email: email})
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Generate a new delete token
	token := uuid()

	// Save token/email pair in database to we can verify it later
	err = app.Put(&ResetRequest{token, email})
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Render confirmation email
	var buff bytes.Buffer
	err = app.Templates.DataResetEmail.Execute(&buff, map[string]string{
		"email":       email,
		"delete_link": fmt.Sprintf("%s://%s/reset/%s", schemeFromRequest(r), r.Host, token),
	})
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Send email with confirmation link
	body := buff.String()
	go app.Send(email, "Padlock Cloud Delete Request", body)

	// Send ACCEPTED status code
	w.WriteHeader(http.StatusAccepted)
}

// Handler function for updating the data associated with a given account
func (app *App) ResetData(w http.ResponseWriter, r *http.Request) {
	// Extract confirmation token from url
	token, err := tokenFromUrl(r.URL.Path, "/reset/")
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Fetch reset request from database
	resetRequest := &ResetRequest{Token: token}
	err = app.Get(resetRequest)
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// If the corresponding delete request was found in the database, we consider the data reset request
	// as verified so we can proceed with deleting the data for the corresponding account
	err = app.Delete(&Data{Account: &Account{Email: resetRequest.Account}})
	if err != nil {
		app.handleError(err, w, r)
		return
	}

	// Render success page
	var buff bytes.Buffer
	err = app.Templates.DataResetSuccess.Execute(&buff, map[string]string{
		"email": string(resetRequest.Account),
	})
	if err != nil {
		app.handleError(err, w, r)
		return
	}
	buff.WriteTo(w)

	// Delete the request token
	app.Delete(resetRequest)
}

// Registeres http handlers for various routes
func (app *App) setupRoutes() {
	// Endpoint for requesting api keys, only POST method is supported
	app.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			app.RequestApiKey(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	// Endpoint for activating api keys. Only GET method is supported
	app.HandleFunc("/activate/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			app.ActivateApiKey(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	// Endpoint for requesting a data reset. Only GET supported
	app.HandleFunc("/reset/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			app.ResetData(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	// Endpoint for getting/putting data. Supported methods are GET and PUT
	// DELETE method is supported if email address is provided as part of the url
	// TODO: Use POST for requesting data reset, deprecate DELETE method
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			switch r.Method {
			case "GET":
				app.GetData(w, r)
			case "PUT":
				app.PutData(w, r)
			default:
				http.Error(w, "", http.StatusMethodNotAllowed)
			}
		} else if r.Method == "DELETE" {
			app.RequestDataReset(w, r)
		} else {
			http.Error(w, "", http.StatusNotFound)
		}
	})
}

// Implements `http.Handler.ServeHTTP` interface method. Handles panic recovery and TLS checking, Delegates
// requests to embedded `http.ServeMux`
func (app *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if e := recover(); e != nil {
			log.Printf("Recovered from panic: %v", e)

			app.handleError(ErrPanic, w, r)

			if app.NotifyEmail != "" {
				go app.Send(app.NotifyEmail, "Padlock Cloud Error Notification",
					fmt.Sprintf("Recovered from panic: %v\nRequest: %v", e, r))
			}
		}
	}()

	// Only accept connections via https if `RequireTLS` configuration is true
	if app.RequireTLS && r.TLS == nil {
		app.handleError(ErrInsecureConnection, w, r)
		return
	}

	// Delegate requests to embedded `http.ServeMux`
	app.ServeMux.ServeHTTP(w, r)
}

// Initialize App with dependencies and configuration
func (app *App) Init(storage Storage, sender Sender, templates *Templates, config Config) {
	app.ServeMux = http.NewServeMux()
	app.setupRoutes()
	app.Storage = storage
	app.Sender = sender
	app.Templates = templates
	app.Config = config
}

// Start server and listen at the given address
func (app *App) Start(addr string) {
	// Open storage
	err := app.Storage.Open()
	if err != nil {
		log.Fatal(err)
	}

	// Close database connection when the method returns
	defer app.Storage.Close()

	// Start server
	err = http.ListenAndServe(addr, app)
	if err != nil {
		log.Fatal(err)
	}
}

// Does any necessary cleanup work after `App.Start()` was called
func (app *App) Stop() {
	app.Storage.Close()
}

// Instantiates and initializes a new App and returns a reference to it
func NewApp(storage Storage, sender Sender, templates *Templates, config Config) *App {
	app := &App{}
	app.Init(storage, sender, templates, config)
	return app
}
