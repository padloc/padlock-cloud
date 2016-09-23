package padlockcloud

import "net"
import "net/http"
import "net/http/httputil"
import "io/ioutil"
import "fmt"
import "encoding/json"
import "encoding/base64"
import "regexp"
import "errors"
import "bytes"
import "time"
import "strconv"
import "path/filepath"
import "strings"
import "gopkg.in/tylerb/graceful.v1"
import "github.com/gorilla/csrf"

const (
	ApiVersion = 1
)

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
	// Secret used for authenticating cookies
	Secret string `yaml:"secret"`
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
	Secure    bool
	secret    []byte
	endpoints map[string]*Endpoint
}

func (server *Server) GetHost(r *http.Request) string {
	if server.Config.Host != "" {
		return server.Config.Host
	} else {
		return r.Host
	}
}

// Retreives Account object from a http.Request object by evaluating the Authorization header and
// cross-checking it with api keys of existing accounts. Returns an `InvalidAuthToken` error
// if no valid Authorization header is provided or if the provided email:api_key pair does not match
// any of the accounts in the database.
func (server *Server) Authenticate(r *http.Request) (*AuthToken, error) {
	authToken, err := AuthTokenFromRequest(r)
	if err != nil {
		return nil, &InvalidAuthToken{}
	}

	invalidErr := &InvalidAuthToken{authToken.Email, authToken.Token}

	acc := &Account{Email: authToken.Email}

	// Fetch account for the given email address
	if err := server.Storage.Get(acc); err != nil {
		if err == ErrNotFound {
			return nil, invalidErr
		} else {
			return nil, err
		}
	}

	// Find the fully populated auth token struct on account. If not found, the value will be nil
	// and we know that the provided token is not valid
	if !authToken.Validate(acc) {
		return nil, invalidErr
	}

	// Check if the token is expired
	if authToken.Expired() {
		return nil, &ExpiredAuthToken{authToken.Email, authToken.Token}
	}

	// If everything checks out, update the `LastUsed` field with the current time
	authToken.LastUsed = time.Now()

	acc.UpdateAuthToken(authToken)

	// Save account info to persist last used data for auth tokens
	if err := server.Storage.Put(acc); err != nil {
		return nil, err
	}

	return authToken, nil
}

func (server *Server) LogError(err error, r *http.Request) {
	switch e := err.(type) {
	case *ServerError, *InvalidCsrfToken:
		server.Error.Printf("%s - %v\nRequest:\n%s\n", formatRequest(r), e, formatRequestVerbose(r))
	default:
		server.Info.Printf("%s - %v", formatRequest(r), e)
	}
}

func (server *Server) CheckEndpointVersion(r *http.Request, v int) error {
	if v == 0 {
		return nil
	}

	version := versionFromRequest(r)
	if version != v {
		if err := server.SendDeprecatedVersionEmail(r); err != nil {
			server.LogError(&ServerError{err}, r)
		}
		return &UnsupportedApiVersion{version, v}
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
func (server *Server) RequestAuthToken(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
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

	if redirect != "" && server.endpoints[redirect] == nil {
		return &BadRequest{"invalid redirect path"}
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

	authRequest.Redirect = redirect

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
func (server *Server) ActivateAuthToken(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	token := r.URL.Query().Get("t")

	if token == "" {
		return &BadRequest{"no activation token provided"}
	}

	// Let's check if an unactivate api key exists for this token. If not,
	// the token is not valid
	authRequest := &AuthRequest{Token: token}
	if err := server.Storage.Get(authRequest); err != nil {
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
	if err := server.Storage.Get(acc); err != nil && err != ErrNotFound {
		return err
	}

	// Add the new key to the account
	acc.AddAuthToken(authToken)

	// Save the changes
	if err := server.Storage.Put(acc); err != nil {
		return err
	}

	// Delete the authentication request from the database
	if err := server.Storage.Delete(authRequest); err != nil {
		return err
	}

	redirect := authRequest.Redirect

	if authToken.Type == "web" {
		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    authToken.String(),
			HttpOnly: true,
			Path:     "/",
			Secure:   server.Secure,
		})

		if redirect == "" {
			redirect = "/dashboard/"
		}
	}

	if redirect != "" {
		http.Redirect(w, r, redirect, http.StatusFound)
	} else {
		var b bytes.Buffer
		if err := server.Templates.ActivateAuthTokenSuccess.Execute(&b, map[string]interface{}{
			"token": authToken,
		}); err != nil {
			return err
		}
		b.WriteTo(w)
	}

	server.Info.Printf("%s - auth_token:activate - %s:%s:%s\n", formatRequest(r), acc.Email, authToken.Type, authToken.Id)

	return nil
}

// Handler function for retrieving the data associated with a given account
func (server *Server) ReadStore(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

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
func (server *Server) WriteStore(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

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

func (server *Server) DeleteStore(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	deleted := false

	if r.Method == "POST" {
		if err := server.Storage.Delete(&DataStore{Account: acc}); err != nil {
			return err
		}
		deleted = true
	}

	var b bytes.Buffer
	if err := server.Templates.DeleteStore.Execute(&b, map[string]interface{}{
		"account":        acc,
		"deleted":        deleted,
		csrf.TemplateTag: csrf.TemplateField(r),
	}); err != nil {
		return err
	}

	b.WriteTo(w)
	return nil
}

// Handler function for requesting a data reset for a given account
func (server *Server) RequestDeleteStore(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	// Create AuthRequest
	authRequest, err := NewAuthRequest(acc.Email, "web")
	if err != nil {
		return err
	}

	// After logging in, redirect to delete store page
	authRequest.Redirect = "/deletestore/"

	// Save authrequest
	if err := server.Storage.Put(authRequest); err != nil {
		return err
	}

	// Render confirmation email
	var buff bytes.Buffer
	if err := server.Templates.ActivateAuthTokenEmail.Execute(&buff, map[string]string{
		"email": acc.Email,
		"activation_link": fmt.Sprintf("%s://%s/activate/?v=%d&t=%s", schemeFromRequest(r),
			server.GetHost(r), ApiVersion, authRequest.Token),
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

func (server *Server) LoginPage(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	var b bytes.Buffer
	if err := server.Templates.LoginPage.Execute(&b, nil); err != nil {
		return err
	}

	b.WriteTo(w)

	return nil
}

func (server *Server) Dashboard(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	var b bytes.Buffer
	if err := server.Templates.Dashboard.Execute(&b, map[string]interface{}{
		"account":        acc,
		csrf.TemplateTag: csrf.TemplateField(r),
	}); err != nil {
		return err
	}

	b.WriteTo(w)
	return nil
}

func (server *Server) Logout(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	acc := auth.Account()

	acc.RemoveAuthToken(auth)
	if err := server.Storage.Put(acc); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		Secure:   server.Secure,
	})
	http.Redirect(w, r, "/login/", http.StatusFound)
	return nil
}

func (server *Server) Revoke(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
	token := r.PostFormValue("token")
	id := r.PostFormValue("id")
	if token == "" && id == "" {
		return &BadRequest{"No token or id provided"}
	}

	acc := auth.Account()

	t := &AuthToken{Token: token, Id: id}
	if !t.Validate(acc) {
		return &BadRequest{"No such token"}
	}

	t.Expires = time.Now().Add(-time.Minute)

	acc.UpdateAuthToken(t)

	if err := server.Storage.Put(acc); err != nil {
		return err
	}

	http.Redirect(w, r, "/dashboard/", http.StatusFound)

	return nil
}

func (server *Server) CSRF(h http.Handler) http.Handler {
	errorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.HandleError(&InvalidCsrfToken{csrf.FailureReason(r)}, w, r)
	})
	return csrf.Protect(
		server.secret,
		csrf.Path("/"),
		csrf.Secure(server.Secure),
		csrf.ErrorHandler(errorHandler),
	)(h)
}

type MethodFuncs map[string]func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error

type Endpoint struct {
	Path     string
	Handlers MethodFuncs
	Version  int
	AuthType string
}

// Registers handlers mapped by method for a given path
func (server *Server) Route(endpoint *Endpoint) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := func() error {
			fn := endpoint.Handlers[r.Method]

			// If no function for this method was found, return MethodNotAllowed error
			if fn == nil {
				return &MethodNotAllowed{r.Method}
			}

			// Check correct endpoint version
			if err := server.CheckEndpointVersion(r, endpoint.Version); err != nil {
				return err
			}

			// Get auth token from request
			auth, authErr := server.Authenticate(r)

			// Endpoint requires authentation but no auth token could be aquired
			if endpoint.AuthType != "" && authErr != nil {
				// If this endpoint requires web authentication, simply redirect to login page
				if endpoint.AuthType == "web" {
					http.Redirect(w, r, "/login/", http.StatusFound)
					return nil
				}

				return authErr
			}

			// Make sure auth token has the right type
			if endpoint.AuthType != "" && auth.Type != endpoint.AuthType {
				return &InvalidAuthToken{auth.Email, auth.Token}
			}

			// Wrap the handler function in a http.HandlerFunc; Capture error in `e` variable for
			// later use. We need to do this because the csrf middleware only works with a http.Handler
			var e error
			var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				e = fn(w, r, auth)
			})

			// If auth type is "web", wrap handler in csrf middleware
			if endpoint.AuthType != "" && auth != nil && auth.Type == "web" {
				h = server.CSRF(h)
			}

			// Execute handler. If our original handler function returns an error, it will
			// be captures in the `e` variable
			h.ServeHTTP(w, r)

			return e
		}(); err != nil {
			server.HandleError(err, w, r)
		}
	})

	server.mux.Handle(endpoint.Path, handler)

	server.endpoints[endpoint.Path] = endpoint
}

// Registeres http handlers for various routes
func (server *Server) SetupRoutes() {
	// Endpoint for logging in / requesting api keys
	server.Route(&Endpoint{
		Path: "/auth/",
		Handlers: MethodFuncs{
			"PUT":  server.RequestAuthToken,
			"POST": server.RequestAuthToken,
		},
		Version: ApiVersion,
	})

	// Endpoint for logging in / requesting api keys
	server.Route(&Endpoint{
		Path: "/login/",
		Handlers: MethodFuncs{
			"GET":  server.LoginPage,
			"POST": server.RequestAuthToken,
		},
	})

	// Endpoint for activating auth tokens
	server.Route(&Endpoint{
		Path: "/activate/",
		Handlers: MethodFuncs{
			"GET": server.ActivateAuthToken,
		},
	})

	// Endpoint for reading / writing and deleting a store
	server.Route(&Endpoint{
		Path: "/store/",
		Handlers: MethodFuncs{
			"GET":    server.ReadStore,
			"HEAD":   server.ReadStore,
			"PUT":    server.WriteStore,
			"DELETE": server.RequestDeleteStore,
		},
		Version:  ApiVersion,
		AuthType: "api",
	})

	server.Route(&Endpoint{
		Path: "/deletestore/",
		Handlers: MethodFuncs{
			"GET":  server.DeleteStore,
			"POST": server.DeleteStore,
		},
		AuthType: "web",
	})

	// Dashboard for managing data, auth tokens etc.
	server.Route(&Endpoint{
		Path: "/dashboard/",
		Handlers: MethodFuncs{
			"GET": server.Dashboard,
		},
		AuthType: "web",
	})

	// Endpoint for logging out
	server.Route(&Endpoint{
		Path: "/logout/",
		Handlers: MethodFuncs{
			"GET": server.Logout,
		},
		AuthType: "web",
	})

	// Endpoint for revoking auth tokens
	server.Route(&Endpoint{
		Path: "/revoke/",
		Handlers: MethodFuncs{
			"POST": server.Revoke,
		},
		AuthType: "web",
	})

	// Serve up static files
	fs := http.FileServer(http.Dir(filepath.Join(server.Config.AssetsPath, "static")))
	server.mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Fall through route
	server.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), "ApiKey") {
			server.SendDeprecatedVersionEmail(r)
		}

		accept := r.Header.Get("Accept")
		// If accept header contains "html", assume that the request comes from a browser and redirect
		// to dashboard
		if r.URL.Path == "/" && strings.Contains(accept, "html") {
			http.Redirect(w, r, "/dashboard/", http.StatusFound)
		} else {
			server.HandleError(&UnsupportedEndpoint{r.URL.Path}, w, r)
		}
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

func (server *Server) HandlePanic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				err, ok := e.(error)
				if !ok {
					err = errors.New(fmt.Sprintf("%v", e))
				}
				server.HandleError(err, w, r)
			}
		}()

		h.ServeHTTP(w, r)
	})
}

// Initialize Server with dependencies and configuration
func (server *Server) Init() error {
	var err error

	if server.Config.Secret != "" {
		if s, err := base64.StdEncoding.DecodeString(server.Config.Secret); err != nil {
			server.secret = s
		} else {
			return err
		}
	} else {
		if key, err := randomBytes(32); err != nil {
			return err
		} else {
			server.secret = key
		}
	}

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

	var handler http.Handler = server.mux

	// Add rate limiting middleWare
	handler = RateLimit(handler, map[Route]RateQuota{
		Route{"POST", "/auth/"}:    RateQuota{PerMin(1), 5},
		Route{"PUT", "/auth/"}:     RateQuota{PerMin(1), 5},
		Route{"DELETE", "/store/"}: RateQuota{PerMin(1), 5},
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.HandleError(&TooManyRequests{}, w, r)
	}))

	// Add CORS middleware
	handler = Cors(handler)

	// Add panic recovery
	handler = server.HandlePanic(handler)

	server.Handler = handler

	port := server.Config.Port
	tlsCert := server.Config.TLSCert
	tlsKey := server.Config.TLSKey

	server.Addr = fmt.Sprintf(":%d", port)

	// Start server
	if tlsCert != "" && tlsKey != "" {
		server.Info.Printf("Starting server with TLS on port %v", port)
		server.Secure = true
		return server.ListenAndServeTLS(tlsCert, tlsKey)
	} else {
		server.Info.Printf("Starting server on port %v", port)
		return server.ListenAndServe()
	}
}

// Instantiates and initializes a new Server and returns a reference to it
func NewServer(log *Log, storage Storage, sender Sender, config *ServerConfig) *Server {
	server := &Server{
		Server: &graceful.Server{
			Server:  &http.Server{},
			Timeout: time.Second * 10,
		},
		Log:       log,
		mux:       http.NewServeMux(),
		Storage:   storage,
		Sender:    sender,
		Config:    config,
		endpoints: make(map[string]*Endpoint),
	}

	// Hook up logger for http.Server
	server.ErrorLog = server.Error
	// Hook up logger for graceful.Server
	server.Logger = server.Error

	return server
}

func init() {
	RegisterStorable(&DataStore{}, "data-stores")
}
