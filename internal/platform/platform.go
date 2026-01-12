package platform

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"autorun/internal/models"
)

// ServiceProvider defines the interface for platform-specific service management
type ServiceProvider interface {
	// Name returns the platform name (e.g., "systemd", "launchd")
	Name() string

	// ListServices returns all services for the given scope
	ListServices(scope models.Scope) ([]models.Service, error)

	// GetService returns details for a specific service
	GetService(name string, scope models.Scope) (*models.Service, error)

	// Start starts a service
	Start(name string, scope models.Scope) error

	// Stop stops a service
	Stop(name string, scope models.Scope) error

	// Restart restarts a service
	Restart(name string, scope models.Scope) error

	// Enable enables a service to start at boot
	Enable(name string, scope models.Scope) error

	// Disable disables a service from starting at boot
	Disable(name string, scope models.Scope) error

	// StreamLogs returns a channel that streams log lines for a service
	StreamLogs(ctx context.Context, name string, scope models.Scope) (<-chan string, error)

	// CreateService creates a new service with the given configuration
	CreateService(config models.ServiceConfig, scope models.Scope) error

	// DeleteService removes a service
	DeleteService(name string, scope models.Scope) error
}

// Detect detects the current platform and returns the appropriate ServiceProvider
func Detect() (ServiceProvider, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewLaunchdProvider()
	case "linux":
		// Check if systemd is available
		if _, err := os.Stat("/run/systemd/system"); err == nil {
			return NewSystemdProvider()
		}
		return nil, fmt.Errorf("systemd not detected on this Linux system")
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
