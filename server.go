package main

import "net/http"
import "io/ioutil"
import "fmt"
import "github.com/codegangsta/martini"
import "github.com/syndtr/goleveldb/leveldb"

func main() {
	db, err := leveldb.OpenFile("db", nil)

	if err != nil {
		panic("Failed to open database!")
	}

	defer db.Close()

	m := martini.Classic()
	m.Map(db)

	m.Get("/:id", func(params martini.Params, db *leveldb.DB) (int, string) {
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

	m.Post("/:id", func(req *http.Request, params martini.Params, db *leveldb.DB) (int, string) {
		data, err := ioutil.ReadAll(req.Body)

		if err != nil {
			return 500, fmt.Sprintf("An error occured while reading the request body: %s", err)
		}

		err = db.Put([]byte(params["id"]), data, nil)

		if err != nil {
			return 500, fmt.Sprintf("An error occured while storing the data: %s", err)
		}

		return 200, string(data)
	})

	m.Run()
}
