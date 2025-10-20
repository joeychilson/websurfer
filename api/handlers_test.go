package api

import (
	"context"
	"testing"

	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/content"
	"github.com/joeychilson/websurfer/logger"
)

func TestHandler(t *testing.T) {
	cfg := config.New()
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	handler := NewHandler(c, logger.Noop())

	t.Run("validates URL", func(t *testing.T) {
		req := &FetchRequest{
			URL: "",
		}

		_, err := handler.processFetch(context.Background(), req)
		if err == nil {
			t.Error("processFetch() should return error for empty URL")
		}
	})
}

func TestValidateRequest(t *testing.T) {
	cfg := config.New()
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}
	handler := NewHandler(c, logger.Noop())

	tests := []struct {
		name    string
		req     *FetchRequest
		wantErr bool
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
		},
		{
			name: "empty URL",
			req: &FetchRequest{
				URL: "",
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			req: &FetchRequest{
				URL: "not a url",
			},
			wantErr: true,
		},
		{
			name: "valid URL",
			req: &FetchRequest{
				URL: "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "negative max tokens",
			req: &FetchRequest{
				URL:       "https://example.com",
				MaxTokens: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid range type",
			req: &FetchRequest{
				URL: "https://example.com",
				Range: &content.RangeOptions{
					Type:  "invalid",
					Start: 0,
					End:   10,
				},
			},
			wantErr: true,
		},
		{
			name: "valid range",
			req: &FetchRequest{
				URL: "https://example.com",
				Range: &content.RangeOptions{
					Type:  "lines",
					Start: 0,
					End:   10,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
