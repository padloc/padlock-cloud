package main

import "net/http"
import "io/ioutil"
import "crypto/rand"
import "fmt"

// import "strings"
import "github.com/codegangsta/martini"
import "github.com/syndtr/goleveldb/leveldb"

type DataDB struct {
	*leveldb.DB
}

type AuthDB struct {
	*leveldb.DB
}

type RequestBody []byte

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

func main() {
	ddb, err := leveldb.OpenFile("db/data", nil)
	adb, err := leveldb.OpenFile("db/auth", nil)

	dataDB := &DataDB{ddb}
	authDB := &AuthDB{adb}

	if err != nil {
		panic("Failed to open database!")
	}

	defer dataDB.Close()
	defer authDB.Close()

	m := martini.Classic()
	m.Map(dataDB)
	m.Map(authDB)

	m.Use(InjectBody)

	m.Post("/auth", func(rb RequestBody, adb *AuthDB) string {
		apiKey := uuid()
		actKey := uuid()
		data := []byte(apiKey + "," + actKey + ",0")
		adb.Put(rb, data, nil)

		return apiKey
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
