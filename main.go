package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"autorun/internal/api"
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
	flag.Parse()

	// Find an available port starting from the specified port
	actualPort, err := findAvailablePort(*listen, *port, 100)
	if err != nil {
		log.Fatalf("Failed to find available port: %v", err)
	}
	if actualPort != *port {
		log.Printf("Port %d is in use, using port %d instead", *port, actualPort)
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
		log.Fatalf("Failed to detect platform: %v", err)
	}

	log.Printf("Detected platform: %s", provider.Name())

	// Get embedded frontend
	frontendFS, err := GetFrontendFS()
	if err != nil {
		log.Fatalf("Failed to load frontend: %v", err)
	}

	// Create router
	router := api.NewRouter(provider, frontendFS)

	// Start server
	addr := fmt.Sprintf("%s:%d", *listen, actualPort)
	log.Printf("Starting server at http://%s", addr)

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
		log.Printf("Shutting down (signal: %s)", sig)
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
		if err := srv.Close(); err != nil {
			log.Printf("Server close failed: %v", err)
		}
	}

	if err := <-serverErr; err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
