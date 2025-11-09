package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/logger"
	"github.com/joeychilson/websurfer/server/middleware"
)

// ServerConfig holds configuration for the API server.
type ServerConfig struct {
	// RedisURL for rate limiting (optional, uses in-memory if empty)
	RedisURL string
	// RateLimitRequests is the number of requests allowed per window (default: 100)
	RateLimitRequests int
	// RateLimitWindow is the time window for rate limiting (default: 1 minute)
	RateLimitWindow time.Duration
}

// Server is the HTTP server for the API.
type Server struct {
	handler     *Handler
	logger      logger.Logger
	router      *chi.Mux
	rateLimiter *middleware.RateLimiter
}

// NewServer creates a new API server with chi router and middleware stack.
func NewServer(c *client.Client, log logger.Logger, cfg *ServerConfig) (*Server, error) {
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

	handler := NewHandler(c, log)

	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger(log))
	r.Use(chimiddleware.Recoverer)

	rateLimitConfig := middleware.RateLimitConfig{
		RequestLimit:   cfg.RateLimitRequests,
		WindowDuration: cfg.RateLimitWindow,
		RedisURL:       cfg.RedisURL,
	}
	rateLimiter, err := middleware.RateLimit(rateLimitConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limiter: %w", err)
	}
	r.Use(rateLimiter.Handler)

	r.Post("/v1/fetch", handler.HandleFetch)
	r.Get("/health", handler.HandleHealth)

	s := &Server{
		handler:     handler,
		logger:      log,
		router:      r,
		rateLimiter: rateLimiter,
	}

	return s, nil
}

// Start starts the HTTP server.
func (s *Server) Start(addr string) error {
	s.logger.Info("starting API server", "addr", addr)
	return http.ListenAndServe(addr, s.router)
}

// StartWithShutdown starts the HTTP server with graceful shutdown support.
func (s *Server) StartWithShutdown(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("starting API server", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("shutting down API server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Close releases resources held by the server (e.g., Redis connections).
func (s *Server) Close() error {
	if s.rateLimiter != nil {
		return s.rateLimiter.Close()
	}
	return nil
}
