package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/httprate"
	httprateredis "github.com/go-chi/httprate-redis"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig holds configuration for the rate limiter.
type RateLimitConfig struct {
	RequestLimit   int
	WindowDuration time.Duration
	RedisClient    *redis.Client
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

// AuthMiddleware returns a middleware that validates API key from Authorization header or X-API-Key header.
// The API key is loaded from the API_KEY environment variable.
// If API_KEY is not set, the middleware is disabled and all requests are allowed.
func AuthMiddleware() func(next http.Handler) http.Handler {
	apiKey := os.Getenv("API_KEY")

	if apiKey == "" {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")

			if key == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					key = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if key == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"missing API key","status_code":401,"message":"Provide API key via X-API-Key header or Authorization: Bearer <key>"}`))
				return
			}

			if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid API key","status_code":401}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
