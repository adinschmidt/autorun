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
