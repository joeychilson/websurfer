package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/config"
	api "github.com/joeychilson/websurfer/server"
	"github.com/redis/go-redis/v9"
)

const (
	defaultAddr       = ":8080"
	defaultConfigFile = "./config.yaml"
	defaultRedisURL   = ""
	defaultLogLevel   = "info"
)

type appConfig struct {
	addr       string
	configFile string
	redisURL   string
	logLevel   string
}

func main() {
	cfg := parseFlags()

	log := setupLogger(cfg.logLevel)
	log.Info("starting websurfer API server",
		"addr", cfg.addr,
		"log_level", cfg.logLevel)

	var c *client.Client
	var err error

	if _, statErr := os.Stat(cfg.configFile); statErr == nil {
		log.Info("loading config from file", "file", cfg.configFile)
		c, err = client.NewFromFile(cfg.configFile)
		if err != nil {
			log.Error("failed to load config from file", "error", err)
			os.Exit(1)
		}
	} else {
		log.Info("using default configuration (config file not found)", "checked", cfg.configFile)
		clientCfg := config.New()
		c, err = client.New(clientCfg)
		if err != nil {
			log.Error("failed to create client", "error", err)
			os.Exit(1)
		}
	}

	c = c.WithLogger(log)

	// Create shared Redis client if configured
	var redisClient *redis.Client
	if cfg.redisURL != "" {
		opts, err := redis.ParseURL(cfg.redisURL)
		if err != nil {
			log.Error("failed to parse redis URL", "error", err)
			os.Exit(1)
		}
		redisClient = redis.NewClient(opts)
		defer redisClient.Close()
		log.Info("redis client created", "url", cfg.redisURL)

		// Use Redis cache
		cacheImpl := cache.NewRedisCache(redisClient, cache.RedisConfig{})
		c = c.WithCache(cacheImpl)
		log.Info("redis cache enabled")
	} else {
		log.Info("cache disabled (no redis URL configured)")
	}

	serverConfig := &api.ServerConfig{
		RedisClient: redisClient,
	}

	srv := api.New(c, log, serverConfig)

	router := srv.Router()

	httpServer := &http.Server{
		Addr:         cfg.addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting API server", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down API server")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Error("server shutdown error", "error", err)
		}
	case err := <-errCh:
		log.Error("server error", "error", err)
		os.Exit(1)
	}

	log.Info("server shutdown complete")
}

func parseFlags() *appConfig {
	cfg := &appConfig{}

	flag.StringVar(&cfg.addr, "addr", getEnv("ADDR", defaultAddr),
		"HTTP server address")
	flag.StringVar(&cfg.configFile, "config", getEnv("CONFIG_FILE", defaultConfigFile),
		"Path to config file (optional)")
	flag.StringVar(&cfg.redisURL, "redis-url", getEnv("REDIS_URL", defaultRedisURL),
		"Redis URL (enables cache and distributed rate limiting)")
	flag.StringVar(&cfg.logLevel, "log-level", getEnv("LOG_LEVEL", defaultLogLevel),
		"Log level: debug, info, warn, error")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "WebSurfer API Server - LLM-optimized web scraping API\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  ADDR          HTTP server address (default: %s)\n", defaultAddr)
		fmt.Fprintf(os.Stderr, "  CONFIG_FILE   Path to config file (default: %s)\n", defaultConfigFile)
		fmt.Fprintf(os.Stderr, "  REDIS_URL     Redis URL for cache and rate limiting (optional)\n")
		fmt.Fprintf(os.Stderr, "  LOG_LEVEL     Log level: debug, info, warn, error (default: %s)\n", defaultLogLevel)
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -addr :3000 -redis-url redis://localhost:6379/0\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -config config.yaml -log-level debug\n", os.Args[0])
	}

	flag.Parse()
	return cfg
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		slog.Warn("unknown log level, using info", "level", level)
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	return slog.New(handler)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
