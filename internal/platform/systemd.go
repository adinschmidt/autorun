package platform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"autorun/internal/logger"
	"autorun/internal/models"
)

// SystemdProvider implements ServiceProvider for Linux systemd
type SystemdProvider struct{}

// NewSystemdProvider creates a new systemd provider
func NewSystemdProvider() (*SystemdProvider, error) {
	return &SystemdProvider{}, nil
}

func (p *SystemdProvider) Name() string {
	return "systemd"
}

// systemdUnit represents a unit from systemctl list-units --output=json
type systemdUnit struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

func (p *SystemdProvider) listUnits(scope models.Scope) ([]systemdUnit, error) {
	var args []string

	if scope == models.ScopeUser {
		args = append(args, "--user")
	}
	args = append(args, "list-units", "--type=service", "--all", "--output=json")

	logger.Debug("executing systemctl", "args", args)
	cmd := exec.Command("systemctl", args...)
	output, err := cmd.Output()
	if err != nil {
		// Get stderr for more details
		if exitErr, ok := err.(*exec.ExitError); ok {
			logger.Error("systemctl list-units failed", "scope", scope, "error", err, "stderr", string(exitErr.Stderr))
		} else {
			logger.Error("systemctl list-units failed", "scope", scope, "error", err)
		}
		return nil, fmt.Errorf("systemctl list-units failed: %w", err)
	}

	var units []systemdUnit
	if err := json.Unmarshal(output, &units); err != nil {
		logger.Error("failed to parse systemctl output", "error", err, "output", string(output[:min(len(output), 200)]))
		return nil, fmt.Errorf("failed to parse systemctl output: %w", err)
	}

	logger.Debug("listed units", "scope", scope, "count", len(units))
	return units, nil
}

func (p *SystemdProvider) isEnabled(name string, scope models.Scope) bool {
	var args []string
	if scope == models.ScopeUser {
		args = append(args, "--user")
	}
	args = append(args, "is-enabled", name)

	cmd := exec.Command("systemctl", args...)
	output, _ := cmd.Output()
	return strings.TrimSpace(string(output)) == "enabled"
}

func (p *SystemdProvider) ListServices(scope models.Scope) ([]models.Service, error) {
	units, err := p.listUnits(scope)
	if err != nil {
		return nil, err
	}

	var services []models.Service
	for _, unit := range units {
		// Extract service name without .service suffix
		name := unit.Unit
		if strings.HasSuffix(name, ".service") {
			name = strings.TrimSuffix(name, ".service")
		}

		status := models.StatusUnknown
		switch unit.Active {
		case "active":
			if unit.Sub == "running" {
				status = models.StatusRunning
			} else {
				status = models.StatusStopped
			}
		case "inactive":
			status = models.StatusStopped
		case "failed":
			status = models.StatusFailed
		}

		services = append(services, models.Service{
			Name:        name,
			DisplayName: name,
			Status:      status,
			Enabled:     p.isEnabled(unit.Unit, scope),
			Scope:       scope,
			Description: unit.Description,
		})
	}

	return services, nil
}

func (p *SystemdProvider) GetService(name string, scope models.Scope) (*models.Service, error) {
	services, err := p.ListServices(scope)
	if err != nil {
		return nil, err
	}

	for _, svc := range services {
		if svc.Name == name || svc.Name+".service" == name {
			return &svc, nil
		}
	}

	return nil, fmt.Errorf("service not found: %s", name)
}

func (p *SystemdProvider) runSystemctl(action, name string, scope models.Scope) error {
	var args []string
	if scope == models.ScopeUser {
		args = append(args, "--user")
	}

	// Ensure .service suffix
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	args = append(args, action, name)
	logger.Debug("executing systemctl", "action", action, "name", name, "args", args)
	cmd := exec.Command("systemctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Error("systemctl command failed", "action", action, "name", name, "scope", scope, "error", err, "output", string(output))
		return fmt.Errorf("systemctl %s failed: %s", action, string(output))
	}
	logger.Debug("systemctl command succeeded", "action", action, "name", name)
	return nil
}

func (p *SystemdProvider) Start(name string, scope models.Scope) error {
	return p.runSystemctl("start", name, scope)
}

func (p *SystemdProvider) Stop(name string, scope models.Scope) error {
	return p.runSystemctl("stop", name, scope)
}

func (p *SystemdProvider) Restart(name string, scope models.Scope) error {
	return p.runSystemctl("restart", name, scope)
}

func (p *SystemdProvider) Enable(name string, scope models.Scope) error {
	return p.runSystemctl("enable", name, scope)
}

func (p *SystemdProvider) Disable(name string, scope models.Scope) error {
	return p.runSystemctl("disable", name, scope)
}

func (p *SystemdProvider) StreamLogs(ctx context.Context, name string, scope models.Scope) (<-chan string, error) {
	ch := make(chan string, 100)

	var args []string
	args = append(args, "-f", "-n", "100") // Follow, last 100 lines

	if scope == models.ScopeUser {
		args = append(args, "--user-unit", name+".service")
	} else {
		args = append(args, "-u", name+".service")
	}

	logger.Debug("starting journalctl", "args", args)
	cmd := exec.CommandContext(ctx, "journalctl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error("failed to create stdout pipe", "error", err)
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		logger.Error("failed to start journalctl", "name", name, "scope", scope, "error", err)
		return nil, fmt.Errorf("failed to start journalctl: %w", err)
	}

	logger.Debug("journalctl started", "name", name, "scope", scope)

	go func() {
		defer close(ch)
		defer cmd.Wait()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				logger.Debug("log stream context cancelled", "name", name)
				return
			case ch <- scanner.Text():
			}
		}
		logger.Debug("log stream ended", "name", name)
	}()

	return ch, nil
}

// CreateService creates a new systemd service with the given configuration
func (p *SystemdProvider) CreateService(config models.ServiceConfig, scope models.Scope) error {
	logger.Debug("creating systemd service", "name", config.Name, "program", config.Program, "scope", scope)

	if config.Name == "" {
		return fmt.Errorf("service name is required")
	}
	if config.Program == "" {
		return fmt.Errorf("program path is required")
	}

	// Determine the target directory
	var targetDir string
	switch scope {
	case models.ScopeUser:
		u, err := user.Current()
		if err != nil {
			logger.Error("failed to get current user", "error", err)
			return fmt.Errorf("failed to get current user: %w", err)
		}
		targetDir = filepath.Join(u.HomeDir, ".config", "systemd", "user")
	case models.ScopeSystem:
		targetDir = "/etc/systemd/system"
	default:
		return fmt.Errorf("invalid scope: %s", scope)
	}

	logger.Debug("target directory", "dir", targetDir)

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		logger.Error("failed to create directory", "dir", targetDir, "error", err)
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	// Service name for file
	serviceName := config.Name
	if !strings.HasSuffix(serviceName, ".service") {
		serviceName = serviceName + ".service"
	}

	// Check if service already exists
	unitPath := filepath.Join(targetDir, serviceName)
	if _, err := os.Stat(unitPath); err == nil {
		logger.Warn("service already exists", "name", config.Name, "path", unitPath)
		return fmt.Errorf("service %s already exists", config.Name)
	}

	// Generate the unit file content
	unitContent := p.generateUnitFile(config)

	// Write the unit file
	logger.Debug("writing unit file", "path", unitPath)
	if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
		logger.Error("failed to write unit file", "path", unitPath, "error", err)
		return fmt.Errorf("failed to write unit file: %w", err)
	}

	// Reload systemd to pick up the new unit
	logger.Debug("reloading systemd daemon")
	if err := p.daemonReload(scope); err != nil {
		logger.Error("daemon reload failed, cleaning up", "error", err)
		os.Remove(unitPath)
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable and start the service if RunAtLoad is set
	if config.RunAtLoad {
		logger.Debug("enabling and starting service", "name", config.Name)
		if err := p.Enable(config.Name, scope); err != nil {
			logger.Error("failed to enable service", "name", config.Name, "error", err)
			return fmt.Errorf("failed to enable service: %w", err)
		}
		if err := p.Start(config.Name, scope); err != nil {
			logger.Error("failed to start service", "name", config.Name, "error", err)
			return fmt.Errorf("failed to start service: %w", err)
		}
	}

	logger.Debug("service created successfully", "name", config.Name)
	return nil
}

// generateUnitFile creates the systemd unit file content for a service configuration
func (p *SystemdProvider) generateUnitFile(config models.ServiceConfig) string {
	var sb strings.Builder

	// [Unit] section
	sb.WriteString("[Unit]\n")
	if config.Description != "" {
		sb.WriteString(fmt.Sprintf("Description=%s\n", config.Description))
	} else {
		sb.WriteString(fmt.Sprintf("Description=%s service\n", config.Name))
	}
	sb.WriteString("After=network.target\n")
	sb.WriteString("\n")

	// [Service] section
	sb.WriteString("[Service]\n")
	sb.WriteString("Type=simple\n")

	// ExecStart with program and arguments
	execStart := config.Program
	if len(config.Arguments) > 0 {
		for _, arg := range config.Arguments {
			// Escape spaces in arguments
			if strings.Contains(arg, " ") {
				execStart += fmt.Sprintf(" \"%s\"", arg)
			} else {
				execStart += " " + arg
			}
		}
	}
	sb.WriteString(fmt.Sprintf("ExecStart=%s\n", execStart))

	// Working directory
	if config.WorkingDirectory != "" {
		sb.WriteString(fmt.Sprintf("WorkingDirectory=%s\n", config.WorkingDirectory))
	}

	// Environment variables
	for key, value := range config.Environment {
		sb.WriteString(fmt.Sprintf("Environment=\"%s=%s\"\n", key, value))
	}

	// Restart policy
	if config.KeepAlive {
		sb.WriteString("Restart=always\n")
		sb.WriteString("RestartSec=5\n")
	}

	// Standard output/error
	if config.StandardOutPath != "" {
		sb.WriteString(fmt.Sprintf("StandardOutput=file:%s\n", config.StandardOutPath))
	}
	if config.StandardErrorPath != "" {
		sb.WriteString(fmt.Sprintf("StandardError=file:%s\n", config.StandardErrorPath))
	}

	sb.WriteString("\n")

	// [Install] section
	sb.WriteString("[Install]\n")
	sb.WriteString("WantedBy=default.target\n")

	return sb.String()
}

// daemonReload runs systemctl daemon-reload
func (p *SystemdProvider) daemonReload(scope models.Scope) error {
	var args []string
	if scope == models.ScopeUser {
		args = append(args, "--user")
	}
	args = append(args, "daemon-reload")

	logger.Debug("executing daemon-reload", "args", args)
	cmd := exec.Command("systemctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Error("daemon-reload failed", "scope", scope, "error", err, "output", string(output))
		return fmt.Errorf("daemon-reload failed: %s", string(output))
	}
	logger.Debug("daemon-reload succeeded", "scope", scope)
	return nil
}

// DeleteService removes a systemd service
func (p *SystemdProvider) DeleteService(name string, scope models.Scope) error {
	logger.Debug("deleting systemd service", "name", name, "scope", scope)

	// Determine the target directory
	var targetDir string
	switch scope {
	case models.ScopeUser:
		u, err := user.Current()
		if err != nil {
			logger.Error("failed to get current user", "error", err)
			return fmt.Errorf("failed to get current user: %w", err)
		}
		targetDir = filepath.Join(u.HomeDir, ".config", "systemd", "user")
	case models.ScopeSystem:
		targetDir = "/etc/systemd/system"
	default:
		return fmt.Errorf("invalid scope: %s", scope)
	}

	// Service name for file
	serviceName := name
	if !strings.HasSuffix(serviceName, ".service") {
		serviceName = serviceName + ".service"
	}

	unitPath := filepath.Join(targetDir, serviceName)
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		logger.Error("service not found for deletion", "name", name, "path", unitPath)
		return fmt.Errorf("service not found: %s", name)
	}

	// Stop the service first (ignore errors if not running)
	logger.Debug("stopping service before deletion", "name", name)
	_ = p.Stop(name, scope)

	// Disable the service
	logger.Debug("disabling service before deletion", "name", name)
	_ = p.Disable(name, scope)

	// Delete the unit file
	logger.Debug("removing unit file", "path", unitPath)
	if err := os.Remove(unitPath); err != nil {
		logger.Error("failed to delete unit file", "path", unitPath, "error", err)
		return fmt.Errorf("failed to delete service file: %w", err)
	}

	// Reload systemd
	logger.Debug("reloading systemd daemon")
	if err := p.daemonReload(scope); err != nil {
		logger.Error("daemon reload failed", "error", err)
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	logger.Debug("service deleted successfully", "name", name)
	return nil
}
