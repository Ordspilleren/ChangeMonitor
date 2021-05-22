package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/Ordspilleren/ChangeMonitor/html"
	"github.com/Ordspilleren/ChangeMonitor/monitor"
	"github.com/Ordspilleren/ChangeMonitor/notify"
)

var wg = &sync.WaitGroup{}

type Config struct {
	ConfigFile       string
	StorageDirectory string
	Monitors         monitor.Monitors `json:"monitors"`
	Notifiers        notify.Notifiers `json:"notifiers"`
}

var config Config
var notifiers notify.NotifierMap

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func init() {
	config.ConfigFile = getEnv("CONFIG_FILE", "config.json")
	config.StorageDirectory = getEnv("STORAGE_DIRECTORY", "data")
	log.Printf("Config File: %s", config.ConfigFile)
	log.Printf("Storage Directory: %s", config.StorageDirectory)

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
	config.Monitors.StartMonitoring(wg, notifiers, config.StorageDirectory)

	//wg.Wait()

	startHTTPServer()
}

func startHTTPServer() {
	http.Handle("/assets/", http.FileServer(html.GetAssetFS()))
	http.HandleFunc("/", monitorList)
	http.ListenAndServe(":8080", nil)
}

func monitorList(w http.ResponseWriter, r *http.Request) {
	p := html.MonitorListParams{
		Monitors: &config.Monitors,
	}
	html.MonitorList(w, p)
}
