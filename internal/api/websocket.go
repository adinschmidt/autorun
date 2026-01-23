package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"autorun/internal/logger"
	"autorun/internal/models"
	"autorun/internal/platform"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for localhost usage
	},
}

// LogStreamer handles WebSocket connections for log streaming
type LogStreamer struct {
	provider platform.ServiceProvider
}

// NewLogStreamer creates a new log streamer
func NewLogStreamer(provider platform.ServiceProvider) *LogStreamer {
	return &LogStreamer{provider: provider}
}

// HandleLogStream handles WebSocket connections for streaming logs
func (ls *LogStreamer) HandleLogStream(w http.ResponseWriter, r *http.Request, serviceName string) {
	scope := models.ScopeUser
	if r.URL.Query().Get("scope") == "system" {
		scope = models.ScopeSystem
	}

	logger.Debug("websocket log stream requested", "service", serviceName, "scope", scope)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("websocket upgrade failed", "service", serviceName, "error", err)
		return
	}
	defer conn.Close()

	logger.Info("websocket connected", "service", serviceName, "scope", scope)

	// Create a context that cancels when the connection closes
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Handle client disconnect
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				logger.Debug("websocket client disconnected", "service", serviceName)
				cancel()
				return
			}
		}
	}()

	// Start log streaming
	logCh, err := ls.provider.StreamLogs(ctx, serviceName, scope)
	if err != nil {
		logger.Error("failed to start log stream", "service", serviceName, "scope", scope, "error", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		return
	}

	// Send an initial message
	conn.WriteMessage(websocket.TextMessage, []byte("--- Connected to log stream for "+serviceName+" ---"))

	// Stream logs to the WebSocket
	for {
		select {
		case <-ctx.Done():
			logger.Debug("websocket stream ended", "service", serviceName, "reason", "context cancelled")
			return
		case line, ok := <-logCh:
			if !ok {
				logger.Debug("websocket stream ended", "service", serviceName, "reason", "channel closed")
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
				logger.Debug("websocket write failed", "service", serviceName, "error", err)
				return
			}
		}
	}
}
