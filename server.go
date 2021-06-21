package main

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Ordspilleren/ChangeMonitor/html"
	"github.com/Ordspilleren/ChangeMonitor/monitor"
	"github.com/go-chi/chi/v5"
)

type server struct {
	router chi.Router
}

func (s *server) routes() {
	s.router.Handle("/assets/*", http.FileServer(html.GetAssetFS()))
	s.router.HandleFunc("/", s.handleIndex())
	s.router.Route("/monitors", func(r chi.Router) {
		r.Get("/new", s.handleMonitorNew())
		r.Post("/new", s.handleMonitorCreate())
		r.Get("/{monitorID}/edit", s.handleMonitorEdit())
		r.Post("/{monitorID}/edit", s.handleMonitorUpdate())
	})
}

func (s *server) start() {
	s.router = chi.NewRouter()
	s.routes()

	http.ListenAndServe(":8080", s.router)
}

func (s *server) handleIndex() http.HandlerFunc {
	p := html.MonitorListParams{
		MonitorService: monitorService,
	}

	return func(w http.ResponseWriter, r *http.Request) {
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
}

func (s *server) handleMonitorCreate() http.HandlerFunc {
	p := html.MonitorNewParams{}

	return func(w http.ResponseWriter, r *http.Request) {
		name := r.FormValue("name")
		url := r.FormValue("url")
		interval, err := strconv.ParseInt(r.FormValue("interval"), 10, 64)
		if err != nil {
			log.Print(err)
		}
		var useChrome bool
		if r.FormValue("usechrome") == "yes" {
			useChrome = true
		} else {
			useChrome = false
		}
		selectorType := r.FormValue("selectortype")
		selectorPaths := r.Form["path"]

		monitor := monitor.NewMonitor(name, url, interval)
		monitor.UseChrome = useChrome

		if selectorType == "css" {
			monitor.AddCSSSelectors(selectorPaths...)
		}
		if selectorType == "json" {
			monitor.AddJSONSelectors(selectorPaths...)
		}

		monitor.Init(*monitorService.NotifierService, *monitorService.Storage, *monitorService.HttpClient, *monitorService.ChromeClient)

		monitorService.AddMonitors(*monitor)

		p.Success = true

		/*
			config.Monitors = append(config.Monitors, *monitor)

			newConfig, _ := config.JSON()
			err = ioutil.WriteFile(ConfigFile, newConfig, 0644)
			if err != nil {
				log.Print(err)
			}
		*/

		html.MonitorNew(w, p)
	}
}

func (s *server) handleMonitorNew() http.HandlerFunc {
	p := html.MonitorNewParams{}

	return func(w http.ResponseWriter, r *http.Request) {
		html.MonitorNew(w, p)
	}
}

func (s *server) handleMonitorUpdate() http.HandlerFunc {
	p := html.MonitorNewParams{}

	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "monitorID"))
		if err != nil {
			http.Error(w, "Could not parse ID to integer.", http.StatusBadRequest)
		}

		name := r.FormValue("name")
		url := r.FormValue("url")
		interval, err := strconv.ParseInt(r.FormValue("interval"), 10, 64)
		if err != nil {
			log.Print(err)
		}
		var useChrome bool
		if r.FormValue("usechrome") == "yes" {
			useChrome = true
		} else {
			useChrome = false
		}
		selectorType := r.FormValue("selectortype")
		selectorPaths := r.Form["path"]

		monitorService.Monitors[id].Name = name
		monitorService.Monitors[id].URL = url
		monitorService.Monitors[id].Interval = time.Duration(interval)
		monitorService.Monitors[id].UseChrome = useChrome

		if selectorType == "css" {
			monitorService.Monitors[id].AddCSSSelectors(selectorPaths...)
		}
		if selectorType == "json" {
			monitorService.Monitors[id].AddJSONSelectors(selectorPaths...)
		}

		monitorService.Monitors[id].Init(*monitorService.NotifierService, *monitorService.Storage, *monitorService.HttpClient, *monitorService.ChromeClient)

		p.Success = true

		html.MonitorNew(w, p)
	}
}

func (s *server) handleMonitorEdit() http.HandlerFunc {
	p := html.MonitorNewParams{}

	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "monitorID"))
		if err != nil {
			http.Error(w, "Could not parse ID to integer.", http.StatusBadRequest)
		}
		p.Monitor = monitorService.Monitors[id]
		html.MonitorNew(w, p)
	}
}
