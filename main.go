package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"autorun/internal/api"
	"autorun/internal/platform"
)

func main() {
	port := flag.Int("port", 8080, "Port to listen on")
	listen := flag.String("listen", "127.0.0.1", "Address to bind to")
	flag.Parse()

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
	addr := fmt.Sprintf("%s:%d", *listen, *port)
	log.Printf("Starting server at http://%s", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
