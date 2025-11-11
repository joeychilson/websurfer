package middleware

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
	RedisURL       string
}

// DefaultRateLimitConfig returns a default rate limit configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestLimit:   100,
		WindowDuration: time.Minute,
	}
}

// RateLimiter wraps the rate limiting middleware with a cleanup function.
type RateLimiter struct {
	Handler     func(next http.Handler) http.Handler
	redisClient *redis.Client
}

// RateLimit returns a rate limiter middleware that rate limits requests per IP address.
func RateLimit(config RateLimitConfig) (*RateLimiter, error) {
	if config.RequestLimit == 0 {
		config = DefaultRateLimitConfig()
	}

	var rateLimiter *httprate.RateLimiter
	var redisClient *redis.Client

	if config.RedisURL != "" {
		opts, err := redis.ParseURL(config.RedisURL)
		if err != nil {
			return nil, err
		}

		redisClient = redis.NewClient(opts)

		redisConfig := &httprateredis.Config{
			Client:    redisClient,
			PrefixKey: "websurfer:ratelimit",
		}

		rateLimiter = httprate.NewRateLimiter(
			config.RequestLimit,
			config.WindowDuration,
			httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded","status_code":429}`))
			}),
			httprate.WithKeyByRealIP(),
			httprateredis.WithRedisLimitCounter(redisConfig),
		)
	} else {
		rateLimiter = httprate.NewRateLimiter(
			config.RequestLimit,
			config.WindowDuration,
			httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded","status_code":429}`))
			}),
			httprate.WithKeyByRealIP(),
		)
	}

	return &RateLimiter{
		Handler:     rateLimiter.Handler,
		redisClient: redisClient,
	}, nil
}

// Close releases resources held by the rate limiter (e.g., Redis connection).
func (rl *RateLimiter) Close() error {
	if rl.redisClient != nil {
		return rl.redisClient.Close()
	}
	return nil
}
