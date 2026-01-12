package platform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

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

	cmd := exec.Command("systemctl", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl list-units failed: %w", err)
	}

	var units []systemdUnit
	if err := json.Unmarshal(output, &units); err != nil {
		return nil, fmt.Errorf("failed to parse systemctl output: %w", err)
	}

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
	cmd := exec.Command("systemctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s failed: %s", action, string(output))
	}
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

	cmd := exec.CommandContext(ctx, "journalctl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start journalctl: %w", err)
	}

	go func() {
		defer close(ch)
		defer cmd.Wait()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case ch <- scanner.Text():
			}
		}
	}()

	return ch, nil
}
