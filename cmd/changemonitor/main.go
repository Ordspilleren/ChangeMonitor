package main

import (
	"log"
	"os"

	appcfg "github.com/Ordspilleren/ChangeMonitor/config"
	"github.com/Ordspilleren/ChangeMonitor/frontend"
	"github.com/Ordspilleren/ChangeMonitor/internal/server"
	"github.com/Ordspilleren/ChangeMonitor/monitor"
	"github.com/Ordspilleren/ChangeMonitor/notifier"
	"github.com/Ordspilleren/ChangeMonitor/notifier/pushover"
	"github.com/Ordspilleren/ChangeMonitor/storage"
)

var ConfigFile string
var StorageDirectory string
var ChromePath string
var ChromeWs string

var config *appcfg.Config
var monitorService *monitor.MonitorService

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	ConfigFile = getEnv("CONFIG_FILE", "config.json")
	StorageDirectory = getEnv("STORAGE_DIRECTORY", "data")
	ChromePath = getEnv("CHROME_PATH", "/usr/bin/chromium")
	ChromeWs = getEnv("CHROME_WS", "")
	log.Printf("Config File: %s", ConfigFile)
	log.Printf("Storage Directory: %s", StorageDirectory)

	var err error
	config, err = appcfg.Load(ConfigFile)
	if err != nil {
		log.Print(err)
		return
	}

	notifiers := InitNotifiers()
	notifierService := notifier.NewNotifierService(notifiers)
	storageService := storage.InitStorage(StorageDirectory)

	monitorService = monitor.NewMonitorService(config.Monitors, storageService, notifierService)
	if err := monitorService.SetupChrome(ChromePath, ChromeWs); err != nil {
		log.Fatal(err)
	}
	monitorService.Start()

	server := server.NewServer(config, ConfigFile, frontend.FrontendDistFS(), monitorService)
	server.Start()
}

func InitNotifiers() notifier.Notifiers {
	var notifiers notifier.Notifiers
	if config.Notifiers.Pushover != nil {
		notifiers = append(notifiers, pushover.New(config.Notifiers.Pushover.APIToken, config.Notifiers.Pushover.UserKey))
	}
	return notifiers
}
