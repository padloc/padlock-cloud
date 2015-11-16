package main

import "net/http"
import "io/ioutil"
import "crypto/rand"
import "fmt"
import "log"
import "net/smtp"
import "os"
import "encoding/json"
import "regexp"
import "errors"
import "bytes"
import "flag"
import "text/template"
import htmlTemplate "html/template"

const defaultDbPath = "./db"
const defaultAssetsPath = "./assets"
const defaultPort = 3000
const uuidPattern = "[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89aAbB][a-f0-9]{3}-[a-f0-9]{12}"

var (
	ErrInvalidToken     = errors.New("padlock: invalid token")
	ErrNotAuthenticated = errors.New("padlock: not authenticated")
	ErrWrongMethod      = errors.New("padlock: wrong http method")
)

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

type Sender interface {
	Send(string, string, string) error
}

type EmailSender struct {
	User     string
	Server   string
	Port     string
	Password string
}

// Helper function for sending emails
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
type AuthAccount struct {
	// The email servers as a unique identifier and as a means for
	// requesting/activating api keys
	Email string
	// A set of api keys that can be used to access the data associated with this
	// account
	ApiKeys []ApiKey
}

func (acc *AuthAccount) Key() []byte {
	return []byte(acc.Email)
}

func (acc *AuthAccount) Deserialize(data []byte) error {
	return json.Unmarshal(data, acc)
}

func (acc *AuthAccount) Serialize() ([]byte, error) {
	return json.Marshal(acc)
}

// Removes the api key for a given device name
func (a *AuthAccount) RemoveKeyForDevice(deviceName string) {
	for i, apiKey := range a.ApiKeys {
		if apiKey.DeviceName == deviceName {
			a.ApiKeys = append(a.ApiKeys[:i], a.ApiKeys[i+1:]...)
			return
		}
	}
}

// Adds an api key to this account. If an api key for the given device
// is already registered, that one will be replaced
func (a *AuthAccount) SetKey(apiKey ApiKey) {
	a.RemoveKeyForDevice(apiKey.DeviceName)
	a.ApiKeys = append(a.ApiKeys, apiKey)
}

// Checks if a given api key is valid for this account
func (a *AuthAccount) Validate(key string) bool {
	// Check if the account contains any ApiKey with that matches
	// the given key
	for _, apiKey := range a.ApiKeys {
		if apiKey.Key == key {
			return true
		}
	}

	return false
}

type AuthRequest struct {
	Token  string
	ApiKey ApiKey
}

func (ar *AuthRequest) Key() []byte {
	return []byte(ar.Token)
}

func (ar *AuthRequest) Deserialize(data []byte) error {
	return json.Unmarshal(data, &ar.ApiKey)
}

func (ar *AuthRequest) Serialize() ([]byte, error) {
	return json.Marshal(&ar.ApiKey)
}

type ResetRequest struct {
	Token   string
	Account string
}

func (rr *ResetRequest) Key() []byte {
	return []byte(rr.Token)
}

func (rr *ResetRequest) Deserialize(data []byte) error {
	rr.Account = string(data)
	return nil
}

func (rr *ResetRequest) Serialize() ([]byte, error) {
	return []byte(rr.Account), nil
}

type Data struct {
	Account *AuthAccount
	Content []byte
}

func (d *Data) Key() []byte {
	return []byte(d.Account.Email)
}

func (d *Data) Deserialize(data []byte) error {
	d.Content = data
	return nil
}

func (d *Data) Serialize() ([]byte, error) {
	return d.Content, nil
}

type App struct {
	*http.ServeMux
	Sender
	Storage
	// Email template for api key activation email
	ActEmailTemp *template.Template
	// Email template for deletion confirmation email
	DelEmailTemp *template.Template
	// Template for connected page
	ConnectedTemp *htmlTemplate.Template
	// Template for connected page
	DeletedTemp *htmlTemplate.Template
}

func (app *App) accountFromRequest(r *http.Request) (*AuthAccount, error) {
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
	acc := &AuthAccount{Email: email}

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

func handleError(e error, w http.ResponseWriter, r *http.Request) {
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
	default:
		{
			http.Error(w, "", http.StatusInternalServerError)
		}
	}
}

// Handler function for requesting an api key. Generates a key-token pair and stores them.
// The token can later be used to activate the api key. An email is sent to the corresponding
// email address with an activation url
func (app *App) RequestApiKey(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	// TODO: Add validation
	email, deviceName := r.PostForm.Get("email"), r.PostForm.Get("device_name")

	// Generate key-token pair
	key := uuid()
	token := uuid()
	apiKey := ApiKey{
		email,
		deviceName,
		key,
	}

	err := app.Put(&AuthRequest{token, apiKey})
	if err != nil {
		handleError(err, w, r)
		return
	}

	// Render email
	var buff bytes.Buffer
	app.ActEmailTemp.Execute(&buff, map[string]string{
		"email":           apiKey.Email,
		"device_name":     apiKey.DeviceName,
		"activation_link": fmt.Sprintf("https://%s/activate/%s", r.Host, token),
	})
	body := buff.String()

	// Send email with activation link
	go app.Send(email, "Connect to Padlock Cloud", body)

	// We're returning a JSON serialization of the ApiKey object
	w.Header().Set("Content-Type", "application/json")

	data, err := json.Marshal(apiKey)

	if err != nil {
		handleError(err, w, r)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}

// Hander function for activating a given api key
func (app *App) ActivateApiKey(w http.ResponseWriter, r *http.Request) {
	token, err := tokenFromUrl(r.URL.Path, "/activate/")
	if err != nil {
		handleError(err, w, r)
		return
	}

	authRequest := &AuthRequest{Token: token}
	// Let's check if an unactivate api key exists for this token. If not,
	// the token is obviously not valid
	err = app.Get(authRequest)
	if err != nil {
		handleError(err, w, r)
		return
	}

	acc := &AuthAccount{Email: authRequest.ApiKey.Email}

	// Fetch the account for the given email address if there is one
	err = app.Get(acc)
	if err != nil && err != ErrNotFound {
		handleError(err, w, r)
		return
	}

	// Add the new key to the account (keys with the same device name will be replaced)
	acc.SetKey(authRequest.ApiKey)

	// Save the changes
	err = app.Put(acc)
	if err != nil {
		handleError(err, w, r)
	}

	// Remove the entry for this token
	app.Delete(authRequest)

	var buff bytes.Buffer
	// Render success page
	err = app.ConnectedTemp.Execute(&buff, map[string]string{
		"device_name": authRequest.ApiKey.DeviceName,
	})

	if err != nil {
		handleError(err, w, r)
	}

	buff.WriteTo(w)
}

// Handler function for retrieving the data associated with a given account
func (app *App) GetData(w http.ResponseWriter, r *http.Request) {
	acc, err := app.accountFromRequest(r)
	if acc == nil {
		handleError(err, w, r)
		return
	}

	data := &Data{Account: acc}
	err = app.Get(data)

	// I case of a not found error we simply return an empty string
	if err != nil && err != ErrNotFound {
		handleError(err, w, r)
		return
	}

	w.Write(data.Content)
}

// Handler function for updating the data associated with a given account
func (app *App) PutData(w http.ResponseWriter, r *http.Request) {
	acc, err := app.accountFromRequest(r)
	if err != nil {
		handleError(err, w, r)
		return
	}

	data := &Data{Account: acc}
	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		handleError(err, w, r)
		return
	}
	data.Content = content

	err = app.Put(data)
	if err != nil {
		handleError(err, w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Handler function for requesting a data reset for a given account
func (app *App) RequestDataReset(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Path[1:]

	// Fetch the account for the given email address if there is one
	err := app.Get(&AuthAccount{Email: email})

	if err != nil {
		handleError(err, w, r)
		return
	}

	// Generate a new delete token
	token := uuid()

	// Save token/email pair in database to we can verify it later
	err = app.Put(&ResetRequest{token, email})
	if err != nil {
		handleError(err, w, r)
		return
	}

	// Render email
	var buff bytes.Buffer
	err = app.DelEmailTemp.Execute(&buff, map[string]string{
		"email":       email,
		"delete_link": fmt.Sprintf("https://%s/reset/%s", r.Host, token),
	})

	if err != nil {
		handleError(err, w, r)
		return
	}

	// Send email with confirmation link
	body := buff.String()
	go app.Send(email, "Padlock Cloud Delete Request", body)

	w.WriteHeader(http.StatusAccepted)
}

// Handler function for updating the data associated with a given account
func (app *App) ResetData(w http.ResponseWriter, r *http.Request) {
	token, err := tokenFromUrl(r.URL.Path, "/reset/")

	if err != nil {
		handleError(err, w, r)
		return
	}

	resetRequest := &ResetRequest{Token: token}
	// Fetch email from lookup database
	err = app.Get(resetRequest)

	if err != nil {
		handleError(err, w, r)
		return
	}

	// Delete data from database
	err = app.Delete(&Data{Account: &AuthAccount{Email: resetRequest.Account}})
	if err != nil {
		handleError(err, w, r)
		return
	}

	var buff bytes.Buffer
	// Render success page
	err = app.DeletedTemp.Execute(&buff, map[string]string{
		"email": string(resetRequest.Account),
	})

	if err != nil {
		handleError(err, w, r)
		return
	}

	buff.WriteTo(w)

	// Delete the request token
	app.Delete(resetRequest)
}

func (app *App) setupRoutes() {
	app.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			app.RequestApiKey(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	app.HandleFunc("/activate/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			app.ActivateApiKey(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	app.HandleFunc("/reset/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			app.ResetData(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

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

func (app *App) Init() {
	app.ServeMux = http.NewServeMux()
	app.setupRoutes()
}

func (app *App) Start(addr string) {
	app.Init()

	err := app.Storage.Open()
	if err != nil {
		log.Fatal(err)
	}

	defer app.Storage.Close()

	err = http.ListenAndServe(addr, app)

	if err != nil {
		log.Fatal(err)
	}
}

func loadEnv(app *App, storage *LevelDBStorage, emailSender *EmailSender, assetsPath *string) {
	emailSender.User = os.Getenv("PADLOCK_EMAIL_USERNAME")
	emailSender.Server = os.Getenv("PADLOCK_EMAIL_SERVER")
	emailSender.Port = os.Getenv("PADLOCK_EMAIL_PORT")
	emailSender.Password = os.Getenv("PADLOCK_EMAIL_PASSWORD")
	*assetsPath = os.Getenv("PADLOCK_ASSETS_PATH")
	if *assetsPath == "" {
		*assetsPath = defaultAssetsPath
	}
	storage.Path = os.Getenv("PADLOCK_DB_PATH")
	if storage.Path == "" {
		storage.Path = defaultDbPath
	}
}

func loadTemplates(app *App, path string) {
	app.ActEmailTemp = template.Must(template.ParseFiles(path + "activate.txt"))
	app.DelEmailTemp = template.Must(template.ParseFiles(path + "delete.txt"))
	app.ConnectedTemp = htmlTemplate.Must(htmlTemplate.ParseFiles(path + "connected.html"))
	app.DeletedTemp = htmlTemplate.Must(htmlTemplate.ParseFiles(path + "deleted.html"))
}

func main() {
	app := &App{}

	storage := &LevelDBStorage{}
	app.Storage = storage

	sender := &EmailSender{}
	app.Sender = sender

	var assetsPath string

	loadEnv(app, storage, sender, &assetsPath)

	loadTemplates(app, assetsPath+"/templates/")

	port := flag.Int("p", defaultPort, "Port to listen on")
	flag.Parse()

	log.Printf("Starting server on port %v", *port)
	app.Start(fmt.Sprintf(":%d", *port))
}
