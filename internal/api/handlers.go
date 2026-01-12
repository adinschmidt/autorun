package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"autorun/internal/models"
	"autorun/internal/platform"
)

// Handler wraps the service provider and provides HTTP handlers
type Handler struct {
	provider platform.ServiceProvider
}

// NewHandler creates a new API handler
func NewHandler(provider platform.ServiceProvider) *Handler {
	return &Handler{provider: provider}
}

// jsonResponse writes a JSON response
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// errorResponse writes an error response
func errorResponse(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

// parseScope extracts and validates the scope from query parameters
func parseScope(r *http.Request) models.Scope {
	scope := r.URL.Query().Get("scope")
	switch scope {
	case "system":
		return models.ScopeSystem
	case "user":
		return models.ScopeUser
	default:
		return models.ScopeUser
	}
}

// GetPlatform returns the current platform name and elevation status
func (h *Handler) GetPlatform(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"platform": h.provider.Name(),
		"elevated": os.Geteuid() == 0,
	})
}

// ListServices returns all services for the requested scope
func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	scopeParam := r.URL.Query().Get("scope")

	var allServices []models.Service

	if scopeParam == "all" || scopeParam == "" {
		// Get both system and user services
		systemServices, err := h.provider.ListServices(models.ScopeSystem)
		if err != nil {
			// Log but don't fail - user might not have permissions
		} else {
			allServices = append(allServices, systemServices...)
		}

		userServices, err := h.provider.ListServices(models.ScopeUser)
		if err != nil {
			// Log but don't fail
		} else {
			allServices = append(allServices, userServices...)
		}
	} else {
		scope := parseScope(r)
		services, err := h.provider.ListServices(scope)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		allServices = services
	}

	jsonResponse(w, http.StatusOK, allServices)
}

// GetService returns details for a specific service
func (h *Handler) GetService(w http.ResponseWriter, r *http.Request, name string) {
	scope := parseScope(r)
	service, err := h.provider.GetService(name, scope)
	if err != nil {
		errorResponse(w, http.StatusNotFound, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, service)
}

// StartService starts a service
func (h *Handler) StartService(w http.ResponseWriter, r *http.Request, name string) {
	scope := parseScope(r)
	if err := h.provider.Start(name, scope); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "started"})
}

// StopService stops a service
func (h *Handler) StopService(w http.ResponseWriter, r *http.Request, name string) {
	scope := parseScope(r)
	if err := h.provider.Stop(name, scope); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// RestartService restarts a service
func (h *Handler) RestartService(w http.ResponseWriter, r *http.Request, name string) {
	scope := parseScope(r)
	if err := h.provider.Restart(name, scope); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "restarted"})
}

// EnableService enables a service
func (h *Handler) EnableService(w http.ResponseWriter, r *http.Request, name string) {
	scope := parseScope(r)
	if err := h.provider.Enable(name, scope); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "enabled"})
}

// DisableService disables a service
func (h *Handler) DisableService(w http.ResponseWriter, r *http.Request, name string) {
	scope := parseScope(r)
	if err := h.provider.Disable(name, scope); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "disabled"})
}

// CreateService creates a new service
func (h *Handler) CreateService(w http.ResponseWriter, r *http.Request) {
	scope := parseScope(r)

	var config models.ServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if config.Name == "" {
		errorResponse(w, http.StatusBadRequest, "Service name is required")
		return
	}
	if config.Program == "" {
		errorResponse(w, http.StatusBadRequest, "Program path is required")
		return
	}

	if err := h.provider.CreateService(config, scope); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]string{
		"status": "created",
		"name":   config.Name,
	})
}

// DeleteService deletes a service
func (h *Handler) DeleteService(w http.ResponseWriter, r *http.Request, name string) {
	scope := parseScope(r)
	if err := h.provider.DeleteService(name, scope); err != nil {
		errorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// extractServiceName extracts the service name from the URL path
// Expects paths like /api/services/{name}/action
func extractServiceName(path string) string {
	// Remove /api/services/ prefix
	path = strings.TrimPrefix(path, "/api/services/")
	// Get everything before the next /
	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
