package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"

	appcfg "github.com/Ordspilleren/ChangeMonitor/config"
	"github.com/Ordspilleren/ChangeMonitor/monitor"
)

type Server struct {
	config         *appcfg.Config
	configFile     string
	mux            *http.ServeMux
	monitorService *monitor.MonitorService
}

func NewServer(config *appcfg.Config, configFile string, staticFS fs.FS, monitorService *monitor.MonitorService) *Server {
	s := &Server{
		config:         config,
		configFile:     configFile,
		mux:            http.NewServeMux(),
		monitorService: monitorService,
	}
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/preview", s.handlePreview)
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
	return s
}

func (s *Server) Start() {
	log.Println("Starting web server on :8080")
	if err := http.ListenAndServe(":8080", s.mux); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req monitor.PreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := s.monitorService.Preview(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getConfig(w)
	case http.MethodPost:
		s.postConfig(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getConfig(w http.ResponseWriter) {
	data, err := s.config.JSON()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) postConfig(w http.ResponseWriter, r *http.Request) {
	var newConfig appcfg.Config
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := newConfig.JSON()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(s.configFile, data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.config = &newConfig

	if err := s.monitorService.Reload(newConfig.Monitors); err != nil {
		log.Printf("server: config reload: %v", err)
		http.Error(w, "config saved but monitors failed to reload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
