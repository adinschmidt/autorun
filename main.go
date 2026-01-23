package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"autorun/internal/api"
	"autorun/internal/logger"
	"autorun/internal/platform"
)

// findAvailablePort finds the first available port starting from startPort.
// It tries up to maxAttempts ports before giving up.
func findAvailablePort(host string, startPort, maxAttempts int) (int, error) {
	for i := 0; i < maxAttempts; i++ {
		port := startPort + i
		addr := fmt.Sprintf("%s:%d", host, port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", startPort, startPort+maxAttempts-1)
}

func main() {
	port := flag.Int("port", 8080, "Starting port to listen on (will auto-increment if in use)")
	listen := flag.String("listen", "127.0.0.1", "Address to bind to")
	verbose := flag.Bool("verbose", false, "Enable debug logging (or set LOG_LEVEL=debug)")
	flag.BoolVar(verbose, "v", false, "Enable debug logging (shorthand)")
	flag.Parse()

	// Initialize logger
	logger.Init(*verbose)

	// Find an available port starting from the specified port
	actualPort, err := findAvailablePort(*listen, *port, 100)
	if err != nil {
		logger.Error("failed to find available port", "error", err)
		os.Exit(1)
	}
	if actualPort != *port {
		logger.Info("port in use, using alternative", "requested", *port, "actual", actualPort)
	}

	// Warn about security implications of non-localhost binding
	if *listen != "127.0.0.1" && *listen != "localhost" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "╔════════════════════════════════════════════════════════════════╗")
		fmt.Fprintln(os.Stderr, "║                        ⚠️  WARNING ⚠️                            ║")
		fmt.Fprintln(os.Stderr, "╠════════════════════════════════════════════════════════════════╣")
		fmt.Fprintln(os.Stderr, "║  You are binding to a non-localhost address!                  ║")
		fmt.Fprintln(os.Stderr, "║                                                               ║")
		fmt.Fprintln(os.Stderr, "║  This exposes service control capabilities to the network.    ║")
		fmt.Fprintln(os.Stderr, "║  Anyone who can reach this address can:                       ║")
		fmt.Fprintln(os.Stderr, "║    - View all system and user services                        ║")
		fmt.Fprintln(os.Stderr, "║    - Start, stop, and restart services                        ║")
		fmt.Fprintln(os.Stderr, "║    - Enable and disable services                              ║")
		fmt.Fprintln(os.Stderr, "║    - View service logs                                        ║")
		fmt.Fprintln(os.Stderr, "║                                                               ║")
		fmt.Fprintln(os.Stderr, "║  There is NO authentication. Use at your own risk.           ║")
		fmt.Fprintln(os.Stderr, "╚════════════════════════════════════════════════════════════════╝")
		fmt.Fprintln(os.Stderr, "")
	}

	// Detect platform and create provider
	provider, err := platform.Detect()
	if err != nil {
		logger.Error("failed to detect platform", "error", err)
		os.Exit(1)
	}

	logger.Info("detected platform", "platform", provider.Name())

	// Get embedded frontend
	frontendFS, err := GetFrontendFS()
	if err != nil {
		logger.Error("failed to load frontend", "error", err)
		os.Exit(1)
	}

	// Create router
	router := api.NewRouter(provider, frontendFS)

	// Start server
	addr := fmt.Sprintf("%s:%d", *listen, actualPort)
	logger.Info("starting server", "address", fmt.Sprintf("http://%s", addr))

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("shutting down", "signal", sig)
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Warn("graceful shutdown failed", "error", err)
		if err := srv.Close(); err != nil {
			logger.Error("server close failed", "error", err)
		}
	}

	if err := <-serverErr; err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
