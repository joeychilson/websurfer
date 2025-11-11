package main

import (
	"context"
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
	defaultRedisURL   = "redis://localhost:6379/0"
	defaultLogLevel   = "info"
)

type appConfig struct {
	addr       string
	configFile string
	redisURL   string
	logLevel   string
}

func main() {
	cfg := loadConfig()

	log := setupLogger(cfg.logLevel)
	log.Info("starting websurfer API server",
		"addr", cfg.addr,
		"log_level", cfg.logLevel,
		"redis_url", cfg.redisURL)

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
	defer c.Close()

	if cfg.redisURL == "" {
		log.Error("redis URL is required")
		os.Exit(1)
	}

	opts, err := redis.ParseURL(cfg.redisURL)
	if err != nil {
		log.Error("failed to parse redis URL", "error", err)
		os.Exit(1)
	}

	redisClient := redis.NewClient(opts)
	defer redisClient.Close()

	log.Info("redis client created", "url", cfg.redisURL)

	cacheImpl := cache.New(redisClient, cache.Config{})
	c = c.WithCache(cacheImpl)

	log.Info("redis cache enabled")

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

func loadConfig() *appConfig {
	return &appConfig{
		addr:       getEnv("ADDR", defaultAddr),
		configFile: getEnv("CONFIG_FILE", defaultConfigFile),
		redisURL:   getEnv("REDIS_URL", defaultRedisURL),
		logLevel:   getEnv("LOG_LEVEL", defaultLogLevel),
	}
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
