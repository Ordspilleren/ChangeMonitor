package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"

	appcfg "github.com/Ordspilleren/ChangeMonitor/config"
)

type Server struct {
	config     *appcfg.Config
	configFile string
	mux        *http.ServeMux
}

func NewServer(config *appcfg.Config, configFile string, staticFS fs.FS) *Server {
	s := &Server{
		config:     config,
		configFile: configFile,
		mux:        http.NewServeMux(),
	}
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
	return s
}

func (s *Server) Start() {
	log.Println("Starting web server on :8080")
	if err := http.ListenAndServe(":8080", s.mux); err != nil {
		log.Fatal(err)
	}
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
	w.WriteHeader(http.StatusNoContent)
}
