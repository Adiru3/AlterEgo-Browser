package api

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"github.com/alterego/browser/internal/browser"
	"github.com/alterego/browser/internal/profile"
)

type Server struct {
	mgr    *profile.Manager
	static embed.FS
}

func NewServer(mgr *profile.Manager, static embed.FS) *Server {
	return &Server{
		mgr:    mgr,
		static: static,
	}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// Static files
	fsys, err := fs.Sub(s.static, "static")
	if err != nil {
		log.Fatalf("Failed to load static files: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(fsys)))

	// API Routes
	mux.HandleFunc("GET /api/profiles", s.handleGetProfiles)
	mux.HandleFunc("POST /api/profiles", s.handleCreateProfile)
	mux.HandleFunc("PUT /api/profiles/", s.handleUpdateProfile)
	mux.HandleFunc("DELETE /api/profiles/", s.handleDeleteProfile)
	mux.HandleFunc("POST /api/profiles/launch/", s.handleLaunchProfile)

	return mux
}

func (s *Server) handleGetProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.mgr.LoadAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if profiles == nil {
		profiles = []*profile.Config{}
	}
	json.NewEncoder(w).Encode(profiles)
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = "New Profile"
	}
	
	cfg, err := s.mgr.Create(req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(cfg)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/profiles/"):]
	if id == "" {
		http.Error(w, "ID required", http.StatusBadRequest)
		return
	}

	var updated profile.Config
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// Make sure ID matches
	if updated.ID != id {
		http.Error(w, "ID mismatch", http.StatusBadRequest)
		return
	}

	if err := s.mgr.Save(&updated); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updated)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/profiles/"):]
	if id == "" {
		http.Error(w, "ID required", http.StatusBadRequest)
		return
	}

	if err := s.mgr.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLaunchProfile(w http.ResponseWriter, r *http.Request) {
	// e.g. /api/profiles/launch/<id>
	id := r.URL.Path[len("/api/profiles/launch/"):]
	if id == "" {
		http.Error(w, "ID required", http.StatusBadRequest)
		return
	}

	cfg, err := s.mgr.Get(id)
	if err != nil {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	go func() {
		_, err := browser.LaunchRod(cfg, s.mgr)
		if err != nil {
			log.Printf("Failed to launch profile %s: %v", id, err)
		}
	}()

	w.WriteHeader(http.StatusOK)
}
