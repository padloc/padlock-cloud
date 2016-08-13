package padlockcloud

import "os"
import "log"
import "io/ioutil"
import "encoding/json"
import "github.com/gravitational/configure"

func LoadConfig(configs ...interface{}) {
	// load config file
	yamlData, err := ioutil.ReadFile("config.yaml")
	if err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}

	for _, config := range configs {
		// parse environment variables
		err = configure.ParseEnv(config)
		if err != nil {
			log.Fatal(err)
		}

		// parse YAML
		err = configure.ParseYAML(yamlData, config)
		if err != nil {
			log.Fatal(err)
		}

		// // parse command line arguments
		// err = configure.ParseCommandLine(config, os.Args[1:])
		// if err != nil {
		// 	log.Fatal(err)
		// }

		jsonStr, _ := json.MarshalIndent(config, "", "  ")
		log.Println(string(jsonStr))
	}
}
