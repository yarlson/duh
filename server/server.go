package server

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/yarlson/duh/docker"
	"github.com/yarlson/duh/logger"
	"github.com/yarlson/duh/service"
)

//go:generate moq -out docker_moq_test.go . DockerClient

// DockerClient defines the interface for Docker operations
type DockerClient interface {
	ListContainers(ctx context.Context, all bool) ([]docker.Container, error)
	GetContainerStats(ctx context.Context, id string) (*docker.ContainerStats, error)
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string) error
}

// Server represents the HTTP server
type Server struct {
	service  *service.ContainerService
	staticFS embed.FS
}

// New creates a new HTTP server
func New(service *service.ContainerService, staticFS embed.FS) *Server {
	return &Server{
		service:  service,
		staticFS: staticFS,
	}
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe(addr string) error {
	l := logger.New()
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/containers", s.handleContainers)
	mux.HandleFunc("/api/containers/", s.handleContainer)

	// Get the dist subdirectory from the embedded files
	distFS, err := fs.Sub(s.staticFS, "www/dist")
	if err != nil {
		return err
	}

	// Static file server for the React app
	staticFs := http.FileServer(http.FS(distFS))

	// Serve index.html for all non-API routes to support client-side routing
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// If it's an API request, return 404 (they should be handled above)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the requested file
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		}

		// Try to open the file from the embedded filesystem
		f, err := distFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			_ = f.Close()
			staticFs.ServeHTTP(w, r)
			return
		}

		// File doesn't exist, serve index.html
		w.Header().Set("Content-Type", "text/html")
		index, err := distFS.Open("index.html")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = index.Close() }()
		http.ServeContent(w, r, "index.html", time.Time{}, index.(io.ReadSeeker))
	})

	l.Info("http://localhost%s", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		containers := s.service.List()
		writeJSON(w, containers)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContainer(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/containers/")
	if id == "" {
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		container, exists := s.service.Get(id)
		if !exists {
			http.Error(w, "Container not found", http.StatusNotFound)
			return
		}
		writeJSON(w, container)

	case http.MethodPost:
		action := r.URL.Query().Get("action")
		if err := s.handleContainerAction(r.Context(), id, action); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContainerAction(ctx context.Context, id, action string) error {
	switch action {
	case "start":
		return s.service.StartContainer(ctx, id)
	case "stop":
		return s.service.StopContainer(ctx, id)
	default:
		return &httpError{
			Status:  http.StatusBadRequest,
			Message: "Invalid action",
		}
	}
}

type httpError struct {
	Status  int
	Message string
}

func (e *httpError) Error() string {
	return e.Message
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	l := logger.New()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		l.Warn("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
