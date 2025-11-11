package server

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
	httprateredis "github.com/go-chi/httprate-redis"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig holds configuration for the rate limiter.
type RateLimitConfig struct {
	RequestLimit   int
	WindowDuration time.Duration
	RedisClient    *redis.Client // Optional Redis client for distributed rate limiting
}

// DefaultRateLimitConfig returns a default rate limit configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestLimit:   100,
		WindowDuration: time.Minute,
	}
}

// RateLimit returns a rate limiter middleware that rate limits requests per IP address.
func RateLimit(config RateLimitConfig) func(next http.Handler) http.Handler {
	if config.RequestLimit == 0 {
		config = DefaultRateLimitConfig()
	}

	limitHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limit exceeded","status_code":429}`))
	}

	baseOptions := []httprate.Option{
		httprate.WithLimitHandler(limitHandler),
		httprate.WithKeyByRealIP(),
	}

	var rateLimiter *httprate.RateLimiter
	if config.RedisClient != nil {
		redisConfig := &httprateredis.Config{
			Client:    config.RedisClient,
			PrefixKey: "websurfer:ratelimit",
		}
		options := append(baseOptions, httprateredis.WithRedisLimitCounter(redisConfig))
		rateLimiter = httprate.NewRateLimiter(config.RequestLimit, config.WindowDuration, options...)
	} else {
		rateLimiter = httprate.NewRateLimiter(config.RequestLimit, config.WindowDuration, baseOptions...)
	}

	return rateLimiter.Handler
}
