package padlockcloud

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/rs/cors"
	"gopkg.in/tylerb/graceful.v1"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ApiVersion = 1
)

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

func IPFromRequest(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.RemoteAddr
	}
	return ip
}

func FormatRequest(r *http.Request) string {
	return fmt.Sprintf("%s %s %s", IPFromRequest(r), r.Method, r.URL)
}

func formatRequestVerbose(r *http.Request) string {
	dump, _ := httputil.DumpRequest(r, false)
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
	// Explicit base url to use in place of http.Request::Host when generating urls and such
	BaseUrl string `yaml:"base_url"`
	// Secret used for authenticating cookies
	Secret string `yaml:"secret"`
	// Enable Cross-Origin Resource Sharing
	Cors bool `yaml:"cors"`
	// Test mode
	Test bool `yaml:"test"`
	// Whitelisted path
	WhitelistPath string `yaml:"whitelist_path"`
}

// The Server type holds all the contextual data and logic used for running a Padlock Cloud instances
// Users should use the `NewServer` function to instantiate an `Server` instance
type Server struct {
	*graceful.Server
	*Log
	Storage           Storage
	Sender            Sender
	Templates         *Templates
	Config            *ServerConfig
	Secure            bool
	Endpoints         map[string]*Endpoint
	secret            []byte
	emailRateLimiter  *EmailRateLimiter
	cleanAuthRequests *Job
	whitelist         *Whitelist
	accountMutexes    map[string]*sync.Mutex
}

func (server *Server) BaseUrl(r *http.Request) string {
	if server.Config.BaseUrl != "" {
		return strings.TrimSuffix(server.Config.BaseUrl, "/")
	} else {
		var scheme string
		if server.Secure {
			scheme = "https"
		} else {
			scheme = "http"
		}
		return fmt.Sprintf("%s://%s", scheme, r.Host)
	}
}

func (server *Server) GetAccountMutex(email string) *sync.Mutex {
	if server.accountMutexes[email] == nil {
		server.accountMutexes[email] = &sync.Mutex{}
	}

	return server.accountMutexes[email]
}

func (server *Server) LockAccount(email string) {
	server.GetAccountMutex(email).Lock()
}

func (server *Server) UnlockAccount(email string) {
	server.GetAccountMutex(email).Unlock()
}

func (server *Server) DeleteAccount(email string) error {
	acc := &Account{Email: email}

	if err := server.Storage.Delete(&DataStore{Account: acc}); err != nil {
		return err
	}

	if err := server.Storage.Delete(acc); err != nil {
		return err
	}

	return nil
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

	acc.ExpireUnusedAuthTokens()
	acc.RemoveExpiredAuthTokens()

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

	// Update device meta data
	if authToken.Device == nil {
		authToken.Device = DeviceFromRequest(r)
	} else {
		authToken.Device.UpdateFromRequest(r)
	}

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
		server.Error.Printf("%s\n***STACK TRACE***\n%+v\n***REQUEST***\n%s\n", FormatRequest(r), e, formatRequestVerbose(r))
	default:
		server.Info.Printf("%s - %v", FormatRequest(r), e)
	}
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

// Registers handlers mapped by method for a given path
func (server *Server) WrapEndpoint(endpoint *Endpoint) Handler {
	var h Handler = endpoint

	// If endpoint is authenticated, wrap handler in csrf middleware
	if endpoint.AuthType != "" {
		h = (&CSRF{server}).Wrap(h)
	}

	// Check for correct endpoint version
	h = (&CheckEndpointVersion{server, endpoint.Version}).Wrap(h)

	// Wrap handler in auth middleware
	h = (&Authenticate{server, endpoint.AuthType}).Wrap(h)

	// Wrap handler in auth middleware
	h = (&LockAccount{server}).Wrap(h)

	// Check if Method is supported
	h = (&CheckMethod{endpoint.Handlers}).Wrap(h)

	h = (&HandlePanic{}).Wrap(h)

	h = (&HandleError{server}).Wrap(h)

	return h
}

// Registeres http handlers for various routes
func (server *Server) InitEndpoints() {
	if server.Endpoints == nil {
		server.Endpoints = make(map[string]*Endpoint)
	}

	// Endpoint for logging in / requesting api keys
	server.Endpoints["/auth/"] = &Endpoint{
		Handlers: map[string]Handler{
			"PUT":  &RequestAuthToken{server},
			"POST": &RequestAuthToken{server},
		},
		Version: ApiVersion,
	}

	// Endpoint for logging in / requesting api keys
	server.Endpoints["/login/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET":  &LoginPage{server},
			"POST": &RequestAuthToken{server},
		},
	}

	// Endpoint for activating auth tokens
	server.Endpoints["/activate/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET":  &ActivateAuthToken{server},
			"POST": &ActivateAuthToken{server},
		},
	}

	// Endpoint for activating auth tokens (alias)
	server.Endpoints["/a/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET":  &ActivateAuthToken{server},
			"POST": &ActivateAuthToken{server},
		},
	}

	// Endpoint for reading / writing and deleting a store
	server.Endpoints["/store/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET":  &ReadStore{server},
			"HEAD": &ReadStore{server},
			"PUT":  &WriteStore{server},
			"POST": &WriteStore{server},
		},
		Version:  ApiVersion,
		AuthType: "api",
	}

	server.Endpoints["/deletestore/"] = &Endpoint{
		Handlers: map[string]Handler{
			"POST": &DeleteStore{server},
		},
		AuthType: "web",
	}

	server.Endpoints["/deleteaccount/"] = &Endpoint{
		Handlers: map[string]Handler{
			"POST": &DeleteAccount{server},
		},
		AuthType: "universal",
	}

	// Dashboard for managing data, auth tokens etc.
	server.Endpoints["/dashboard/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET": &Dashboard{server},
		},
		AuthType: "web",
	}

	// Endpoint for logging out
	server.Endpoints["/logout/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET": &Logout{server},
		},
		AuthType: "universal",
	}

	// Endpoint for revoking auth tokens
	server.Endpoints["/revoke/"] = &Endpoint{
		Handlers: map[string]Handler{
			"POST": &Revoke{server},
		},
		AuthType: "universal",
	}

	// Account info
	server.Endpoints["/account/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET": &AccountInfo{server},
		},
		AuthType: "universal",
	}

	server.Endpoints["/static/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET": NewStaticHandler(
				filepath.Join(server.Config.AssetsPath, "static"),
				"/static/",
			),
		},
	}

	server.Endpoints["/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET": &RootHandler{server},
			// Older clients might still be using this method. Add a void handler so
			// the request gets past the allowed method check and the request can be handled
			// as a UnsupportedApiVersion error
			"PUT": &VoidHandler{},
		},
	}

}
func (server *Server) InitHandler() {
	mux := http.NewServeMux()

	for key, endpoint := range server.Endpoints {
		mux.Handle(key, HttpHandler(server.WrapEndpoint(endpoint)))
	}

	if server.Config.Cors {
		exposedHeaders := []string{"X-Sub-Required", "X-Sub-Status", "X-Sub-Trial-End", "X-Stripe-Pub-Key"}
		if server.Config.Test {
			exposedHeaders = append(exposedHeaders, "X-Test-Act-Url")
		}
		server.Handler = cors.New(cors.Options{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"HEAD", "GET", "POST", "PUT", "DELETE"},
			AllowedHeaders: []string{
				"Authorization", "Accept", "Content-Type", "X-Client-Version", "X-Client-Platform",
				"X-Device-App-Version",
				"X-Device-Platform",
				"X-Device-UUID",
				"X-Device-Manufacturer",
				"X-Device-OS-Version",
				"X-Device-Model",
				"X-Device-Hostname",
			},
			ExposedHeaders: exposedHeaders,
		}).Handler(mux)
	} else {
		server.Handler = mux
	}
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

	if email != "" && !server.emailRateLimiter.RateLimit(IPFromRequest(r), email) {
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

func (server *Server) Init() error {
	var err error

	if err := server.Log.Init(); err != nil {
		return err
	}

	if server.Config.Secret != "" {
		if s, err := base64.StdEncoding.DecodeString(server.Config.Secret); err != nil {
			return err
		} else {
			server.secret = s
		}
	} else {
		if key, err := randomBytes(32); err != nil {
			return err
		} else {
			server.secret = key
		}
	}

	server.InitEndpoints()

	if server.Templates == nil {
		server.Templates = &Templates{}
		// Load templates from assets directory
		if err := LoadTemplates(server.Templates, filepath.Join(server.Config.AssetsPath, "templates")); err != nil {
			return err
		}
	}

	// Open storage
	if err = server.Storage.Open(); err != nil {
		return err
	}

	if rl, err := NewEmailRateLimiter(
		RateQuota{PerMin(1), 5},
		RateQuota{PerMin(1), 5},
	); err != nil {
		return err
	} else {
		server.emailRateLimiter = rl
	}

	server.cleanAuthRequests = &Job{
		Action: func() {
			ar := &AuthRequest{}
			iter, err := server.Storage.Iterator(ar)
			if err != nil {
				server.Log.Error.Println("Error while cleaning auth requests:", err)
				return
			}
			defer iter.Release()

			n := 0
			for iter.Next() {
				if err := iter.Get(ar); err != nil {
					server.Log.Error.Println("Error while cleaning auth requests:", err)
				}
				if ar.Created.Before(time.Now().Add(-24 * time.Hour)) {
					if err := server.Storage.Delete(ar); err != nil {
						server.Log.Error.Println("Error while cleaning auth requests:", err)
					}
					n = n + 1
				}
			}

			if n > 0 {
				server.Log.Info.Printf("Deleted %d auth requests older than 24 hrs", n)
			}
		},
	}

	server.cleanAuthRequests.Start(24 * time.Hour)

	if server.Config.WhitelistPath != "" {
		whitelist, err := NewWhitelist(server.Config.WhitelistPath)
		if err != nil {
			return err
		}
		server.whitelist = whitelist
		server.Log.Info.Printf("%d Whitelist emails set.\n", len(whitelist.Emails))
	}

	server.accountMutexes = make(map[string]*sync.Mutex)

	return nil
}

func (server *Server) CleanUp() error {
	if server.cleanAuthRequests != nil {
		server.cleanAuthRequests.Stop()
	}
	return server.Storage.Close()
}

func (server *Server) Start() error {
	defer server.CleanUp()

	server.InitHandler()

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
		Log:     log,
		Storage: storage,
		Sender:  sender,
		Config:  config,
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
