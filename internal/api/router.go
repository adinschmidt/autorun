package api

import (
	"io/fs"
	"net/http"
	"strings"

	"autorun/internal/platform"
)

// Router sets up the HTTP routes
type Router struct {
	handler    *Handler
	streamer   *LogStreamer
	mux        *http.ServeMux
	frontendFS fs.FS
}

// NewRouter creates a new router with all API endpoints
func NewRouter(provider platform.ServiceProvider, frontendFS fs.FS) *Router {
	r := &Router{
		handler:    NewHandler(provider),
		streamer:   NewLogStreamer(provider),
		mux:        http.NewServeMux(),
		frontendFS: frontendFS,
	}

	r.setupRoutes()
	return r
}

func (r *Router) setupRoutes() {
	// API routes
	r.mux.HandleFunc("/api/platform", r.handler.GetPlatform)
	r.mux.HandleFunc("/api/services", r.handleServices)
	r.mux.HandleFunc("/api/services/", r.handleServiceAction)

	// Frontend static files
	if r.frontendFS != nil {
		fileServer := http.FileServer(http.FS(r.frontendFS))
		r.mux.Handle("/", fileServer)
	}
}

// handleServices handles GET /api/services
func (r *Router) handleServices(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.handler.ListServices(w, req)
}

// handleServiceAction routes service-specific actions
func (r *Router) handleServiceAction(w http.ResponseWriter, req *http.Request) {
	// Parse path: /api/services/{name} or /api/services/{name}/{action}
	path := strings.TrimPrefix(req.URL.Path, "/api/services/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}

	serviceName := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "":
		// GET /api/services/{name}
		if req.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.handler.GetService(w, req, serviceName)

	case "start":
		if req.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.handler.StartService(w, req, serviceName)

	case "stop":
		if req.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.handler.StopService(w, req, serviceName)

	case "restart":
		if req.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.handler.RestartService(w, req, serviceName)

	case "enable":
		if req.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.handler.EnableService(w, req, serviceName)

	case "disable":
		if req.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.handler.DisableService(w, req, serviceName)

	case "logs":
		// WebSocket upgrade for log streaming
		r.streamer.HandleLogStream(w, req, serviceName)

	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}
