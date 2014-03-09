package main

import "net/http"
import "io/ioutil"
import "crypto/rand"
import "fmt"
import "net/smtp"
import "os"

// import "strings"
import "github.com/codegangsta/martini"
import "github.com/syndtr/goleveldb/leveldb"

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

var (
	emailUser     = os.Getenv("PADLOCK_EMAIL_USERNAME")
	emailServer   = os.Getenv("PADLOCK_EMAIL_SERVER")
	emailPort     = os.Getenv("PADLOCK_EMAIL_PORT")
	emailPassword = os.Getenv("PADLOCK_EMAIL_PASSWORD")
)

func uuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func InjectBody(res http.ResponseWriter, req *http.Request, c martini.Context) {
	b, err := ioutil.ReadAll(req.Body)
	rb := RequestBody(b)

	if err != nil {
		http.Error(res, fmt.Sprintf("An error occured while reading the request body: %s", err), http.StatusInternalServerError)
	}

	c.Map(rb)
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

func main() {
	ddb, err := leveldb.OpenFile("db/data", nil)
	adb, err := leveldb.OpenFile("db/auth", nil)

	dataDB := &DataDB{ddb}
	authDB := &AuthDB{adb}
	actDB := &ActDB{adb}

	if err != nil {
		panic("Failed to open database!")
	}

	defer dataDB.Close()
	defer authDB.Close()

	m := martini.Classic()
	m.Map(dataDB)
	m.Map(authDB)
	m.Map(actDB)

	m.Use(InjectBody)

	m.Post("/auth", func(rb RequestBody, db *ActDB) (int, string) {
		apiKey := uuid()
		actKey := uuid()
		data := []byte(apiKey + "," + actKey)
		adb.Put(rb, data, nil)

		go sendMail(string(rb), "Api key activation", actKey)

		return 200, apiKey
	})

	m.Get("/:id", func(params martini.Params, db *DataDB) (int, string) {
		id := params["id"]
		data, err := db.Get([]byte(id), nil)

		if err == leveldb.ErrNotFound {
			return 404, "Could not find data for " + id
		}

		if err != nil {
			return 500, fmt.Sprintf("An error occured while fetching the data: %s", err)
		}

		return 200, string(data)
	})

	m.Post("/:id", func(req *http.Request, params martini.Params, rb RequestBody, db *DataDB) (int, string) {
		err := db.Put([]byte(params["id"]), rb, nil)

		if err != nil {
			return 500, fmt.Sprintf("An error occured while storing the data: %s", err)
		}

		return 200, string(rb)
	})

	m.Run()
}
