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
	defaultRedisURL     = "redis://localhost:6379/0"
	defaultLogLevel     = "info"
	httpReadTimeout     = 30 * time.Second
	httpWriteTimeout    = 120 * time.Second
	httpIdleTimeout     = 60 * time.Second
	httpShutdownTimeout = 10 * time.Second
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

	c := setupClient(cfg, log)
	defer c.Close()

	redisClient := setupRedis(cfg, log)
	defer redisClient.Close()

	c = c.WithCache(cache.New(redisClient, cache.Config{}))
	log.Info("redis cache enabled")

	srv := setupServer(c, log, redisClient)
	httpServer := createHTTPServer(cfg.addr, srv.Router())

	if err := runServer(httpServer, log); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}

	log.Info("server shutdown complete")
}

// setupClient creates and configures the websurfer client.
func setupClient(cfg *appConfig, log *slog.Logger) *client.Client {
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

	return c.WithLogger(log)
}

// setupRedis creates and configures the Redis client.
func setupRedis(cfg *appConfig, log *slog.Logger) *redis.Client {
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
	log.Info("redis client created", "url", cfg.redisURL)
	return redisClient
}

// setupServer creates the API server with the given configuration.
func setupServer(c *client.Client, log *slog.Logger, redisClient *redis.Client) *server.Server {
	serverConfig := &server.ServerConfig{
		RedisClient: redisClient,
	}

	srv, err := server.New(c, log, serverConfig)
	if err != nil {
		log.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	return srv
}

// createHTTPServer creates an HTTP server with standard timeouts.
func createHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: httpWriteTimeout,
		IdleTimeout:  httpIdleTimeout,
	}
}

// runServer starts the HTTP server and handles graceful shutdown.
func runServer(httpServer *http.Server, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupSignalHandler(cancel, log)

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting API server", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return gracefulShutdown(httpServer, log)
	case err := <-errCh:
		return err
	}
}

// setupSignalHandler configures OS signal handling for graceful shutdown.
func setupSignalHandler(cancel context.CancelFunc, log *slog.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info("received shutdown signal", "signal", sig.String())
		cancel()
	}()
}

// gracefulShutdown performs a graceful shutdown of the HTTP server.
func gracefulShutdown(httpServer *http.Server, log *slog.Logger) error {
	log.Info("shutting down API server")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown error", "error", err)
		return err
	}

	return nil
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
