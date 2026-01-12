package models

// Scope represents whether a service is system-level or user-level
type Scope string

const (
	ScopeSystem Scope = "system"
	ScopeUser   Scope = "user"
)

// Service represents a managed service
type Service struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"` // running, stopped, failed, unknown
	Enabled     bool   `json:"enabled"`
	Scope       Scope  `json:"scope"`
	Description string `json:"description,omitempty"`
}

// Status constants
const (
	StatusRunning = "running"
	StatusStopped = "stopped"
	StatusFailed  = "failed"
	StatusUnknown = "unknown"
)

// ServiceConfig holds the configuration for creating a new service
type ServiceConfig struct {
	Name             string            `json:"name"`             // Service name/label (required)
	Description      string            `json:"description"`      // Human-readable description
	Program          string            `json:"program"`          // Executable path (required)
	Arguments        []string          `json:"arguments"`        // Command line arguments
	WorkingDirectory string            `json:"workingDirectory"` // Working directory for the service
	Environment      map[string]string `json:"environment"`      // Environment variables
	RunAtLoad        bool              `json:"runAtLoad"`        // Start service when loaded/enabled
	KeepAlive        bool              `json:"keepAlive"`        // Restart if it exits
	StandardOutPath  string            `json:"standardOutPath"`  // Path for stdout log
	StandardErrorPath string           `json:"standardErrorPath"` // Path for stderr log
}
