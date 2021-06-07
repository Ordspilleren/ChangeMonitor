package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Ordspilleren/ChangeMonitor/html"
	"github.com/Ordspilleren/ChangeMonitor/monitor"
)

type server struct {
}

func (s *server) start() {
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
		p.Monitor = monitorService.Monitors[id]
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
