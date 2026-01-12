package platform

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"autorun/internal/models"
)

// LaunchdProvider implements ServiceProvider for macOS launchd
type LaunchdProvider struct {
	userHome string
	uid      string
}

// NewLaunchdProvider creates a new launchd provider
func NewLaunchdProvider() (*LaunchdProvider, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	return &LaunchdProvider{
		userHome: u.HomeDir,
		uid:      u.Uid,
	}, nil
}

func (p *LaunchdProvider) Name() string {
	return "launchd"
}

// launchdEntry represents a parsed line from launchctl list
type launchdEntry struct {
	pid    int    // -1 if not running
	status int    // exit status, 0 if running
	label  string // service label
}

// parseLaunchctlList parses the output of launchctl list
func parseLaunchctlList(output string) []launchdEntry {
	var entries []launchdEntry
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		// Skip header line
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		entry := launchdEntry{label: fields[2]}

		// Parse PID
		if fields[0] == "-" {
			entry.pid = -1
		} else {
			pid, _ := strconv.Atoi(fields[0])
			entry.pid = pid
		}

		// Parse status
		if fields[1] == "-" {
			entry.status = 0
		} else {
			status, _ := strconv.Atoi(fields[1])
			entry.status = status
		}

		entries = append(entries, entry)
	}

	return entries
}

// getServiceDirs returns the directories to search for plist files
func (p *LaunchdProvider) getServiceDirs(scope models.Scope) []string {
	switch scope {
	case models.ScopeUser:
		return []string{
			filepath.Join(p.userHome, "Library", "LaunchAgents"),
			"/Library/LaunchAgents",
		}
	case models.ScopeSystem:
		return []string{
			"/Library/LaunchDaemons",
			"/System/Library/LaunchDaemons",
		}
	default:
		return nil
	}
}

// findPlistForLabel searches for a plist file matching the label
func (p *LaunchdProvider) findPlistForLabel(label string, scope models.Scope) string {
	dirs := p.getServiceDirs(scope)
	for _, dir := range dirs {
		plistPath := filepath.Join(dir, label+".plist")
		if _, err := os.Stat(plistPath); err == nil {
			return plistPath
		}
	}
	return ""
}

func (p *LaunchdProvider) ListServices(scope models.Scope) ([]models.Service, error) {
	var cmd *exec.Cmd

	switch scope {
	case models.ScopeUser:
		cmd = exec.Command("launchctl", "list")
	case models.ScopeSystem:
		cmd = exec.Command("launchctl", "list")
	default:
		return nil, fmt.Errorf("invalid scope: %s", scope)
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("launchctl list failed: %w", err)
	}

	entries := parseLaunchctlList(string(output))

	// Build a set of known labels from plist files in relevant directories
	knownLabels := make(map[string]bool)
	dirs := p.getServiceDirs(scope)
	for _, dir := range dirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".plist") {
				label := strings.TrimSuffix(f.Name(), ".plist")
				knownLabels[label] = true
			}
		}
	}

	var services []models.Service
	for _, entry := range entries {
		// For user scope, include services that have plist files in user directories
		// For system scope, include services from system directories
		// Also filter out Apple system services (com.apple.*) for cleaner output
		hasPllist := knownLabels[entry.label]

		// Skip services without plist files in our known directories
		if !hasPllist {
			continue
		}

		status := models.StatusStopped
		if entry.pid > 0 {
			status = models.StatusRunning
		} else if entry.status != 0 {
			status = models.StatusFailed
		}

		services = append(services, models.Service{
			Name:        entry.label,
			DisplayName: entry.label,
			Status:      status,
			Enabled:     true, // launchd services are enabled if they're loaded
			Scope:       scope,
		})
	}

	// Also add services that have plist files but aren't loaded
	for label := range knownLabels {
		found := false
		for _, svc := range services {
			if svc.Name == label {
				found = true
				break
			}
		}
		if !found {
			services = append(services, models.Service{
				Name:        label,
				DisplayName: label,
				Status:      models.StatusStopped,
				Enabled:     false,
				Scope:       scope,
			})
		}
	}

	return services, nil
}

func (p *LaunchdProvider) GetService(name string, scope models.Scope) (*models.Service, error) {
	services, err := p.ListServices(scope)
	if err != nil {
		return nil, err
	}

	for _, svc := range services {
		if svc.Name == name {
			return &svc, nil
		}
	}

	return nil, fmt.Errorf("service not found: %s", name)
}

func (p *LaunchdProvider) Start(name string, scope models.Scope) error {
	plistPath := p.findPlistForLabel(name, scope)
	if plistPath == "" {
		return fmt.Errorf("plist not found for service: %s", name)
	}

	var domainTarget string
	if scope == models.ScopeUser {
		domainTarget = fmt.Sprintf("gui/%s", p.uid)
	} else {
		domainTarget = "system"
	}
	serviceTarget := fmt.Sprintf("%s/%s", domainTarget, name)

	// Try modern bootstrap first (macOS 10.10+)
	// bootstrap loads the service into the domain
	cmd := exec.Command("launchctl", "bootstrap", domainTarget, plistPath)
	bootstrapErr := cmd.Run()

	// If bootstrap succeeded or service already loaded, try to kickstart it
	// kickstart -k will kill any existing instance and restart
	cmd = exec.Command("launchctl", "kickstart", "-k", serviceTarget)
	if err := cmd.Run(); err != nil {
		// If kickstart fails and bootstrap also failed, try legacy load
		if bootstrapErr != nil {
			cmd = exec.Command("launchctl", "load", plistPath)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to start service: %w", err)
			}
			// After legacy load, try kickstart again
			cmd = exec.Command("launchctl", "kickstart", serviceTarget)
			cmd.Run() // Ignore error, load may have started it
		}
	}

	return nil
}

func (p *LaunchdProvider) Stop(name string, scope models.Scope) error {
	var domainTarget string
	if scope == models.ScopeUser {
		domainTarget = fmt.Sprintf("gui/%s", p.uid)
	} else {
		domainTarget = "system"
	}
	serviceTarget := fmt.Sprintf("%s/%s", domainTarget, name)

	// Try modern bootout first (opposite of bootstrap)
	plistPath := p.findPlistForLabel(name, scope)
	if plistPath != "" {
		cmd := exec.Command("launchctl", "bootout", serviceTarget)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback: try kill
	cmd := exec.Command("launchctl", "kill", "SIGTERM", serviceTarget)
	if err := cmd.Run(); err != nil {
		// Final fallback: legacy unload
		if plistPath != "" {
			cmd = exec.Command("launchctl", "unload", plistPath)
			return cmd.Run()
		}
		return fmt.Errorf("failed to stop service: %w", err)
	}
	return nil
}

func (p *LaunchdProvider) Restart(name string, scope models.Scope) error {
	if err := p.Stop(name, scope); err != nil {
		// Ignore stop errors, service might not be running
	}
	return p.Start(name, scope)
}

func (p *LaunchdProvider) Enable(name string, scope models.Scope) error {
	plistPath := p.findPlistForLabel(name, scope)
	if plistPath == "" {
		return fmt.Errorf("plist not found for service: %s", name)
	}

	cmd := exec.Command("launchctl", "load", "-w", plistPath)
	return cmd.Run()
}

func (p *LaunchdProvider) Disable(name string, scope models.Scope) error {
	plistPath := p.findPlistForLabel(name, scope)
	if plistPath == "" {
		return fmt.Errorf("plist not found for service: %s", name)
	}

	cmd := exec.Command("launchctl", "unload", "-w", plistPath)
	return cmd.Run()
}

// getProcessNameForService extracts the program/process name from a plist file
// Returns the basename of the executable, or falls back to the last component of the service label
func (p *LaunchdProvider) getProcessNameForService(name string, scope models.Scope) string {
	plistPath := p.findPlistForLabel(name, scope)
	if plistPath == "" {
		// Fallback: use last component of service label
		parts := strings.Split(name, ".")
		return parts[len(parts)-1]
	}

	// Try to read the plist and extract Program or ProgramArguments
	// Use plutil to convert to xml and parse
	cmd := exec.Command("plutil", "-convert", "xml1", "-o", "-", plistPath)
	output, err := cmd.Output()
	if err != nil {
		parts := strings.Split(name, ".")
		return parts[len(parts)-1]
	}

	content := string(output)

	// Look for <key>Program</key> or <key>ProgramArguments</key>
	// Simple string parsing to find the program path
	var programPath string

	// Check for Program key first
	if idx := strings.Index(content, "<key>Program</key>"); idx != -1 {
		// Find the next <string> element
		rest := content[idx:]
		if start := strings.Index(rest, "<string>"); start != -1 {
			rest = rest[start+8:]
			if end := strings.Index(rest, "</string>"); end != -1 {
				programPath = rest[:end]
			}
		}
	}

	// If no Program, try ProgramArguments (first element is the executable)
	if programPath == "" {
		if idx := strings.Index(content, "<key>ProgramArguments</key>"); idx != -1 {
			rest := content[idx:]
			if start := strings.Index(rest, "<string>"); start != -1 {
				rest = rest[start+8:]
				if end := strings.Index(rest, "</string>"); end != -1 {
					programPath = rest[:end]
				}
			}
		}
	}

	if programPath != "" {
		// Return just the basename
		return filepath.Base(programPath)
	}

	// Fallback: use last component of service label
	parts := strings.Split(name, ".")
	return parts[len(parts)-1]
}

func (p *LaunchdProvider) StreamLogs(ctx context.Context, name string, scope models.Scope) (<-chan string, error) {
	ch := make(chan string, 100)

	// Get the program name from the plist to use in log filtering
	processName := p.getProcessNameForService(name, scope)

	// Use log stream with predicate to filter by process name
	// We use CONTAINS for more flexible matching since process names may vary
	predicate := fmt.Sprintf("process == '%s' OR process CONTAINS '%s' OR subsystem CONTAINS '%s'",
		processName, processName, name)
	cmd := exec.CommandContext(ctx, "log", "stream",
		"--predicate", predicate,
		"--style", "compact")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start log stream: %w", err)
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

// CreateService creates a new launchd service with the given configuration
func (p *LaunchdProvider) CreateService(config models.ServiceConfig, scope models.Scope) error {
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
		targetDir = filepath.Join(p.userHome, "Library", "LaunchAgents")
	case models.ScopeSystem:
		targetDir = "/Library/LaunchDaemons"
	default:
		return fmt.Errorf("invalid scope: %s", scope)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	// Check if service already exists
	plistPath := filepath.Join(targetDir, config.Name+".plist")
	if _, err := os.Stat(plistPath); err == nil {
		return fmt.Errorf("service %s already exists", config.Name)
	}

	// Generate the plist content
	plist := p.generatePlist(config)

	// Write the plist file
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	// Load the service if RunAtLoad is set
	if config.RunAtLoad {
		return p.Start(config.Name, scope)
	}

	return nil
}

// generatePlist creates the XML plist content for a service configuration
func (p *LaunchdProvider) generatePlist(config models.ServiceConfig) string {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>`)
	sb.WriteString(escapeXML(config.Name))
	sb.WriteString(`</string>
`)

	// Program and arguments
	if len(config.Arguments) > 0 {
		sb.WriteString(`	<key>ProgramArguments</key>
	<array>
		<string>`)
		sb.WriteString(escapeXML(config.Program))
		sb.WriteString(`</string>
`)
		for _, arg := range config.Arguments {
			sb.WriteString(`		<string>`)
			sb.WriteString(escapeXML(arg))
			sb.WriteString(`</string>
`)
		}
		sb.WriteString(`	</array>
`)
	} else {
		sb.WriteString(`	<key>Program</key>
	<string>`)
		sb.WriteString(escapeXML(config.Program))
		sb.WriteString(`</string>
`)
	}

	// Working directory
	if config.WorkingDirectory != "" {
		sb.WriteString(`	<key>WorkingDirectory</key>
	<string>`)
		sb.WriteString(escapeXML(config.WorkingDirectory))
		sb.WriteString(`</string>
`)
	}

	// Environment variables
	if len(config.Environment) > 0 {
		sb.WriteString(`	<key>EnvironmentVariables</key>
	<dict>
`)
		for key, value := range config.Environment {
			sb.WriteString(`		<key>`)
			sb.WriteString(escapeXML(key))
			sb.WriteString(`</key>
		<string>`)
			sb.WriteString(escapeXML(value))
			sb.WriteString(`</string>
`)
		}
		sb.WriteString(`	</dict>
`)
	}

	// RunAtLoad
	sb.WriteString(`	<key>RunAtLoad</key>
	<`)
	if config.RunAtLoad {
		sb.WriteString("true")
	} else {
		sb.WriteString("false")
	}
	sb.WriteString(`/>
`)

	// KeepAlive
	if config.KeepAlive {
		sb.WriteString(`	<key>KeepAlive</key>
	<true/>
`)
	}

	// Standard output path
	if config.StandardOutPath != "" {
		sb.WriteString(`	<key>StandardOutPath</key>
	<string>`)
		sb.WriteString(escapeXML(config.StandardOutPath))
		sb.WriteString(`</string>
`)
	}

	// Standard error path
	if config.StandardErrorPath != "" {
		sb.WriteString(`	<key>StandardErrorPath</key>
	<string>`)
		sb.WriteString(escapeXML(config.StandardErrorPath))
		sb.WriteString(`</string>
`)
	}

	sb.WriteString(`</dict>
</plist>
`)

	return sb.String()
}

// escapeXML escapes special characters for XML
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// DeleteService removes a launchd service
func (p *LaunchdProvider) DeleteService(name string, scope models.Scope) error {
	plistPath := p.findPlistForLabel(name, scope)
	if plistPath == "" {
		return fmt.Errorf("service not found: %s", name)
	}

	// Stop the service first (ignore errors if not running)
	_ = p.Stop(name, scope)

	// Disable the service
	_ = p.Disable(name, scope)

	// Delete the plist file
	if err := os.Remove(plistPath); err != nil {
		return fmt.Errorf("failed to delete service file: %w", err)
	}

	return nil
}
