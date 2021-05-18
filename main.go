package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"sync"
)

var wg = &sync.WaitGroup{}

type Config struct {
	ConfigFile       string
	StorageDirectory string
	Monitors         Monitors  `json:"monitors"`
	Notifiers        Notifiers `json:"notifiers"`
}

var config Config
var notifiers NotifierMap

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func init() {
	config.ConfigFile = getEnv("CONFIG_FILE", "config.json")
	config.StorageDirectory = getEnv("STORAGE_DIRECTORY", "data")

	b, err := ioutil.ReadFile(config.ConfigFile)
	if err != nil {
		log.Print(err)
		return
	}
	err = json.Unmarshal(b, &config)
	if err != nil {
		log.Print(err)
		return
	}

	notifiers = config.Notifiers.InitNotifiers()
}

func main() {
	config.Monitors.StartMonitoring(wg, notifiers)

	wg.Wait()
}
