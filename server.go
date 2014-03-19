package main

import "net/http"
import "io/ioutil"
import "crypto/rand"
import "fmt"
import "net/smtp"
import "os"
import "encoding/json"
import "regexp"
import "github.com/codegangsta/martini"
import "github.com/syndtr/goleveldb/leveldb"

var (
	emailUser     = os.Getenv("PADLOCK_EMAIL_USERNAME")
	emailServer   = os.Getenv("PADLOCK_EMAIL_SERVER")
	emailPort     = os.Getenv("PADLOCK_EMAIL_PORT")
	emailPassword = os.Getenv("PADLOCK_EMAIL_PASSWORD")
	dbPath        = os.Getenv("PADLOCK_DB_PATH")
)

type DataDB struct {
	*leveldb.DB
}

type AuthDB struct {
	*leveldb.DB
}

type ActDB struct {
	*leveldb.DB
}

type RequestBody []byte

type ApiKey struct {
	Email      string `json:"email"`
	DeviceName string `json:"device_name"`
	Key        string `json:"key"`
}

type AuthAccount struct {
	Email   string
	ApiKeys []ApiKey
}

func (a *AuthAccount) KeyForDevice(deviceName string) *ApiKey {
	for _, apiKey := range a.ApiKeys {
		if apiKey.DeviceName == deviceName {
			return &apiKey
		}
	}

	return nil
}

func (a *AuthAccount) RemoveKeyForDevice(deviceName string) {
	for i, apiKey := range a.ApiKeys {
		if apiKey.DeviceName == deviceName {
			a.ApiKeys = append(a.ApiKeys[:i], a.ApiKeys[i+1:]...)
			return
		}
	}
}

func (a *AuthAccount) SetKey(apiKey ApiKey) {
	a.RemoveKeyForDevice(apiKey.DeviceName)
	a.ApiKeys = append(a.ApiKeys, apiKey)
}

func (a *AuthAccount) HasKey(key string) bool {
	for _, apiKey := range a.ApiKeys {
		if apiKey.Key == key {
			return true
		}
	}

	return false
}

func SaveAuthAccount(a AuthAccount, db *AuthDB) error {
	key := []byte(a.Email)
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return db.Put(key, data, nil)
}

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

func uuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func sendMail(rec string, subject string, body string) error {
	auth := smtp.PlainAuth(
		"",
		emailUser,
		emailPassword,
		emailServer,
	)

	message := fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body)
	return smtp.SendMail(
		emailServer+":"+emailPort,
		auth,
		emailUser,
		[]string{rec},
		[]byte(message),
	)
}

func InjectBody(res http.ResponseWriter, req *http.Request, c martini.Context) {
	b, err := ioutil.ReadAll(req.Body)
	rb := RequestBody(b)

	if err != nil {
		http.Error(res, fmt.Sprintf("An error occured while reading the request body: %s", err), http.StatusInternalServerError)
	}

	c.Map(rb)
}

func Auth(req *http.Request, w http.ResponseWriter, db *AuthDB, c martini.Context) {
	re := regexp.MustCompile("ApiKey (?P<email>.+):(?P<key>.+)")
	authHeader := req.Header.Get("Authorization")

	fmt.Println(authHeader)

	if !re.MatchString(authHeader) {
		http.Error(w, "No valid authorization header provided", http.StatusUnauthorized)
		return
	}

	matches := re.FindStringSubmatch(authHeader)
	email, key := matches[1], matches[2]

	authAccount, err := FetchAuthAccount(email, db)

	if err != nil {
		if err == leveldb.ErrNotFound {
			http.Error(w, fmt.Sprintf("User %s does not exists", email), http.StatusUnauthorized)
		} else {
			http.Error(w, fmt.Sprintf("Database error: %s", err), http.StatusInternalServerError)
		}
		return
	}

	if !authAccount.HasKey(key) {
		http.Error(w, "The provided key was not valid", http.StatusUnauthorized)
		return
	}

	c.Map(authAccount)
}

func RequestApiKey(req *http.Request, db *ActDB, w http.ResponseWriter) (int, string) {
	req.ParseForm()
	// TODO: Add validation
	email, deviceName := req.PostForm.Get("email"), req.PostForm.Get("device_name")

	key := uuid()
	token := uuid()
	apiKey := ApiKey{
		email,
		deviceName,
		key,
	}

	// TODO: Handle the error?
	data, _ := json.Marshal(apiKey)

	// TODO: Handle the error
	db.Put([]byte(token), data, nil)

	// TODO: Use proper email body
	go sendMail(email, "Api key activation", token)

	w.Header().Set("Content-Type", "application/json")

	return http.StatusOK, string(data)
}

func ActivateApiKey(params martini.Params, actDB *ActDB, authDB *AuthDB) (int, string) {
	token := params["token"]

	data, err := actDB.Get([]byte(token), nil)
	if err != nil {
		return http.StatusNotFound, "Token not valid"
	}

	apiKey := ApiKey{}
	// TODO: Handle error?
	json.Unmarshal(data, &apiKey)

	acc, err := FetchAuthAccount(apiKey.Email, authDB)

	if err != nil && err != leveldb.ErrNotFound {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	if err == leveldb.ErrNotFound {
		acc = AuthAccount{}
		acc.Email = apiKey.Email
	}
	acc.SetKey(apiKey)

	err = SaveAuthAccount(acc, authDB)

	// TODO: Handle error?
	actDB.Delete([]byte(token), nil)

	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	return http.StatusOK, fmt.Sprintf("The api key for the device %s has been activated!", apiKey.DeviceName)
}

func GetData(acc AuthAccount, db *DataDB) (int, string) {
	data, err := db.Get([]byte(acc.Email), nil)

	if err == leveldb.ErrNotFound {
		return http.StatusNotFound, "Could not find data for " + acc.Email
	}

	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	return http.StatusOK, string(data)
}

func PutData(acc AuthAccount, data RequestBody, db *DataDB) (int, string) {
	err := db.Put([]byte(acc.Email), data, nil)

	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Database error: %s", err)
	}

	return http.StatusOK, string(data)
}

func main() {
	if dbPath == "" {
		dbPath = "/Users/martin/padlock/db"
	}
	ddb, err := leveldb.OpenFile(dbPath+"/data", nil)
	adb, err := leveldb.OpenFile(dbPath+"/auth", nil)
	acdb, err := leveldb.OpenFile(dbPath+"/act", nil)

	dataDB := &DataDB{ddb}
	authDB := &AuthDB{adb}
	actDB := &ActDB{acdb}

	if err != nil {
		panic("Failed to open database!")
	}

	defer dataDB.Close()
	defer authDB.Close()
	defer actDB.Close()

	m := martini.Classic()
	m.Map(dataDB)
	m.Map(authDB)
	m.Map(actDB)

	m.Post("/auth", RequestApiKey)

	m.Get("/activate/:token", ActivateApiKey)

	// m.Get("/:email", func(params martini.Params, db *AuthDB) (int, string) {
	// 	accData, _ := db.Get([]byte(params["email"]), nil)
	// 	return 200, string(accData)
	// })

	m.Get("/", Auth, GetData)

	m.Put("/", Auth, InjectBody, PutData)

	m.Run()
}
