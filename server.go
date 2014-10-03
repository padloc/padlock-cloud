package main

import "net/http"
import "io/ioutil"
import "crypto/rand"
import "fmt"
import "net/smtp"
import "os"
import "encoding/json"
import "regexp"
import "bytes"
import "text/template"
import htmlTemplate "html/template"
import "github.com/codegangsta/martini"
import "github.com/syndtr/goleveldb/leveldb"

var (
	// Settings for sending emails
	emailUser     = os.Getenv("PADLOCK_EMAIL_USERNAME")
	emailServer   = os.Getenv("PADLOCK_EMAIL_SERVER")
	emailPort     = os.Getenv("PADLOCK_EMAIL_PORT")
	emailPassword = os.Getenv("PADLOCK_EMAIL_PASSWORD")
	// Path to the leveldb database
	dbPath = os.Getenv("PADLOCK_DB_PATH")
	// Email template for api key activation email
	actEmailTemp = template.Must(template.ParseFiles("templates/activate.txt"))
	// Email template for deletion confirmation email
	delEmailTemp = template.Must(template.ParseFiles("templates/delete.txt"))
	// Template for connected page
	connectedTemp = htmlTemplate.Must(htmlTemplate.ParseFiles("templates/connected.html"))
	// Template for connected page
	deletedTemp = htmlTemplate.Must(htmlTemplate.ParseFiles("templates/deleted.html"))
)

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

// These are used so the different databases can be injected as services
// into hanlder functions
type DataDB struct {
	*leveldb.DB
}
type AuthDB struct {
	*leveldb.DB
}
type ActDB struct {
	*leveldb.DB
}
type DelDB struct {
	*leveldb.DB
}

// Service type for use in handler functions. Gets injectected by the InjectBody
// middleware
type RequestBody []byte

// Middleware for reading the request body and injecting it as a RequestBody
func InjectBody(res http.ResponseWriter, req *http.Request, c martini.Context) {
	b, err := ioutil.ReadAll(req.Body)
	rb := RequestBody(b)

	if err != nil {
		http.Error(res, fmt.Sprintf("An error occured while reading the request body: %s", err), http.StatusInternalServerError)
	}

	c.Map(rb)
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
func SaveAuthAccount(a AuthAccount, db *AuthDB) error {
	key := []byte(a.Email)
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return db.Put(key, data, nil)
}

// Fetches an AuthAccount with the given email from the given database
func FetchAuthAccount(email string, db *AuthDB) (AuthAccount, error) {
	key := []byte(email)
	data, err := db.Get(key, nil)
	acc := AuthAccount{}

	if err != nil {
		return acc, err
	}

	err = json.Unmarshal(data, &acc)

	if err != nil {
		return acc, err
	}

	return acc, nil
}

// Authentication middleware. Checks if a valid authentication header is provided
// and, in case of a successful authentication, injects the corresponding AuthAccount
// instance into andy subsequent handlers
func Auth(req *http.Request, w http.ResponseWriter, db *AuthDB, c martini.Context) {
	re := regexp.MustCompile("ApiKey (?P<email>.+):(?P<key>.+)")
	authHeader := req.Header.Get("Authorization")

	// Check if the Authorization header exists and is well formed
	if !re.MatchString(authHeader) {
		http.Error(w, "No valid authorization header provided", http.StatusUnauthorized)
		return
	}

	// Extract email and api key from Authorization header
	matches := re.FindStringSubmatch(authHeader)
	email, key := matches[1], matches[2]

	// Fetch account for the given email address
	authAccount, err := FetchAuthAccount(email, db)

	if err != nil {
		if err == leveldb.ErrNotFound {
			http.Error(w, fmt.Sprintf("User %s does not exists", email), http.StatusUnauthorized)
		} else {
			http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
		}
		return
	}

	// Check if the provide api key is valid
	if !authAccount.Validate(key) {
		http.Error(w, "The provided key was not valid", http.StatusUnauthorized)
		return
	}

	c.Map(authAccount)
}

// Handler function for requesting an api key. Generates a key-token pair and stores them.
// The token can later be used to activate the api key. An email is sent to the corresponding
// email address with an activation url
func RequestApiKey(req *http.Request, actDb *ActDB, w http.ResponseWriter) (int, string) {
	req.ParseForm()
	// TODO: Add validation
	email, deviceName := req.PostForm.Get("email"), req.PostForm.Get("device_name")

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
	actDb.Put([]byte(token), data, nil)

	// Render email
	var buff bytes.Buffer
	actEmailTemp.Execute(&buff, map[string]string{
		"email":           apiKey.Email,
		"device_name":     apiKey.DeviceName,
		"activation_link": fmt.Sprintf("https://%s/activate/%s", req.Host, token),
	})
	body := buff.String()

	// Send email with activation link
	go sendMail(email, "Connect to Padlock Cloud", body)

	// We're returning a JSON serialization of the ApiKey object
	w.Header().Set("Content-Type", "application/json")

	return http.StatusOK, string(data)
}

// Hander function for activating a given api key
func ActivateApiKey(params martini.Params, actDB *ActDB, authDB *AuthDB) (int, string) {
	token := params["token"]

	// Let's check if an unactivate api key exists for this token. If not,
	// the token is obviously not valid
	data, err := actDB.Get([]byte(token), nil)
	if err != nil {
		return http.StatusNotFound, "Token not valid"
	}

	// We've found a record for this token, so let's create an ApiKey instance
	// with it
	apiKey := ApiKey{}
	// TODO: Handle error?
	json.Unmarshal(data, &apiKey)

	// Fetch the account for the given email address if there is one
	acc, err := FetchAuthAccount(apiKey.Email, authDB)

	if err != nil && err != leveldb.ErrNotFound {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	// If an account for this email address, doesn't exist yet, create one
	if err == leveldb.ErrNotFound {
		acc = AuthAccount{}
		acc.Email = apiKey.Email
	}

	// Add the new key to the account (keys with the same device name will be replaced)
	acc.SetKey(apiKey)

	// Save the changes
	err = SaveAuthAccount(acc, authDB)

	// Remove the entry for this token
	err = actDB.Delete([]byte(token), nil)

	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	// Render success page
	var buff bytes.Buffer
	connectedTemp.Execute(&buff, map[string]string{
		"device_name": apiKey.DeviceName,
	})
	return http.StatusOK, buff.String()
}

// Handler function for retrieving the data associated with a given account
func GetData(acc AuthAccount, db *DataDB) (int, string) {
	data, err := db.Get([]byte(acc.Email), nil)

	// I case of a not found error we simply return an empty string
	if err != nil && err != leveldb.ErrNotFound {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	return http.StatusOK, string(data)
}

// Handler function for updating the data associated with a given account
func PutData(acc AuthAccount, data RequestBody, db *DataDB) (int, string) {
	err := db.Put([]byte(acc.Email), data, nil)

	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	return http.StatusOK, string(data)
}

// Handler function for requesting a data reset for a given account
func RequestDataReset(req *http.Request, params martini.Params, authDB *AuthDB, delDB *DelDB) (int, string) {
	email := params["email"]

	// Fetch the account for the given email address if there is one
	acc, err := FetchAuthAccount(email, authDB)

	if err != nil {
		if err == leveldb.ErrNotFound {
			return http.StatusNotFound, fmt.Sprintf("User %s does not exists", email)
		} else {
			return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
		}
	}

	// Dispose of any previous delete tokens for this account
	err = delDB.Delete([]byte(acc.DeleteToken), nil)
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	// Generate a new delete token
	token := uuid()
	acc.DeleteToken = token

	// Save the token both in the accounts database and in a separate lookup database
	err = SaveAuthAccount(acc, authDB)
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}
	err = delDB.Put([]byte(token), []byte(email), nil)
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	// Render email
	var buff bytes.Buffer
	delEmailTemp.Execute(&buff, map[string]string{
		"email":       email,
		"delete_link": fmt.Sprintf("https://%s/reset/%s", req.Host, acc.DeleteToken),
	})
	body := buff.String()

	// Send email with confirmation link
	go sendMail(email, "Padlock Cloud Delete Request", body)

	return http.StatusAccepted, ""
}

// Handler function for updating the data associated with a given account
func ResetData(params martini.Params, delDB *DelDB, db *DataDB) (int, string) {
	token := params["token"]
	
	// Fetch email from lookup database
	email, err := delDB.Get([]byte(token), nil)

	if err != nil {
		if err == leveldb.ErrNotFound {
			return http.StatusNotFound, fmt.Sprintf("This token is not valid")
		} else {
			return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
		}
	}

	// Delete data from database
	err = db.Delete(email, nil)
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	// Render success page
	var buff bytes.Buffer
	deletedTemp.Execute(&buff, map[string]string{
		"email": string(email),
	})
	return http.StatusOK, buff.String()
}

func main() {
	if dbPath == "" {
		dbPath = "/var/lib/padlock"
	}

	// Open databases
	ddb, err := leveldb.OpenFile(dbPath+"/data", nil)
	adb, err := leveldb.OpenFile(dbPath+"/auth", nil)
	acdb, err := leveldb.OpenFile(dbPath+"/act", nil)
	deldb, err := leveldb.OpenFile(dbPath+"/del", nil)

	if err != nil {
		panic("Failed to open database!")
	}

	defer ddb.Close()
	defer adb.Close()
	defer acdb.Close()
	defer deldb.Close()

	// Create new martini web server instance
	m := martini.Classic()

	// Wrap datbases into different types so we can map them
	dataDB := &DataDB{ddb}
	authDB := &AuthDB{adb}
	actDB := &ActDB{acdb}
	delDB := &DelDB{deldb}

	// Map databases so they can be injected into handlers
	m.Map(dataDB)
	m.Map(authDB)
	m.Map(actDB)
	m.Map(delDB)

	m.Post("/auth", RequestApiKey)

	m.Get("/activate/:token", ActivateApiKey)

	m.Delete("/:email", RequestDataReset)
	m.Get("/reset/:token", ResetData)

	// m.Get("/:email", func(params martini.Params, db *AuthDB) (int, string) {
	// 	accData, _ := db.Get([]byte(params["email"]), nil)
	// 	return 200, string(accData)
	// })

	m.Get("/", Auth, GetData)

	m.Put("/", Auth, InjectBody, PutData)

	m.Run()
}
