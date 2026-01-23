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

	"autorun/internal/logger"
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
		logger.Error("failed to get current user", "error", err)
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	uid := u.Uid
	userHome := u.HomeDir
	logger.Debug("launchd provider user info", "uid", uid, "home", userHome)

	// If running as root (e.g., via sudo), get the actual GUI user's UID
	// by checking the owner of /dev/console
	if uid == "0" {
		logger.Debug("running as root, detecting console user")
		cmd := exec.Command("stat", "-f", "%u", "/dev/console")
		if output, err := cmd.Output(); err == nil {
			consoleUID := strings.TrimSpace(string(output))
			if consoleUID != "" && consoleUID != "0" {
				uid = consoleUID
				logger.Debug("detected console user", "uid", uid)
				// Also get the correct home directory for this user
				if guiUser, err := user.LookupId(uid); err == nil {
					userHome = guiUser.HomeDir
					logger.Debug("resolved user home", "home", userHome)
				}
			}
		}
	}

	return &LaunchdProvider{
		userHome: userHome,
		uid:      uid,
	}, nil
}

func (p *LaunchdProvider) Name() string {
	return "launchd"
}

// launchdEntry represents a parsed line from a launchctl domain services listing
// (launchctl print <domain>)
type launchdEntry struct {
	pid   int    // 0 if not running/unknown
	label string // service label
}

// parseLaunchctlPrintServices parses the "services = { ... }" block of
// `launchctl print <domain>` output.
func parseLaunchctlPrintServices(output string) []launchdEntry {
	var entries []launchdEntry

	scanner := bufio.NewScanner(strings.NewReader(output))
	inServices := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inServices {
			if trimmed == "services = {" {
				inServices = true
			}
			continue
		}

		if trimmed == "}" {
			break
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		entries = append(entries, launchdEntry{
			pid:   pid,
			label: fields[2],
		})
	}

	return entries
}

func (p *LaunchdProvider) listDomainServices(domain string) ([]launchdEntry, error) {
	logger.Debug("listing domain services", "domain", domain)
	cmd := exec.Command("launchctl", "print", domain)
	output, err := cmd.Output()
	if err != nil {
		logger.Error("launchctl print failed", "domain", domain, "error", err)
		return nil, fmt.Errorf("launchctl print %s failed: %w", domain, err)
	}

	entries := parseLaunchctlPrintServices(string(output))
	logger.Debug("parsed domain services", "domain", domain, "count", len(entries))
	return entries, nil
}

// listDisabledServices returns a map of label -> disabled for the domain.
// If the command fails, an empty map is returned.
func (p *LaunchdProvider) listDisabledServices(domain string) map[string]bool {
	cmd := exec.Command("launchctl", "print-disabled", domain)
	output, err := cmd.Output()
	if err != nil {
		return map[string]bool{}
	}

	result := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// lines look like: "com.apple.foo" => disabled
		parts := strings.Split(line, "=>")
		if len(parts) != 2 {
			continue
		}

		label := strings.TrimSpace(parts[0])
		label = strings.Trim(label, "\"")

		state := strings.TrimSpace(parts[1])
		state = strings.TrimSuffix(state, ",")
		state = strings.TrimSpace(state)

		if label == "" {
			continue
		}

		result[label] = (state == "disabled")
	}

	return result
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
	var domainTarget string
	switch scope {
	case models.ScopeUser:
		domainTarget = fmt.Sprintf("gui/%s", p.uid)
	case models.ScopeSystem:
		domainTarget = "system"
	default:
		return nil, fmt.Errorf("invalid scope: %s", scope)
	}

	entries, err := p.listDomainServices(domainTarget)
	if err != nil {
		return nil, err
	}

	// Map of running state by label for this domain.
	runningByLabel := make(map[string]bool)
	for _, entry := range entries {
		if entry.pid > 0 {
			runningByLabel[entry.label] = true
		}
	}

	// Launchd doesn't have a single query that returns "enabled" for every service
	// the way systemd does. We approximate enabled/disabled using
	// `launchctl print-disabled <domain>` and fall back to filesystem presence.
	disabledByLabel := p.listDisabledServices(domainTarget)

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

	// Only show services that have plist files in known directories
	services := make([]models.Service, 0, len(knownLabels))
	for label := range knownLabels {
		status := models.StatusStopped
		if runningByLabel[label] {
			status = models.StatusRunning
		}

		enabled := knownLabels[label]
		if disabled, ok := disabledByLabel[label]; ok {
			enabled = !disabled
		}

		services = append(services, models.Service{
			Name:        label,
			DisplayName: label,
			Status:      status,
			Enabled:     enabled,
			Scope:       scope,
		})
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
	logger.Debug("starting service", "name", name, "scope", scope)

	plistPath := p.findPlistForLabel(name, scope)
	if plistPath == "" {
		logger.Error("plist not found", "name", name, "scope", scope)
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
	logger.Debug("attempting bootstrap", "domain", domainTarget, "plist", plistPath)
	cmd := exec.Command("launchctl", "bootstrap", domainTarget, plistPath)
	bootstrapErr := cmd.Run()
	if bootstrapErr != nil {
		logger.Debug("bootstrap failed (may already be loaded)", "error", bootstrapErr)
	}

	// If bootstrap succeeded or service already loaded, try to kickstart it
	// kickstart -k will kill any existing instance and restart
	logger.Debug("attempting kickstart", "target", serviceTarget)
	cmd = exec.Command("launchctl", "kickstart", "-k", serviceTarget)
	if err := cmd.Run(); err != nil {
		logger.Debug("kickstart failed", "error", err)
		// If kickstart fails and bootstrap also failed, try legacy load
		if bootstrapErr != nil {
			logger.Debug("attempting legacy load", "plist", plistPath)
			cmd = exec.Command("launchctl", "load", plistPath)
			if err := cmd.Run(); err != nil {
				logger.Error("all start methods failed", "name", name, "error", err)
				return fmt.Errorf("failed to start service: %w", err)
			}
			// After legacy load, try kickstart again
			cmd = exec.Command("launchctl", "kickstart", serviceTarget)
			cmd.Run() // Ignore error, load may have started it
		}
	}

	logger.Debug("service started", "name", name)
	return nil
}

func (p *LaunchdProvider) Stop(name string, scope models.Scope) error {
	logger.Debug("stopping service", "name", name, "scope", scope)

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
		logger.Debug("attempting bootout", "target", serviceTarget)
		cmd := exec.Command("launchctl", "bootout", serviceTarget)
		if err := cmd.Run(); err == nil {
			logger.Debug("service stopped via bootout", "name", name)
			return nil
		}
		logger.Debug("bootout failed, trying alternatives")
	}

	// Fallback: try kill
	logger.Debug("attempting kill", "target", serviceTarget)
	cmd := exec.Command("launchctl", "kill", "SIGTERM", serviceTarget)
	if err := cmd.Run(); err != nil {
		logger.Debug("kill failed", "error", err)
		// Final fallback: legacy unload
		if plistPath != "" {
			logger.Debug("attempting legacy unload", "plist", plistPath)
			cmd = exec.Command("launchctl", "unload", plistPath)
			return cmd.Run()
		}
		logger.Error("all stop methods failed", "name", name, "error", err)
		return fmt.Errorf("failed to stop service: %w", err)
	}
	logger.Debug("service stopped", "name", name)
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
	logger.Debug("creating service", "name", config.Name, "program", config.Program, "scope", scope)

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

	logger.Debug("target directory", "dir", targetDir)

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		logger.Error("failed to create directory", "dir", targetDir, "error", err)
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	// Check if service already exists
	plistPath := filepath.Join(targetDir, config.Name+".plist")
	if _, err := os.Stat(plistPath); err == nil {
		logger.Warn("service already exists", "name", config.Name, "path", plistPath)
		return fmt.Errorf("service %s already exists", config.Name)
	}

	// Generate the plist content
	plist := p.generatePlist(config)

	// Write the plist file
	logger.Debug("writing plist", "path", plistPath)
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		logger.Error("failed to write plist", "path", plistPath, "error", err)
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	// Load the service if RunAtLoad is set
	if config.RunAtLoad {
		logger.Debug("starting service after creation", "name", config.Name)
		return p.Start(config.Name, scope)
	}

	logger.Debug("service created", "name", config.Name)
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
	logger.Debug("deleting service", "name", name, "scope", scope)

	plistPath := p.findPlistForLabel(name, scope)
	if plistPath == "" {
		logger.Error("service not found for deletion", "name", name, "scope", scope)
		return fmt.Errorf("service not found: %s", name)
	}

	// Stop the service first (ignore errors if not running)
	logger.Debug("stopping service before deletion", "name", name)
	_ = p.Stop(name, scope)

	// Disable the service
	logger.Debug("disabling service before deletion", "name", name)
	_ = p.Disable(name, scope)

	// Delete the plist file
	logger.Debug("removing plist file", "path", plistPath)
	if err := os.Remove(plistPath); err != nil {
		logger.Error("failed to delete plist", "path", plistPath, "error", err)
		return fmt.Errorf("failed to delete service file: %w", err)
	}

	logger.Debug("service deleted", "name", name)
	return nil
}
