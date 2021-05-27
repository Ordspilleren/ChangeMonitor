package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Ordspilleren/ChangeMonitor/html"
	"github.com/Ordspilleren/ChangeMonitor/monitor"
	"github.com/Ordspilleren/ChangeMonitor/notify"
	"github.com/Ordspilleren/ChangeMonitor/storage"
)

var wg = &sync.WaitGroup{}

var ConfigFile string
var StorageDirectory string
var ChromePath string
var EnableWebUI bool

type Config struct {
	Monitors  monitor.Monitors `json:"monitors"`
	Notifiers notify.Notifiers `json:"notifiers"`
}

var config Config
var notifierMap notify.NotifierMap
var storageManager *storage.Storage

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func init() {
	ConfigFile = getEnv("CONFIG_FILE", "config.json")
	StorageDirectory = getEnv("STORAGE_DIRECTORY", "data")
	ChromePath = getEnv("CHROME_PATH", "/usr/bin/chromium")
	EnableWebUI, _ = strconv.ParseBool(getEnv("ENABLE_WEBUI", "false"))
	log.Printf("Config File: %s", ConfigFile)
	log.Printf("Storage Directory: %s", StorageDirectory)

	b, err := ioutil.ReadFile(ConfigFile)
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
	storageManager = storage.InitStorage(StorageDirectory)
}

func main() {
	config.Monitors.StartMonitoring(wg, notifierMap, storageManager, ChromePath)

	if EnableWebUI {
		startHTTPServer()
	} else {
		wg.Wait()
	}
}

func (t *Config) JSON() ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "\t")
	err := encoder.Encode(t)
	return buffer.Bytes(), err
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

	monitorID := r.URL.Query().Get("id")
	if monitorID != "" {
		id, _ := strconv.ParseInt(monitorID, 10, 64)
		p.Monitor = &config.Monitors[id]
	}

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
	cssSelectors := r.FormValue("cssselectors")
	jsonSelectors := r.FormValue("jsonselectors")
	notifiers := r.Form["notifier"]

	monitor := monitor.NewMonitor(name, url, interval, notifiers)

	if cssSelectors != "" {
		cssSelectorSlice := strings.Split(cssSelectors, "\n")
		monitor.AddCSSSelectors(cssSelectorSlice...)
	}
	if jsonSelectors != "" {
		jsonSelectorSlice := strings.Split(jsonSelectors, "\n")
		monitor.AddCSSSelectors(jsonSelectorSlice...)
	}

	monitor.Init(notifierMap, storageManager, ChromePath)

	p.Success = true

	config.Monitors = append(config.Monitors, *monitor)

	newConfig, _ := config.JSON()
	err = ioutil.WriteFile(ConfigFile, newConfig, 0644)
	if err != nil {
		log.Print(err)
	}

	html.MonitorNew(w, p)
}
