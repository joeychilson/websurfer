package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/logger"
	api "github.com/joeychilson/websurfer/server"
)

const (
	defaultAddr       = ":8080"
	defaultConfigFile = "./config.yaml"
	defaultCacheType  = "memory"
	defaultRedisURL   = "redis://localhost:6379/0"
	defaultLogLevel   = "info"
)

type appConfig struct {
	addr       string
	configFile string
	cacheType  string
	redisURL   string
	logLevel   string
}

func main() {
	cfg := parseFlags()

	log := setupLogger(cfg.logLevel)
	log.Info("starting websurfer API server",
		"addr", cfg.addr,
		"cache_type", cfg.cacheType,
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

	if cfg.cacheType != "none" {
		var cacheImpl cache.Cache
		switch cfg.cacheType {
		case "memory":
			log.Info("using in-memory cache")
			cacheImpl = cache.NewMemoryCache(cache.DefaultConfig())
		case "redis":
			log.Info("using Redis cache", "url", cfg.redisURL)
			var err error
			cacheImpl, err = cache.NewRedisCacheFromURL(cfg.redisURL, "websurfer:", cache.DefaultConfig())
			if err != nil {
				log.Error("failed to create Redis cache", "error", err)
				os.Exit(1)
			}
		default:
			log.Error("unknown cache type", "type", cfg.cacheType)
			os.Exit(1)
		}
		c = c.WithCache(cacheImpl)
	} else {
		log.Info("cache disabled")
	}

	serverConfig := &api.ServerConfig{
		RedisURL: cfg.redisURL,
	}

	server, err := api.NewServer(c, log, serverConfig)
	if err != nil {
		log.Error("failed to create server", "error", err)
		os.Exit(1)
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

	if err := server.StartWithShutdown(ctx, cfg.addr); err != nil {
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
	flag.StringVar(&cfg.cacheType, "cache", getEnv("CACHE_TYPE", defaultCacheType),
		"Cache type: none, memory, or redis")
	flag.StringVar(&cfg.redisURL, "redis-url", getEnv("REDIS_URL", defaultRedisURL),
		"Redis URL (for redis cache)")
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
		fmt.Fprintf(os.Stderr, "  CACHE_TYPE    Cache type: none, memory, redis (default: %s)\n", defaultCacheType)
		fmt.Fprintf(os.Stderr, "  REDIS_URL     Redis URL (default: %s)\n", defaultRedisURL)
		fmt.Fprintf(os.Stderr, "  LOG_LEVEL     Log level: debug, info, warn, error (default: %s)\n", defaultLogLevel)
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -addr :3000 -cache redis -redis-url redis://localhost:6379/0\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -config config.json -log-level debug\n", os.Args[0])
	}

	flag.Parse()
	return cfg
}

func setupLogger(level string) logger.Logger {
	var logLevel logger.Level
	switch level {
	case "debug":
		logLevel = logger.LevelDebug
	case "info":
		logLevel = logger.LevelInfo
	case "warn":
		logLevel = logger.LevelWarn
	case "error":
		logLevel = logger.LevelError
	default:
		slog.Warn("unknown log level, using info", "level", level)
		logLevel = logger.LevelInfo
	}

	return logger.NewWithLevel(logLevel)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
