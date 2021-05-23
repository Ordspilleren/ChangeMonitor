package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/Ordspilleren/ChangeMonitor/html"
	"github.com/Ordspilleren/ChangeMonitor/monitor"
	"github.com/Ordspilleren/ChangeMonitor/notify"
)

var wg = &sync.WaitGroup{}

type Config struct {
	ConfigFile       string
	StorageDirectory string
	EnableWebUI      bool
	Monitors         monitor.Monitors `json:"monitors"`
	Notifiers        notify.Notifiers `json:"notifiers"`
}

var config Config
var notifierMap notify.NotifierMap

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func init() {
	config.ConfigFile = getEnv("CONFIG_FILE", "config.json")
	config.StorageDirectory = getEnv("STORAGE_DIRECTORY", "data")
	config.EnableWebUI, _ = strconv.ParseBool(getEnv("ENABLE_WEBUI", "true"))
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

	notifierMap = config.Notifiers.InitNotifiers()
}

func main() {
	config.Monitors.StartMonitoring(wg, notifierMap, config.StorageDirectory)

	if config.EnableWebUI {
		startHTTPServer()
	} else {
		wg.Wait()
	}
}

func startHTTPServer() {
	http.Handle("/assets/", http.FileServer(html.GetAssetFS()))
	http.HandleFunc("/", monitorList)
	http.HandleFunc("/new", monitorNew)
	http.ListenAndServe(":8080", nil)
}

func monitorList(w http.ResponseWriter, r *http.Request) {
	p := html.MonitorListParams{
		Monitors: &config.Monitors,
	}

	if r.Method != http.MethodPost {
		html.MonitorList(w, p)
		return
	}

	monitorID, err := strconv.ParseInt(r.FormValue("monitorid"), 10, 64)
	if err != nil {
		log.Print(err)
	}
	startMonitor := r.FormValue("start")
	if startMonitor != "" {
		config.Monitors[monitorID].Start(wg)
	}

	html.MonitorList(w, p)
}

func monitorNew(w http.ResponseWriter, r *http.Request) {
	p := html.MonitorNewParams{}

	if r.Method != http.MethodPost {
		html.MonitorNew(w, p)
		return
	}

	name := r.FormValue("name")
	url := r.FormValue("url")
	interval, err := strconv.ParseInt(r.FormValue("interval"), 10, 64)
	if err != nil {
		log.Print(err)
	}
	notifiers := r.Form["notifier"]

	monitor := monitor.NewMonitor(name, url, interval, notifiers)
	monitor.Init(notifierMap, config.StorageDirectory)

	p.Success = true

	config.Monitors = append(config.Monitors, *monitor)

	html.MonitorNew(w, p)
}
