package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"ctdaemon/internal/config"
	"ctdaemon/internal/logger"
	"ctdaemon/internal/manager"
)

type Server struct {
	cfg     config.ServerConfig
	mgr     *manager.Manager
	version string
}

func New(cfg config.ServerConfig, mgr *manager.Manager, version string) *Server {
	return &Server{
		cfg:     cfg,
		mgr:     mgr,
		version: version,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/control", s.handleControl)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/version", s.handleVersion)

	addr := fmt.Sprintf(":%d", s.cfg.Port)
	logger.Get("api").Info("API server listening", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action := r.URL.Query().Get("action")
	if action == "" {
		http.Error(w, "Missing action parameter", http.StatusBadRequest)
		return
	}

	var err error
	var statusMsg string

	switch action {
	case "start":
		err = s.mgr.Start()
		if err == manager.ErrAlreadyRunning {
			statusMsg = "already started"
		} else if err == nil {
			statusMsg = "started"
		}
	case "stop":
		err = s.mgr.Stop()
		if err == manager.ErrNotRunning {
			statusMsg = "already stopped"
		} else if err == nil {
			statusMsg = "stopped"
		}
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if err != nil && err != manager.ErrAlreadyRunning && err != manager.ErrNotRunning {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": statusMsg})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := s.mgr.Status()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": s.version})
}
