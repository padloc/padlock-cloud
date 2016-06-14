package padlockcloud

import "net/http"
import "io/ioutil"
import "crypto/rand"
import "fmt"
import "log"
import "os"
import "encoding/json"
import "regexp"
import "errors"
import "bytes"
import "text/template"
import htmlTemplate "html/template"
import "time"
import "strconv"
import "path/filepath"

const (
	version           = 1
	defaultAssetsPath = "./assets"
	defaultPort       = 3000
)

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
func uuid() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// Extracts a uuid-formated token from a given url
func tokenFromRequest(r *http.Request) (string, error) {
	token := r.URL.Query().Get("t")

	if m, _ := regexp.MatchString(uuidPattern, token); !m {
		return "", ErrInvalidToken
	}

	return token, nil
}

// Returns the appropriate protocol based on whether a request was made via https or not
func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	} else {
		return "http"
	}
}

func versionFromRequest(r *http.Request) int {
	var vString string
	accept := r.Header.Get("Accept")

	reg := regexp.MustCompile("^application/vnd.padlock;version=(\\d)$")
	if reg.MatchString(accept) {
		vString = reg.FindStringSubmatch(accept)[1]
	} else {
		vString = r.URL.Query().Get("v")
	}

	version, _ := strconv.Atoi(vString)
	return version
}

func credentialsFromRequest(r *http.Request) (string, string) {

	// Extract email and authentication token from Authorization header
	re := regexp.MustCompile("^AuthToken (.+):(.+)$")
	authHeader := r.Header.Get("Authorization")

	// Check if the Authorization header exists and is well formed
	if !re.MatchString(authHeader) {
		return "", ""
	}

	// Extract email and auth token from Authorization header
	matches := re.FindStringSubmatch(authHeader)
	return matches[1], matches[2]
}

// A wrapper for an api key containing some meta info like the user and device name
type AuthToken struct {
	Email    string    `json:"email"`
	Token    string    `json:"token"`
	Created  time.Time `json:"-"`
	LastUsed time.Time `json:"-"`
}

// A struct representing a user with a set of api keys
type Account struct {
	// The email servers as a unique identifier and as a means for
	// requesting/activating api keys
	Email string
	// A set of api keys that can be used to access the data associated with this
	// account
	AuthTokens []AuthToken
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

// Adds an api key to this account. If an api key for the given device
// is already registered, that one will be replaced
func (a *Account) AddAuthToken(token AuthToken) {
	a.AuthTokens = append(a.AuthTokens, token)
}

// Checks if a given api key is valid for this account
func (a *Account) ValidateAuthToken(token string) bool {
	// Check if the account contains any AuthToken with that matches
	// the given key
	for _, authToken := range a.AuthTokens {
		if authToken.Token == token {
			authToken.LastUsed = time.Now()
			return true
		}
	}

	return false
}

// AuthRequest represents an api key - activation token pair used to activate a given api key
// `AuthRequest.Token` is used to activate the AuthToken through a separate channel (e.g. email)
type AuthRequest struct {
	Token     string
	AuthToken AuthToken
	Created   time.Time
}

// Implementation of the `Storable.Key` interface method
func (ar *AuthRequest) Key() []byte {
	return []byte(ar.Token)
}

// Implementation of the `Storable.Deserialize` method
func (ar *AuthRequest) Deserialize(data []byte) error {
	return json.Unmarshal(data, &ar.AuthToken)
}

// Implementation of the `Storable.Serialize` method
func (ar *AuthRequest) Serialize() ([]byte, error) {
	return json.Marshal(&ar.AuthToken)
}

// Represents a request for reseting the data associated with a given account. `RequestReset.Token` is used
// for validating the request through a separate channel (e.g. email)
type DeleteStoreRequest struct {
	Token   string
	Account string
	Created time.Time
}

// Implementation of the `Storable.Key` interface method
func (rr *DeleteStoreRequest) Key() []byte {
	return []byte(rr.Token)
}

// Implementation of the `Storable.Deserialize` interface method
func (rr *DeleteStoreRequest) Deserialize(data []byte) error {
	rr.Account = string(data)
	return nil
}

// Implementation of the `Storable.Serialize` interface method
func (rr *DeleteStoreRequest) Serialize() ([]byte, error) {
	return []byte(rr.Account), nil
}

// Store represents the data associated to a given account
type Store struct {
	Account *Account
	Content []byte
}

// Implementation of the `Storable.Key` interface method
func (d *Store) Key() []byte {
	return []byte(d.Account.Email)
}

// Implementation of the `Storable.Deserialize` interface method
func (d *Store) Deserialize(data []byte) error {
	d.Content = data
	return nil
}

// Implementation of the `Storable.Serialize` interface method
func (d *Store) Serialize() ([]byte, error) {
	return d.Content, nil
}

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

// Config contains various configuration data
type Config struct {
	// If true, all requests via plain http will be rejected. Only https requests are allowed
	RequireTLS bool
	// Email address for sending error reports; Leave empty for no notifications
	NotifyEmail string
	// Path to assets directory
	AssetsPath string
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
func (app *App) AccountFromRequest(r *http.Request) (*Account, error) {
	email, token := credentialsFromRequest(r)
	if email == "" || token == "" {
		return nil, ErrNotAuthenticated
	}
	acc := &Account{Email: email}

	// Fetch account for the given email address
	err := app.Get(acc)
	if err != nil {
		return nil, ErrNotAuthenticated
	}

	// Check if the provide api token is valid
	if !acc.ValidateAuthToken(token) {
		return nil, ErrNotAuthenticated
	}

	// Save account info to persist last used data for auth tokens
	app.Put(acc)

	return acc, nil
}

// Global error handler. Writes a appropriate response to the provided `http.ResponseWriter` object and
// logs / notifies of internal server errors
func (app *App) HandleError(e error, w http.ResponseWriter, r *http.Request) {
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
func (app *App) RequestAuthToken(w http.ResponseWriter, r *http.Request, create bool) {
	email := r.PostFormValue("email")

	// Make sure email field is set
	if email == "" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	// If the client does not explicitly state that the server should create a new account for this email
	// address in case it does not exist, we have to check if an account exists first
	if !create {
		err := app.Get(&Account{Email: email})
		if err != nil {
			app.HandleError(err, w, r)
			return
		}
	}

	// Generate key-token pair
	actToken, err := uuid()
	if err != nil {
		app.HandleError(err, w, r)
		return
	}
	authT, err := uuid()
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	authToken := AuthToken{
		email,
		authT,
		time.Now(),
		time.Now(),
	}

	// Save key-token pair to database for activating it later in a separate request
	err = app.Put(&AuthRequest{actToken, authToken, time.Now()})
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Render activation email
	var buff bytes.Buffer
	err = app.Templates.ActivateAuthTokenEmail.Execute(&buff, map[string]string{
		"email":           authToken.Email,
		"activation_link": fmt.Sprintf("%s://%s/activate/?v=%d&t=%s", schemeFromRequest(r), r.Host, version, actToken),
	})
	if err != nil {
		app.HandleError(err, w, r)
		return
	}
	body := buff.String()

	// Send email with activation link
	go app.Send(email, "Connect to Padlock Cloud", body)

	resp, err := json.Marshal(authToken)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Return auth token
	w.WriteHeader(http.StatusAccepted)
	w.Write(resp)
}

// Hander function for activating a given api key
func (app *App) ActivateAuthToken(w http.ResponseWriter, r *http.Request) {
	// Extract activation token from url
	token, err := tokenFromRequest(r)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Let's check if an unactivate api key exists for this token. If not,
	// the token is not valid
	authRequest := &AuthRequest{Token: token}
	err = app.Get(authRequest)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Create account instance with the given email address.
	acc := &Account{Email: authRequest.AuthToken.Email}

	// Fetch existing account data. It's fine if no existing data is found. In that case we'll create
	// a new entry in the database
	err = app.Get(acc)
	if err != nil && err != ErrNotFound {
		app.HandleError(err, w, r)
		return
	}

	// Add the new key to the account (keys with the same device name will be replaced)
	acc.AddAuthToken(authRequest.AuthToken)

	// Save the changes
	err = app.Put(acc)
	if err != nil {
		app.HandleError(err, w, r)
	}

	// Delete the authentication request from the database
	app.Delete(authRequest)

	// Render success page
	var buff bytes.Buffer
	err = app.Templates.ActivateAuthTokenSuccess.Execute(&buff, map[string]string{
		"email": authRequest.AuthToken.Email,
	})
	if err != nil {
		app.HandleError(err, w, r)
	}
	buff.WriteTo(w)
}

// Handler function for retrieving the data associated with a given account
func (app *App) ReadStore(w http.ResponseWriter, r *http.Request) {
	// Fetch account based on provided credentials
	acc, err := app.AccountFromRequest(r)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Retrieve data from database. If not database entry is found, the `Content` field simply stays empty.
	// This is not considered an error. Instead we simply return an empty response body. Clients should
	// know how to deal with this.
	data := &Store{Account: acc}
	err = app.Get(data)
	if err != nil && err != ErrNotFound {
		app.HandleError(err, w, r)
		return
	}

	// Return raw data in response body
	w.Write(data.Content)
}

// Handler function for updating the data associated with a given account. This does NOT implement a
// diffing algorith of any kind since Padlock Cloud is completely ignorant of the data structures involved.
// Instead, clients should retrieve existing data through the `ReadStore` endpoint first, perform any necessary
// decryption/parsing, consolidate the data with any existing local data and then reupload the full,
// encrypted data set
func (app *App) WriteStore(w http.ResponseWriter, r *http.Request) {
	// Fetch account based on provided credentials
	acc, err := app.AccountFromRequest(r)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Read data from request body into `Store` instance
	data := &Store{Account: acc}
	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}
	data.Content = content

	// Update database entry
	err = app.Put(data)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Return with NO CONTENT status code
	w.WriteHeader(http.StatusNoContent)
}

// Handler function for requesting a data reset for a given account
func (app *App) RequestDeleteStore(w http.ResponseWriter, r *http.Request) {
	// Fetch account based on provided credentials
	acc, err := app.AccountFromRequest(r)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Generate a new delete token
	token, err := uuid()
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Save token/email pair in database to we can verify it later
	err = app.Put(&DeleteStoreRequest{token, acc.Email, time.Now()})
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Render confirmation email
	var buff bytes.Buffer
	err = app.Templates.DeleteStoreEmail.Execute(&buff, map[string]string{
		"email":       acc.Email,
		"delete_link": fmt.Sprintf("%s://%s/deletestore/?v=%d&t=%s", schemeFromRequest(r), r.Host, version, token),
	})
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Send email with confirmation link
	body := buff.String()
	go app.Send(acc.Email, "Padlock Cloud Delete Request", body)

	// Send ACCEPTED status code
	w.WriteHeader(http.StatusAccepted)
}

// Handler function for updating the data associated with a given account
func (app *App) CompleteDeleteStore(w http.ResponseWriter, r *http.Request) {
	// Extract confirmation token from url
	token, err := tokenFromRequest(r)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Fetch reset request from database
	resetRequest := &DeleteStoreRequest{Token: token}
	err = app.Get(resetRequest)
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// If the corresponding delete request was found in the database, we consider the data reset request
	// as verified so we can proceed with deleting the data for the corresponding account
	err = app.Delete(&Store{Account: &Account{Email: resetRequest.Account}})
	if err != nil {
		app.HandleError(err, w, r)
		return
	}

	// Render success page
	var buff bytes.Buffer
	err = app.Templates.DeleteStoreSuccess.Execute(&buff, map[string]string{
		"email": string(resetRequest.Account),
	})
	if err != nil {
		app.HandleError(err, w, r)
		return
	}
	buff.WriteTo(w)

	// Delete the request token
	app.Delete(resetRequest)
}

// Registeres http handlers for various routes
func (app *App) SetupRoutes() {
	// Endpoint for requesting api keys, only POST method is supported
	app.HandleFunc("/auth/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PUT":
			app.RequestAuthToken(w, r, false)
		case "POST":
			app.RequestAuthToken(w, r, true)
		default:
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	// Endpoint for requesting api keys, only POST method is supported
	app.HandleFunc("/activate/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			app.ActivateAuthToken(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	// Endpoint for reading, writing and deleting store data
	app.HandleFunc("/store/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET", "HEAD":
			app.ReadStore(w, r)
		case "PUT":
			app.WriteStore(w, r)
		case "DELETE":
			app.RequestDeleteStore(w, r)
		default:
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	// Endpoint for requesting a data reset. Only GET supported
	app.HandleFunc("/deletestore/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			app.CompleteDeleteStore(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	// Endpoint for requesting a data reset. Only GET supported
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusNotFound)
	})
}

func (app *App) DeprecatedVersion(w http.ResponseWriter, r *http.Request) {
	// Try getting email from Authorization header first
	email, _ := credentialsFromRequest(r)

	// Try to extract email from url if method is DELETE
	if email == "" && r.Method == "DELETE" {
		email = r.URL.Path[1:]
	}

	// Try to get email from request body if method is POST
	if email == "" && r.Method == "POST" {
		email = r.PostFormValue("email")
	}

	if email != "" {
		// Render activation email
		var buff bytes.Buffer
		err := app.Templates.DeprecatedVersionEmail.Execute(&buff, nil)
		if err != nil {
			app.HandleError(err, w, r)
			return
		}
		body := buff.String()

		// Send email with activation link
		go app.Send(email, "Please update your version of Padlock", body)
	}

	http.Error(w, "", http.StatusNotAcceptable)
}

func (app *App) HandlePanic(w http.ResponseWriter, r *http.Request) {
	if e := recover(); e != nil {
		log.Printf("Recovered from panic: %v", e)

		app.HandleError(ErrPanic, w, r)

		if app.NotifyEmail != "" {
			go app.Send(app.NotifyEmail, "Padlock Cloud Error Notification",
				fmt.Sprintf("Recovered from panic: %v\nRequest: %v", e, r))
		}
	}
}

func (app *App) CheckVersion(w http.ResponseWriter, r *http.Request) bool {
	return versionFromRequest(r) != version
}

// Implements `http.Handler.ServeHTTP` interface method. Handles panic recovery and TLS checking, Delegates
// requests to embedded `http.ServeMux`
func (app *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	defer app.HandlePanic(w, r)

	// Only accept connections via https if `RequireTLS` configuration is true
	if app.RequireTLS && r.TLS == nil {
		app.HandleError(ErrInsecureConnection, w, r)
		return
	}

	if app.CheckVersion(w, r) {
		app.DeprecatedVersion(w, r)
		return
	}

	// Delegate requests to embedded `http.ServeMux`
	app.ServeMux.ServeHTTP(w, r)
}

// Initialize App with dependencies and configuration
func (app *App) Init(storage Storage, sender Sender, templates *Templates, config Config) {
	app.ServeMux = http.NewServeMux()
	app.SetupRoutes()
	app.Storage = storage
	app.Sender = sender
	app.Templates = templates
	app.Config = config
	app.LoadEnv()
}

// Loads settings from environment variables
func (app *App) LoadEnv() {
	app.Sender.LoadEnv()
	app.Storage.LoadEnv()
	if app.AssetsPath == "" {
		app.AssetsPath = os.Getenv("PADLOCK_ASSETS_PATH")
	}
	if app.AssetsPath == "" {
		app.AssetsPath = defaultAssetsPath
	}
	if app.NotifyEmail == "" {
		app.NotifyEmail = os.Getenv("PADLOCK_NOTIFY_EMAIL")
	}
}

// Loads templates from given directory
func (app *App) LoadTemplatesFromAssets() {
	path := filepath.Join(app.AssetsPath, "templates")
	app.Templates = &Templates{
		template.Must(template.ParseFiles(filepath.Join(path, "activate.txt"))),
		template.Must(template.ParseFiles(filepath.Join(path, "delete.txt"))),
		htmlTemplate.Must(htmlTemplate.ParseFiles(filepath.Join(path, "connected.html"))),
		htmlTemplate.Must(htmlTemplate.ParseFiles(filepath.Join(path, "deleted.html"))),
		template.Must(template.ParseFiles(filepath.Join(path, "deprecated.txt"))),
	}
}

// Instantiates and initializes a new App and returns a reference to it
func NewApp(storage Storage, sender Sender, templates *Templates, config Config) *App {
	app := &App{}
	app.Init(storage, sender, templates, config)
	return app
}
