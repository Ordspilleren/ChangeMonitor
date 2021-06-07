package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"sync"

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
	storage := storage.InitStorage(StorageDirectory)

	monitorService = monitor.NewMonitorService(wg, config.Monitors, storage, notifierService)
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
		server := server{}
		server.start()
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
