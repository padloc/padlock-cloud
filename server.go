package main

import "net/http"
import "io/ioutil"
import "crypto/rand"
import "fmt"
import "net/smtp"
import "os"
import "encoding/json"
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

	// m.Use(InjectBody)

	m.Post("/auth", func(req *http.Request, db *ActDB) (int, string) {
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

		return http.StatusOK, string(data)
	})

	m.Get("/activate/:token", func(params martini.Params, actDB *ActDB, authDB *AuthDB) (int, string) {
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

		return http.StatusOK, string(data)
	})

	// m.Get("/:email", func(params martini.Params, db *AuthDB) (int, string) {
	// 	accData, _ := db.Get([]byte(params["email"]), nil)
	// 	return 200, string(accData)
	// })

	// m.Get("/:id", func(params martini.Params, db *DataDB) (int, string) {
	// 	id := params["id"]
	// 	data, err := db.Get([]byte(id), nil)

	// 	if err == leveldb.ErrNotFound {
	// 		return http.StatusNotFound, "Could not find data for " + id
	// 	}

	// 	if err != nil {
	// 		return http.StatusInternalServerError, fmt.Sprintf("An error occured while fetching the data: %s", err)
	// 	}

	// 	return http.StatusOK, string(data)
	// })

	// m.Post("/:id", func(req *http.Request, params martini.Params, rb RequestBody, db *DataDB) (int, string) {
	// 	err := db.Put([]byte(params["id"]), rb, nil)

	// 	if err != nil {
	// 		return http.StatusInternalServerError, fmt.Sprintf("An error occured while storing the data: %s", err)
	// 	}

	// 	return http.StatusOK, string(rb)
	// })

	m.Run()
}
