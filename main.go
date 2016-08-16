package main

import "os"
import "log"

func main() {
	err := NewCliApp().Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
