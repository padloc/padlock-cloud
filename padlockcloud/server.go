package padlockcloud

import "net"
import "net/http"
import "net/http/httputil"
import "io/ioutil"
import "fmt"
import "encoding/json"
import "regexp"
import "errors"
import "bytes"
import "time"
import "strconv"
import "path/filepath"
import "strings"
import "gopkg.in/tylerb/graceful.v1"

const (
	ApiVersion = 1
)

// Extracts a uuid-formated token from a given url
func tokenFromRequest(r *http.Request) (string, error) {
	token := r.URL.Query().Get("t")

	if token == "" {
		return "", &InvalidToken{token}
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
		vString = r.PostFormValue("api_version")
	}

	if vString == "" {
		vString = r.URL.Query().Get("v")
	}

	version, _ := strconv.Atoi(vString)
	return version
}

func getIp(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.RemoteAddr
	}
	return ip
}

func formatRequest(r *http.Request) string {
	return fmt.Sprintf("%s %s %s", getIp(r), r.Method, r.URL)
}

func formatRequestVerbose(r *http.Request) string {
	dump, _ := httputil.DumpRequest(r, true)
	return string(dump)
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
	return json.Unmarshal(data, rr)
}

// Implementation of the `Storable.Serialize` interface method
func (rr *DeleteStoreRequest) Serialize() ([]byte, error) {
	return json.Marshal(rr)
}

// Creates a new `DeleteStoreRequest` with a given `email`
func NewDeleteStoreRequest(email string) (*DeleteStoreRequest, error) {
	// Generate a new delete token
	token, err := token()
	if err != nil {
		return nil, err
	}
	return &DeleteStoreRequest{token, email, time.Now()}, nil
}

// DataStore represents the data associated to a given account
type DataStore struct {
	Account *Account
	Content []byte
}

// Implementation of the `Storable.Key` interface method
func (d *DataStore) Key() []byte {
	return []byte(d.Account.Email)
}

// Implementation of the `Storable.Deserialize` interface method
func (d *DataStore) Deserialize(data []byte) error {
	d.Content = data
	return nil
}

// Implementation of the `Storable.Serialize` interface method
func (d *DataStore) Serialize() ([]byte, error) {
	return d.Content, nil
}

// Server configuration
type ServerConfig struct {
	// Path to assets directory; used for loading templates and such
	AssetsPath string `yaml:"assets_path"`
	// Port to listen on
	Port int `yaml:"port"`
	// Path to TLS certificate
	TLSCert string `yaml:"tls_cert"`
	// Path to TLS key file
	TLSKey string `yaml:"tls_key"`
	// Explicit host to use in place of http.Request::Host when generating urls and such
	Host string `yaml:"host"`
}

// The Server type holds all the contextual data and logic used for running a Padlock Cloud instances
// Users should use the `NewServer` function to instantiate an `Server` instance
type Server struct {
	*graceful.Server
	*Log
	mux       *http.ServeMux
	Listener  net.Listener
	Storage   Storage
	Sender    Sender
	Templates *Templates
	Config    *ServerConfig
}

func (server *Server) GetHost(r *http.Request) string {
	if server.Config.Host != "" {
		return server.Config.Host
	} else {
		return r.Host
	}
}

// Retreives Account object from a http.Request object by evaluating the Authorization header and
// cross-checking it with api keys of existing accounts. Returns an `Unauthorized` error
// if no valid Authorization header is provided or if the provided email:api_key pair does not match
// any of the accounts in the database.
func (server *Server) AccountFromRequest(r *http.Request) (*Account, error) {
	authToken, err := AuthTokenFromRequest(r)
	if err != nil {
		return nil, &Unauthorized{}
	}

	acc := &Account{Email: authToken.Email}

	// Fetch account for the given email address
	if err := server.Storage.Get(acc); err != nil {
		if err == ErrNotFound {
			return nil, &Unauthorized{authToken.Email, authToken.Token}
		} else {
			return nil, err
		}
	}

	// Check if the provide api token is valid
	if t := acc.AuthToken(authToken); t != nil {
		t.LastUsed = time.Now()

		// Save account info to persist last used data for auth tokens
		if err := server.Storage.Put(acc); err != nil {
			return nil, err
		}
	} else {
		return nil, &Unauthorized{authToken.Email, authToken.Token}
	}

	return acc, nil
}

func (server *Server) LogError(err error, r *http.Request) {
	if _, ok := err.(*ServerError); ok {
		server.Error.Printf("%s - %v\nRequest:\n%s\n", formatRequest(r), err, formatRequestVerbose(r))
	} else {
		server.Info.Printf("%s - %v", formatRequest(r), err)
	}
}

func (server *Server) CheckApiVersion(r *http.Request) error {
	version := versionFromRequest(r)
	if version != ApiVersion {
		if err := server.SendDeprecatedVersionEmail(r); err != nil {
			server.LogError(&ServerError{err}, r)
		}
		return &UnsupportedApiVersion{version}
	}

	return nil
}

// Global error handler. Writes a appropriate response to the provided `http.ResponseWriter` object and
// logs / notifies of internal server errors
func (server *Server) HandleError(e error, w http.ResponseWriter, r *http.Request) {
	err, ok := e.(ErrorResponse)

	if !ok {
		err = &ServerError{e}
	}

	server.LogError(err, r)

	var response []byte
	accept := r.Header.Get("Accept")

	if accept == "application/json" || strings.HasPrefix(accept, "application/vnd.padlock") {
		w.Header().Set("Content-Type", "application/json")
		response = JsonifyErrorResponse(err)
	} else if strings.Contains(accept, "text/html") {
		w.Header().Set("Content-Type", "text/html")
		// Render success page
		var buff bytes.Buffer
		if err := server.Templates.ErrorPage.Execute(&buff, map[string]string{
			"message": err.Message(),
		}); err != nil {
			server.LogError(&ServerError{err}, r)
		} else {
			response = buff.Bytes()
		}
	}

	if response == nil {
		response = []byte(err.Message())
	}

	w.WriteHeader(err.Status())
	w.Write(response)
}

// Handler function for requesting an api key. Generates a key-token pair and stores them.
// The token can later be used to activate the api key. An email is sent to the corresponding
// email address with an activation url. Expects `email` and `device_name` parameters through either
// multipart/form-data or application/x-www-urlencoded parameters
func (server *Server) RequestAuthToken(w http.ResponseWriter, r *http.Request, create bool) error {
	if err := server.CheckApiVersion(r); err != nil {
		return err
	}

	email := r.PostFormValue("email")
	tType := r.PostFormValue("type")
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

	// If the client does not explicitly state that the server should create a new account for this email
	// address in case it does not exist, we have to check if an account exists first
	if !create {
		acc := &Account{Email: email}
		if err := server.Storage.Get(acc); err != nil {
			if err == ErrNotFound {
				// See if there exists a data store for this account
				if err := server.Storage.Get(&DataStore{Account: acc}); err != nil {
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

	// Save key-token pair to database for activating it later in a separate request
	err = server.Storage.Put(authRequest)
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
		if err := server.Templates.LoginPage.Execute(&buff, map[string]interface{}{
			"submitted": true,
			"email":     email,
		}); err != nil {
			return err
		}

		response = buff.Bytes()
		emailSubj = "Log in to Padlock Cloud"

		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    authRequest.AuthToken.String(),
			Domain:   server.GetHost(r),
			HttpOnly: true,
			Path:     "/",
		})

		w.Header().Set("Content-Type", "text/html")
	}

	// Render activation email
	if err := server.Templates.ActivateAuthTokenEmail.Execute(&emailBody, map[string]interface{}{
		"activation_link": fmt.Sprintf("%s://%s/activate/?v=%d&t=%s", schemeFromRequest(r),
			server.GetHost(r), ApiVersion, authRequest.Token),
		"token": authRequest.AuthToken,
	}); err != nil {
		return err
	}

	// Send email with activation link
	go func() {
		if err := server.Sender.Send(email, emailSubj, emailBody.String()); err != nil {
			server.LogError(&ServerError{err}, r)
		}
	}()

	server.Info.Printf("%s - auth_token:request - %s:%s:%s\n", formatRequest(r), email, tType, authRequest.AuthToken.Id)

	w.WriteHeader(http.StatusAccepted)
	w.Write(response)

	return nil
}

// Hander function for activating a given api key
func (server *Server) ActivateAuthToken(w http.ResponseWriter, r *http.Request) error {
	// Extract activation token from url
	token, err := tokenFromRequest(r)
	if err != nil {
		return err
	}

	// Let's check if an unactivate api key exists for this token. If not,
	// the token is not valid
	authRequest := &AuthRequest{Token: token}
	if err := server.Storage.Get(authRequest); err != nil {
		if err == ErrNotFound {
			return &InvalidToken{token}
		} else {
			return err
		}
	}

	authToken := authRequest.AuthToken

	// Make sure that this is the same browser that the login was requested with
	if authToken.Type == "web" {
		if t, err := AuthTokenFromRequest(r); err != nil || t.Token != authToken.Token {
			return &InvalidToken{token}
		}
	}

	// Create account instance with the given email address.
	acc := &Account{Email: authToken.Email}

	// Fetch existing account data. It's fine if no existing data is found. In that case we'll create
	// a new entry in the database
	if err := server.Storage.Get(acc); err != nil && err != ErrNotFound {
		return err
	}

	// Add the new key to the account
	acc.AddAuthToken(&authToken)

	// Save the changes
	if err := server.Storage.Put(acc); err != nil {
		return err
	}

	// Delete the authentication request from the database
	if err := server.Storage.Delete(authRequest); err != nil {
		return err
	}

	server.Info.Printf("%s - auth_token:activate - %s:%s:%s\n", formatRequest(r), acc.Email, authToken.Type, authToken.Id)

	if authToken.Type == "web" {
		http.Redirect(w, r, "/dashboard/", http.StatusFound)
	} else {
		var b bytes.Buffer
		if err := server.Templates.ActivateAuthTokenSuccess.Execute(&b, map[string]interface{}{
			"token": authToken,
		}); err != nil {
			return err
		}
		b.WriteTo(w)
	}

	return nil
}

// Handler function for retrieving the data associated with a given account
func (server *Server) ReadStore(w http.ResponseWriter, r *http.Request) error {
	if err := server.CheckApiVersion(r); err != nil {
		return err
	}

	// Fetch account based on provided credentials
	acc, err := server.AccountFromRequest(r)
	if err != nil {
		return err
	}

	// Retrieve data from database. If not database entry is found, the `Content` field simply stays empty.
	// This is not considered an error. Instead we simply return an empty response body. Clients should
	// know how to deal with this.
	data := &DataStore{Account: acc}
	if err := server.Storage.Get(data); err != nil && err != ErrNotFound {
		return err
	}

	server.Info.Printf("%s - data_store:read - %s\n", formatRequest(r), acc.Email)

	// Return raw data in response body
	w.Write(data.Content)

	return nil
}

// Handler function for updating the data associated with a given account. This does NOT implement a
// diffing algorith of any kind since Padlock Cloud is completely ignorant of the data structures involved.
// Instead, clients should retrieve existing data through the `ReadStore` endpoint first, perform any necessary
// decryption/parsing, consolidate the data with any existing local data and then reupload the full,
// encrypted data set
func (server *Server) WriteStore(w http.ResponseWriter, r *http.Request) error {
	if err := server.CheckApiVersion(r); err != nil {
		return err
	}

	// Fetch account based on provided credentials
	acc, err := server.AccountFromRequest(r)
	if err != nil {
		return err
	}

	// Read data from request body into `DataStore` instance
	data := &DataStore{Account: acc}
	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	data.Content = content

	// Update database entry
	if err := server.Storage.Put(data); err != nil {
		return err
	}

	server.Info.Printf("%s - data_store:write - %s\n", formatRequest(r), acc.Email)

	// Return with NO CONTENT status code
	w.WriteHeader(http.StatusNoContent)

	return nil
}

// Handler function for requesting a data reset for a given account
func (server *Server) RequestDeleteStore(w http.ResponseWriter, r *http.Request) error {
	if err := server.CheckApiVersion(r); err != nil {
		return err
	}

	// Fetch account based on provided credentials
	acc, err := server.AccountFromRequest(r)
	if err != nil {
		return err
	}

	// Create DeleteStoreRequest
	deleteRequest, err := NewDeleteStoreRequest(acc.Email)
	if err != nil {
		return err
	}

	// Save token/email pair in database to we can verify it later
	if err := server.Storage.Put(deleteRequest); err != nil {
		return err
	}

	// Render confirmation email
	var buff bytes.Buffer
	if err := server.Templates.DeleteStoreEmail.Execute(&buff, map[string]string{
		"email": acc.Email,
		"delete_link": fmt.Sprintf("%s://%s/deletestore/?v=%d&t=%s", schemeFromRequest(r),
			server.GetHost(r), ApiVersion, deleteRequest.Token),
	}); err != nil {
		return err
	}

	body := buff.String()

	// Send email with confirmation link
	go func() {
		if err := server.Sender.Send(acc.Email, "Padlock Cloud Delete Request", body); err != nil {
			server.LogError(&ServerError{err}, r)
		}
	}()

	server.Info.Printf("%s - data_store:request_delete - %s", formatRequest(r), acc.Email)

	// Send ACCEPTED status code
	w.WriteHeader(http.StatusAccepted)

	return nil
}

// Handler function for updating the data associated with a given account
func (server *Server) CompleteDeleteStore(w http.ResponseWriter, r *http.Request) error {
	// Extract confirmation token from url
	token, err := tokenFromRequest(r)
	if err != nil {
		return err
	}

	// Fetch reset request from database
	resetRequest := &DeleteStoreRequest{Token: token}
	if err := server.Storage.Get(resetRequest); err != nil {
		if err == ErrNotFound {
			return &InvalidToken{token}
		} else {
			return err
		}
	}

	// If the corresponding delete request was found in the database, we consider the data reset request
	// as verified so we can proceed with deleting the data for the corresponding account
	dataStore := &DataStore{Account: &Account{Email: resetRequest.Account}}
	if err := server.Storage.Delete(dataStore); err != nil {
		return err
	}

	// Render success page
	var buff bytes.Buffer
	if err := server.Templates.DeleteStoreSuccess.Execute(&buff, map[string]string{
		"email": string(resetRequest.Account),
	}); err != nil {
		return err
	}

	// Delete the request token
	if err := server.Storage.Delete(resetRequest); err != nil {
		return err
	}

	server.Info.Printf("%s - data_store:confirm_delete - %s", formatRequest(r), resetRequest.Account)

	buff.WriteTo(w)

	return nil
}

func (server *Server) LoginPage(w http.ResponseWriter, r *http.Request) error {
	var b bytes.Buffer
	if err := server.Templates.LoginPage.Execute(&b, map[string]string{}); err != nil {
		return err
	}

	b.WriteTo(w)

	return nil
}

func (server *Server) Dashboard(w http.ResponseWriter, r *http.Request) error {
	acc, err := server.AccountFromRequest(r)
	if err != nil {
		server.LogError(err, r)
		http.Redirect(w, r, "/auth/", http.StatusFound)
		return nil
	}

	var b bytes.Buffer
	if err := server.Templates.Dashboard.Execute(&b, map[string]interface{}{
		"account": acc,
	}); err != nil {
		return err
	}

	b.WriteTo(w)
	return nil
}

func (server *Server) Logout(w http.ResponseWriter, r *http.Request) error {
	if acc, err := server.AccountFromRequest(r); err == nil {
		authToken, _ := AuthTokenFromRequest(r)
		acc.RemoveAuthToken(authToken)
		if err := server.Storage.Put(acc); err != nil {
			return err
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "auth",
		Value:  "",
		MaxAge: -1,
		Path:   "/",
	})
	http.Redirect(w, r, "/auth/", http.StatusFound)
	return nil
}

// Registeres http handlers for various routes
func (server *Server) SetupRoutes() {
	// Endpoint for requesting api keys, only POST method is supported
	server.mux.HandleFunc("/auth/", func(w http.ResponseWriter, r *http.Request) {
		var err error
		switch r.Method {
		case "PUT":
			err = server.RequestAuthToken(w, r, false)
		case "POST":
			err = server.RequestAuthToken(w, r, true)
		default:
			err = server.LoginPage(w, r)
		}

		if err != nil {
			server.HandleError(err, w, r)
		}
	})

	// Endpoint for requesting api keys, only POST method is supported
	server.mux.HandleFunc("/activate/", func(w http.ResponseWriter, r *http.Request) {
		var err error

		if r.Method == "GET" {
			err = server.ActivateAuthToken(w, r)
		} else {
			err = &MethodNotAllowed{r.Method}
		}

		if err != nil {
			server.HandleError(err, w, r)
		}
	})

	// Endpoint for reading, writing and deleting store data
	server.mux.HandleFunc("/store/", func(w http.ResponseWriter, r *http.Request) {
		var err error

		switch r.Method {
		case "GET", "HEAD":
			err = server.ReadStore(w, r)
		case "PUT":
			err = server.WriteStore(w, r)
		case "DELETE":
			err = server.RequestDeleteStore(w, r)
		default:
			err = &MethodNotAllowed{r.Method}
		}

		if err != nil {
			server.HandleError(err, w, r)
		}
	})

	// Endpoint for requesting a data reset. Only GET supported
	server.mux.HandleFunc("/deletestore/", func(w http.ResponseWriter, r *http.Request) {
		var err error

		if r.Method == "GET" {
			err = server.CompleteDeleteStore(w, r)
		} else {
			err = &MethodNotAllowed{r.Method}
		}

		if err != nil {
			server.HandleError(err, w, r)
		}
	})

	server.mux.HandleFunc("/dashboard/", func(w http.ResponseWriter, r *http.Request) {
		var err error

		if r.Method == "GET" {
			err = server.Dashboard(w, r)
		} else {
			err = &MethodNotAllowed{r.Method}
		}

		if err != nil {
			server.HandleError(err, w, r)
		}
	})

	server.mux.HandleFunc("/logout/", func(w http.ResponseWriter, r *http.Request) {
		var err error

		if r.Method == "GET" {
			err = server.Logout(w, r)
		} else {
			err = &MethodNotAllowed{r.Method}
		}

		if err != nil {
			server.HandleError(err, w, r)
		}
	})

	fs := http.FileServer(http.Dir(filepath.Join(server.Config.AssetsPath, "static")))
	server.mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Fall through route
	server.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server.HandleError(&UnsupportedEndpoint{r.URL.Path}, w, r)
	})
}

func (server *Server) SendDeprecatedVersionEmail(r *http.Request) error {
	var email string

	// Try getting email from Authorization header first
	if authToken, err := AuthTokenFromRequest(r); err == nil {
		email = authToken.Email
	}

	// Try to extract email from url if method is DELETE
	if email == "" && r.Method == "DELETE" {
		email = r.URL.Path[1:]
	}

	// Try to get email from request body if method is POST
	if email == "" && (r.Method == "PUT" || r.Method == "POST") {
		email = r.PostFormValue("email")
	}

	if email != "" {
		var buff bytes.Buffer
		if err := server.Templates.DeprecatedVersionEmail.Execute(&buff, nil); err != nil {
			return err
		}
		body := buff.String()

		// Send email about deprecated api version
		go func() {
			if err := server.Sender.Send(email, "Please update your version of Padlock", body); err != nil {
				server.LogError(&ServerError{err}, r)
			}
		}()
	}

	return nil
}

func (server *Server) HandlePanic(w http.ResponseWriter, r *http.Request) {
	if e := recover(); e != nil {
		err, ok := e.(error)
		if !ok {
			err = errors.New(fmt.Sprintf("%v", e))
		}
		server.HandleError(err, w, r)
	}
}

// Implements `http.Handler.ServeHTTP` interface method. Handles panic recovery, Delegates
// requests to embedded `http.ServeMux`
func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer server.HandlePanic(w, r)

	// Delegate requests to embedded `http.ServeMux`
	server.mux.ServeHTTP(w, r)
}

// Initialize Server with dependencies and configuration
func (server *Server) Init() error {
	var err error

	server.SetupRoutes()

	if server.Templates == nil {
		// Load templates from assets directory
		server.Templates, err = LoadTemplates(filepath.Join(server.Config.AssetsPath, "templates"))
		if err != nil {
			return err
		}
	}

	// Open storage
	if err = server.Storage.Open(); err != nil {
		return err
	}

	return nil
}

func (server *Server) CleanUp() error {
	return server.Storage.Close()
}

func (server *Server) Start() error {
	if err := server.Init(); err != nil {
		return err
	}
	defer server.CleanUp()

	// Add rate limiting middleWare
	handler := RateLimit(server, map[Route]RateQuota{
		Route{"POST", "/auth/"}:    RateQuota{PerMin(1), 5},
		Route{"PUT", "/auth/"}:     RateQuota{PerMin(1), 5},
		Route{"DELETE", "/store/"}: RateQuota{PerMin(1), 5},
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.HandleError(&TooManyRequests{}, w, r)
	}))

	// Add CORS middleware
	handler = Cors(handler)

	server.Handler = handler

	port := server.Config.Port
	tlsCert := server.Config.TLSCert
	tlsKey := server.Config.TLSKey

	server.Addr = fmt.Sprintf(":%d", port)

	// Start server
	if tlsCert != "" && tlsKey != "" {
		server.Info.Printf("Starting server with TLS on port %v", port)
		return server.ListenAndServeTLS(tlsCert, tlsKey)
	} else {
		server.Info.Printf("Starting server on port %v", port)
		return server.ListenAndServe()
	}
}

// Instantiates and initializes a new Server and returns a reference to it
func NewServer(log *Log, storage Storage, sender Sender, config *ServerConfig) *Server {
	server := &Server{
		&graceful.Server{
			Server:  &http.Server{},
			Timeout: time.Second * 10,
		},
		log,
		http.NewServeMux(),
		nil,
		storage,
		sender,
		nil,
		config,
	}

	// Hook up logger for http.Server
	server.ErrorLog = server.Error
	// Hook up logger for graceful.Server
	server.Logger = server.Error

	return server
}

func init() {
	RegisterStorable(&DataStore{}, "data-stores")
	RegisterStorable(&DeleteStoreRequest{}, "delete-requests")
}
