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
var ChromeWs string
var EnableWebUI bool

type Config struct {
	Monitors  monitor.Monitors `json:"monitors"`
	Notifiers notify.Notifiers `json:"notifiers"`
}

var config Config
var monitorService *monitor.MonitorService

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
	ChromeWs = getEnv("CHROME_WS", "")
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

	notifierService := notify.NewNotifierService(config.Notifiers)
	storageManager := storage.InitStorage(StorageDirectory)

	monitorService = monitor.NewMonitorService(wg, config.Monitors, storageManager, notifierService)
	if ChromeWs != "" {
		monitorService.NewMonitorClients(ChromeWs, true)
	} else {
		monitorService.NewMonitorClients(ChromePath, false)
	}
	monitorService.InitMonitors()
}

func main() {
	monitorService.StartMonitoring()

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
		MonitorService: monitorService,
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
	stopMonitor := r.FormValue("stop")
	if startMonitor != "" {
		p.MonitorService.Monitors[monitorID].Start(p.MonitorService.WaitGroup)
	}
	if stopMonitor != "" {
		p.MonitorService.Monitors[monitorID].Stop()
	}

	html.MonitorList(w, p)
}

func monitorNew(w http.ResponseWriter, r *http.Request) {
	p := html.MonitorNewParams{}

	monitorID := r.URL.Query().Get("id")
	if monitorID != "" {
		id, _ := strconv.ParseInt(monitorID, 10, 64)
		p.Monitor = &monitorService.Monitors[id]
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

	monitor := monitor.NewMonitor(name, url, interval)

	if cssSelectors != "" {
		cssSelectorSlice := strings.Split(cssSelectors, "\n")
		monitor.AddCSSSelectors(cssSelectorSlice...)
	}
	if jsonSelectors != "" {
		jsonSelectorSlice := strings.Split(jsonSelectors, "\n")
		monitor.AddCSSSelectors(jsonSelectorSlice...)
	}

	monitor.Init(*monitorService.NotifierService, *monitorService.Storage, *monitorService.HttpClient, *monitorService.ChromeClient)

	p.Success = true

	monitorService.AddMonitors(*monitor)

	config.Monitors = append(config.Monitors, *monitor)

	newConfig, _ := config.JSON()
	err = ioutil.WriteFile(ConfigFile, newConfig, 0644)
	if err != nil {
		log.Print(err)
	}

	html.MonitorNew(w, p)
}
