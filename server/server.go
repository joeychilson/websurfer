package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/joeychilson/websurfer/client"
	"github.com/redis/go-redis/v9"
)

// ServerConfig holds configuration for the API server.
type ServerConfig struct {
	RedisClient       *redis.Client
	RateLimitRequests int
	RateLimitWindow   time.Duration
}

// Server represents the API server.
type Server struct {
	client      *client.Client
	logger      *slog.Logger
	rateLimiter func(next http.Handler) http.Handler
}

// New creates a new API server instance.
func New(c *client.Client, log *slog.Logger, cfg *ServerConfig) *Server {
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
		RedisClient:    cfg.RedisClient,
	}
	rateLimiter := RateLimit(rateLimitConfig)

	return &Server{
		client:      c,
		logger:      log,
		rateLimiter: rateLimiter,
	}
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
	r.Use(s.rateLimiter)

	r.Post("/v1/fetch", s.handleFetch)
	r.Get("/health", s.handleHealth)

	return r
}
