package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/server"
)

const (
	defaultAddr         = ":8080"
	defaultConfigFile   = "./config.yaml"
	defaultLogLevel     = "info"
	httpReadTimeout     = 30 * time.Second
	httpWriteTimeout    = 120 * time.Second
	httpIdleTimeout     = 60 * time.Second
	httpShutdownTimeout = 10 * time.Second
)

func main() {
	addr := getEnv("ADDR", defaultAddr)
	configFile := getEnv("CONFIG_FILE", defaultConfigFile)
	redisURL := getEnv("REDIS_URL", "")
	logLevel := getEnv("LOG_LEVEL", defaultLogLevel)

	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		slog.Warn("unknown log level, using info", "level", logLevel)
		level = slog.LevelInfo
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	log.Info("starting websurfer API server", "log_level", logLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if redisURL == "" {
		log.Error("REDIS_URL environment variable is required")
		os.Exit(1)
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Error("failed to parse redis URL", "error", err)
		os.Exit(1)
	}

	redisClient := redis.NewClient(opts)
	defer redisClient.Close()

	log.Info("connecting to redis", "url", redisURL)

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Error("failed to connect to redis", "error", err, "url", redisURL)
		os.Exit(1)
	}

	log.Info("redis connection established", "url", redisURL)

	var c *client.Client
	if _, statErr := os.Stat(configFile); statErr == nil {
		log.Info("loading config from file", "file", configFile)
		c, err = client.NewFromFile(configFile)
		if err != nil {
			log.Error("failed to load config from file", "error", err)
			os.Exit(1)
		}
	} else {
		log.Info("using default configuration (config file not found)", "checked", configFile)
		clientCfg := config.New()
		c, err = client.New(clientCfg)
		if err != nil {
			log.Error("failed to create client", "error", err)
			os.Exit(1)
		}
	}
	c = c.WithLogger(log)
	defer c.Close()

	c = c.WithCache(cache.New(redisClient, cache.Config{}))
	log.Info("redis cache enabled")

	srv, err := server.New(c, log, &server.ServerConfig{RedisClient: redisClient})
	if err != nil {
		log.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Router(),
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: httpWriteTimeout,
		IdleTimeout:  httpIdleTimeout,
	}

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
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Error("server shutdown error", "error", err)
			os.Exit(1)
		}
	case err := <-errCh:
		log.Error("server error", "error", err)
		os.Exit(1)
	}

	log.Info("server shutdown complete")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
