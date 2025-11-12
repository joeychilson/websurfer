package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
)

func TestFetcher_MaxBodySize(t *testing.T) {
	tests := []struct {
		name        string
		bodySize    int
		maxBodySize int64
		wantErr     bool
		errContains string
	}{
		{
			name:        "small body within limit",
			bodySize:    100,
			maxBodySize: 1024,
			wantErr:     false,
		},
		{
			name:        "body exactly at limit",
			bodySize:    1024,
			maxBodySize: 1024,
			wantErr:     true,
			errContains: "exceeds maximum size",
		},
		{
			name:        "body exceeds limit",
			bodySize:    2048,
			maxBodySize: 1024,
			wantErr:     true,
			errContains: "exceeds maximum size",
		},
		{
			name:        "unlimited (maxBodySize = 0)",
			bodySize:    10000,
			maxBodySize: 0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(strings.Repeat("a", tt.bodySize)))
			}))
			defer server.Close()

			cfg := config.FetchConfig{
				Timeout:     10 * time.Second,
				MaxBodySize: tt.maxBodySize,
			}

			fetcher, err := New(cfg)
			if err != nil {
				t.Fatalf("failed to create fetcher: %v", err)
			}

			ctx := context.Background()
			resp, err := fetcher.FetchWithOptions(ctx, server.URL, nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response, got nil")
				} else if len(resp.Body) != tt.bodySize {
					t.Errorf("expected body size %d, got %d", tt.bodySize, len(resp.Body))
				}
			}
		})
	}
}

func TestFetcher_MaxBodySize_Default(t *testing.T) {
	cfg := config.FetchConfig{
		Timeout: 10 * time.Second,
	}

	if cfg.GetMaxBodySize() != 100*1024*1024 {
		t.Errorf("expected default max body size to be 100MB, got %d", cfg.GetMaxBodySize())
	}
}

func TestFetcher_MaxBodySize_Large(t *testing.T) {
	largeBodySize := 50 * 1024 * 1024

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		chunk := strings.Repeat("a", 1024*1024)
		for range 50 {
			w.Write([]byte(chunk))
		}
	}))
	defer server.Close()

	cfg := config.FetchConfig{
		Timeout: 30 * time.Second,
	}

	fetcher, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create fetcher: %v", err)
	}

	ctx := context.Background()
	resp, err := fetcher.FetchWithOptions(ctx, server.URL, nil)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp != nil && len(resp.Body) != largeBodySize {
		t.Errorf("expected body size %d, got %d", largeBodySize, len(resp.Body))
	}
}

func TestFetcher_MaxBodySize_TooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.Repeat("a", 2*1024*1024)))
	}))
	defer server.Close()

	cfg := config.FetchConfig{
		Timeout:     10 * time.Second,
		MaxBodySize: 1 * 1024 * 1024,
	}

	fetcher, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create fetcher: %v", err)
	}

	ctx := context.Background()
	_, err = fetcher.FetchWithOptions(ctx, server.URL, nil)

	if err == nil {
		t.Error("expected error for body exceeding limit, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("expected error about exceeding size, got: %v", err)
	}
}
