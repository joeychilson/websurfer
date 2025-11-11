package server

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/joeychilson/websurfer/client"
)

// ServerConfig holds configuration for the API server.
type ServerConfig struct {
	RedisURL          string
	RateLimitRequests int
	RateLimitWindow   time.Duration
}

// Server represents the API server.
type Server struct {
	client      *client.Client
	logger      *slog.Logger
	rateLimiter *RateLimiter
}

// New creates a new API server instance.
func New(c *client.Client, log *slog.Logger, cfg *ServerConfig) (*Server, error) {
	if log == nil {
		log = slog.Default()
	}

	if cfg == nil {
		cfg = &ServerConfig{}
	}

	if cfg.RateLimitRequests == 0 {
		cfg.RateLimitRequests = 100
	}
	if cfg.RateLimitWindow == 0 {
		cfg.RateLimitWindow = time.Minute
	}

	rateLimitConfig := RateLimitConfig{
		RequestLimit:   cfg.RateLimitRequests,
		WindowDuration: cfg.RateLimitWindow,
		RedisURL:       cfg.RedisURL,
	}
	rateLimiter, err := RateLimit(rateLimitConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limiter: %w", err)
	}

	return &Server{
		client:      c,
		logger:      log,
		rateLimiter: rateLimiter,
	}, nil
}

// Router returns a configured chi.Mux with all routes and middleware.
func (s *Server) Router() chi.Router {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(httplog.RequestLogger(s.logger, &httplog.Options{
		Level:         slog.LevelInfo,
		RecoverPanics: true,
	}))
	r.Use(s.rateLimiter.Handler)

	r.Post("/v1/fetch", s.handleFetch)
	r.Get("/health", s.handleHealth)

	return r
}

// Close releases resources held by the server (e.g., Redis connections).
func (s *Server) Close() error {
	if s.rateLimiter != nil {
		return s.rateLimiter.Close()
	}
	return nil
}
