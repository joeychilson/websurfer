package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/logger"
)

func TestServerHealth(t *testing.T) {
	cfg := config.New()
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	server, err := NewServer(c, logger.Noop(), nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleHealth() status = %d, want %d", w.Code, http.StatusOK)
	}

	var health map[string]string
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if health["status"] != "ok" {
		t.Errorf("health status = %q, want %q", health["status"], "ok")
	}

	if health["time"] == "" {
		t.Error("health time should not be empty")
	}
}

func TestServerFetch(t *testing.T) {
	cfg := config.New()
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	server, err := NewServer(c, logger.Noop(), nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/fetch", bytes.NewBufferString("invalid json"))
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("handleFetch() status = %d, want %d", w.Code, http.StatusBadRequest)
		}

		var errResp ErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}

		if errResp.Error == "" {
			t.Error("error message should not be empty")
		}
	})

	t.Run("empty URL", func(t *testing.T) {
		reqBody := FetchRequest{
			URL: "",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/fetch", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("handleFetch() status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestServerWithNilLogger(t *testing.T) {
	cfg := config.New()
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	server, err := NewServer(c, nil, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	if server.logger == nil {
		t.Error("server logger should not be nil when created with nil logger")
	}
}

func TestServerStartWithShutdown(t *testing.T) {
	cfg := config.New()
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	server, err := NewServer(c, logger.Noop(), nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.StartWithShutdown(ctx, "localhost:0")
	}()

	cancel()

	err = <-errCh
	if err != nil {
		t.Errorf("StartWithShutdown() error = %v", err)
	}
}
