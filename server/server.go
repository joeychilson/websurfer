package server

import (
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/logger"
	"github.com/joeychilson/websurfer/server/middleware"
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
	logger      logger.Logger
	rateLimiter *middleware.RateLimiter
}

// New creates a new API server instance.
func New(c *client.Client, log logger.Logger, cfg *ServerConfig) (*Server, error) {
	if log == nil {
		log = logger.Noop()
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

	rateLimitConfig := middleware.RateLimitConfig{
		RequestLimit:   cfg.RateLimitRequests,
		WindowDuration: cfg.RateLimitWindow,
		RedisURL:       cfg.RedisURL,
	}
	rateLimiter, err := middleware.RateLimit(rateLimitConfig)
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
	r.Use(middleware.Logger(s.logger))
	r.Use(chimiddleware.Recoverer)
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
