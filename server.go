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
import "text/template"
import htmlTemplate "html/template"
import "github.com/MaKleSoft/padlock-cloud/Godeps/_workspace/src/github.com/syndtr/goleveldb/leveldb"

const defaultDbPath = "./db"
const defaultAssetsPath = "./assets"
const uuidPattern = "[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89aAbB][a-f0-9]{3}-[a-f0-9]{12}"

var (
	// Settings for sending emails
	emailUser     string
	emailServer   string
	emailPort     string
	emailPassword string
	// Path to assets directory
	assetsPath string
	// Path to the leveldb database
	dbPath string
	// Email template for api key activation email
	actEmailTemp *template.Template
	// Email template for deletion confirmation email
	delEmailTemp *template.Template
	// Template for connected page
	connectedTemp *htmlTemplate.Template
	// Template for connected page
	deletedTemp *htmlTemplate.Template
	// Database used for storing user accounts
	authDB *leveldb.DB
	// Database used for storing data
	dataDB *leveldb.DB
	// Database used for storing activation / authentication token pairs
	actDB *leveldb.DB
	// Database used for storing delete requests
	delDB *leveldb.DB
)

func loadEnvConfig() {
	emailUser = os.Getenv("PADLOCK_EMAIL_USERNAME")
	emailServer = os.Getenv("PADLOCK_EMAIL_SERVER")
	emailPort = os.Getenv("PADLOCK_EMAIL_PORT")
	emailPassword = os.Getenv("PADLOCK_EMAIL_PASSWORD")

	assetsPath = os.Getenv("PADLOCK_ASSETS_PATH")
	if assetsPath == "" {
		assetsPath = defaultAssetsPath
	}

	dbPath = os.Getenv("PADLOCK_DB_PATH")
	if dbPath == "" {
		dbPath = defaultDbPath
	}
}

func loadTemplates() {
	dir := assetsPath + "/templates/"
	actEmailTemp = template.Must(template.ParseFiles(dir + "activate.txt"))
	delEmailTemp = template.Must(template.ParseFiles(dir + "delete.txt"))
	connectedTemp = htmlTemplate.Must(htmlTemplate.ParseFiles(dir + "connected.html"))
	deletedTemp = htmlTemplate.Must(htmlTemplate.ParseFiles(dir + "deleted.html"))
}

func openDBs() {
	var err error

	// Open databases
	dataDB, err = leveldb.OpenFile(dbPath+"/data", nil)
	authDB, err = leveldb.OpenFile(dbPath+"/auth", nil)
	actDB, err = leveldb.OpenFile(dbPath+"/act", nil)
	delDB, err = leveldb.OpenFile(dbPath+"/del", nil)

	if err != nil {
		log.Fatal(err)
	}
}

func closeDBs() {
	var err error
	err = dataDB.Close()
	err = authDB.Close()
	err = actDB.Close()
	err = delDB.Close()

	if err != nil {
		log.Fatal(err)
	}
}

// RFC4122-compliant uuid generator
func uuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Helper function for sending emails
func sendMail(rec string, subject string, body string) error {
	auth := smtp.PlainAuth(
		"",
		emailUser,
		emailPassword,
		emailServer,
	)

	message := fmt.Sprintf("Subject: %s\r\nFrom: Padlock Cloud <%s>\r\n\r\n%s", subject, emailUser, body)
	return smtp.SendMail(
		emailServer+":"+emailPort,
		auth,
		emailUser,
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
	// Token for verifying delete requests
	DeleteToken string
}

// Fetches the ApiKey for a given device name. Returns nil if none is found
func (a *AuthAccount) KeyForDevice(deviceName string) *ApiKey {
	for _, apiKey := range a.ApiKeys {
		if apiKey.DeviceName == deviceName {
			return &apiKey
		}
	}

	return nil
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

// Saves an AuthAccount instance to a given database
func (a *AuthAccount) Save() error {
	key := []byte(a.Email)
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return authDB.Put(key, data, nil)
}

// Fetches an AuthAccount with the given email from the given database
func AccountFromEmail(email string) (*AuthAccount, error) {
	key := []byte(email)
	data, err := authDB.Get(key, nil)
	acc := AuthAccount{}

	if err != nil {
		return &acc, err
	}

	err = json.Unmarshal(data, &acc)

	return &acc, err
}

// Authentication middleware. Checks if a valid authentication header is provided
// and, in case of a successful authentication, injects the corresponding AuthAccount
// instance into andy subsequent handlers
func AccountFromRequest(r *http.Request) (*AuthAccount, error) {
	// Extract email and authentication token from Authorization header
	re := regexp.MustCompile("^ApiKey (?P<email>.+):(?P<key>.+)$")
	authHeader := r.Header.Get("Authorization")

	// Check if the Authorization header exists and is well formed
	if !re.MatchString(authHeader) {
		return nil, errors.New("No valid authorization header provided")
	}

	// Extract email and api key from Authorization header
	matches := re.FindStringSubmatch(authHeader)
	email, key := matches[1], matches[2]

	// Fetch account for the given email address
	acc, err := AccountFromEmail(email)

	if err != nil {
		return nil, err
	}

	// Check if the provide api key is valid
	if !acc.Validate(key) {
		return nil, errors.New("Invalid key")
	}

	return acc, nil
}

// Extracts a uuid-formated token from a given url
func TokenFromUrl(url string, baseUrl string) string {
	re := regexp.MustCompile("^" + baseUrl + "(?P<token>" + uuidPattern + ")$")

	if !re.MatchString(url) {
		return ""
	}

	return re.FindStringSubmatch(url)[1]
}

// Handler function for requesting an api key. Generates a key-token pair and stores them.
// The token can later be used to activate the api key. An email is sent to the corresponding
// email address with an activation url
func RequestApiKey(w http.ResponseWriter, r *http.Request) {
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

	// Store key-token pair
	// TODO: Handle the error?
	data, _ := json.Marshal(apiKey)
	// TODO: Handle the error
	actDB.Put([]byte(token), data, nil)

	// Render email
	var buff bytes.Buffer
	actEmailTemp.Execute(&buff, map[string]string{
		"email":           apiKey.Email,
		"device_name":     apiKey.DeviceName,
		"activation_link": fmt.Sprintf("https://%s/activate/%s", r.Host, token),
	})
	body := buff.String()

	// Send email with activation link
	go sendMail(email, "Connect to Padlock Cloud", body)

	// We're returning a JSON serialization of the ApiKey object
	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusCreated)
	w.Write(data)
}

// Hander function for activating a given api key
func ActivateApiKey(w http.ResponseWriter, r *http.Request) {
	token := TokenFromUrl(r.URL.Path, "/activate/")

	if token == "" {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return
	}

	// Let's check if an unactivate api key exists for this token. If not,
	// the token is obviously not valid
	data, err := actDB.Get([]byte(token), nil)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusNotFound)
		return
	}

	// We've found a record for this token, so let's create an ApiKey instance
	// with it
	apiKey := ApiKey{}
	// TODO: Handle error?
	json.Unmarshal(data, &apiKey)

	// Fetch the account for the given email address if there is one
	acc, err := AccountFromEmail(apiKey.Email)

	if err != nil && err != leveldb.ErrNotFound {
		http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
		return
	}

	// If an account for this email address, doesn't exist yet, create one
	if err == leveldb.ErrNotFound {
		acc = &AuthAccount{}
		acc.Email = apiKey.Email
	}

	// Add the new key to the account (keys with the same device name will be replaced)
	acc.SetKey(apiKey)

	// Save the changes
	err = acc.Save()

	// Remove the entry for this token
	err = actDB.Delete([]byte(token), nil)

	if err != nil && err != leveldb.ErrNotFound {
		http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
		return
	}

	// Render success page
	connectedTemp.Execute(w, map[string]string{
		"device_name": apiKey.DeviceName,
	})
}

// Handler function for retrieving the data associated with a given account
func GetData(w http.ResponseWriter, r *http.Request) {
	acc, err := AccountFromRequest(r)
	if acc == nil {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	data, err := dataDB.Get([]byte(acc.Email), nil)

	// I case of a not found error we simply return an empty string
	if err != nil && err != leveldb.ErrNotFound {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Write(data)
}

// Handler function for updating the data associated with a given account
func PutData(w http.ResponseWriter, r *http.Request) {
	acc, err := AccountFromRequest(r)
	if acc == nil {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	data, err := ioutil.ReadAll(r.Body)

	if err != nil {
		http.Error(w, fmt.Sprintf("An error occured while reading the request body: %s", err), http.StatusInternalServerError)
	}

	err = dataDB.Put([]byte(acc.Email), data, nil)

	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusNoContent)
}

// Handler function for requesting a data reset for a given account
func RequestDataReset(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Path[1:]

	// Fetch the account for the given email address if there is one
	acc, err := AccountFromEmail(email)

	if err != nil {
		if err == leveldb.ErrNotFound {
			http.Error(w, fmt.Sprintf("User %s does not exists", email), http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
		}
	}

	// Dispose of any previous delete tokens for this account
	err = delDB.Delete([]byte(acc.DeleteToken), nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
	}

	// Generate a new delete token
	token := uuid()
	acc.DeleteToken = token

	// Save the token both in the accounts database and in a separate lookup database
	err = acc.Save()
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
	}
	err = delDB.Put([]byte(token), []byte(email), nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
	}

	// Render email
	var buff bytes.Buffer
	delEmailTemp.Execute(&buff, map[string]string{
		"email":       email,
		"delete_link": fmt.Sprintf("https://%s/reset/%s", r.Host, acc.DeleteToken),
	})
	body := buff.String()

	// Send email with confirmation link
	go sendMail(email, "Padlock Cloud Delete Request", body)

	w.WriteHeader(http.StatusAccepted)
}

// Handler function for updating the data associated with a given account
func ResetData(w http.ResponseWriter, r *http.Request) {
	token := TokenFromUrl(r.URL.Path, "/reset/")

	if token == "" {
		http.Error(w, "Invalid token", http.StatusBadRequest)
	}

	// Fetch email from lookup database
	email, err := delDB.Get([]byte(token), nil)

	if err != nil {
		if err == leveldb.ErrNotFound {
			http.Error(w, "Invalid token", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
		}
	}

	// Delete data from database
	err = dataDB.Delete(email, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
	}

	// Render success page
	deletedTemp.Execute(w, map[string]string{
		"email": string(email),
	})
}

func main() {
	loadEnvConfig()
	loadTemplates()

	openDBs()
	defer closeDBs()

	http.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			RequestApiKey(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/activate/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			ActivateApiKey(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/reset/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			ResetData(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			switch r.Method {
			case "GET":
				GetData(w, r)
			case "PUT":
				PutData(w, r)
			default:
				http.Error(w, "", http.StatusMethodNotAllowed)
			}
		} else if r.Method == "DELETE" {
			RequestDataReset(w, r)
		} else {
			http.Error(w, "", http.StatusNotFound)
		}
	})

	err := http.ListenAndServe(":3000", nil)

	if err != nil {
		log.Fatal(err)
	}
}
