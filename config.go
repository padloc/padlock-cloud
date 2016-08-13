package padlockcloud

import "log"
import "flag"
import "io/ioutil"
import "encoding/json"
import "github.com/gravitational/configure"

func LoadConfig(configs ...interface{}) {
	var yamlData []byte
	var err error
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	if *configPath != "" {
		// load config file
		yamlData, err = ioutil.ReadFile(*configPath)
		if err != nil {
			log.Fatal(err)
		}
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

		// TODO: Activate as soon as https://github.com/gravitational/configure/issues/18 is fixed
		// // parse command line arguments
		// err = configure.ParseCommandLine(config, os.Args[1:])
		// if err != nil {
		// 	log.Fatal(err)
		// }

		jsonStr, _ := json.MarshalIndent(config, "", "  ")
		log.Println(string(jsonStr))
	}
}
