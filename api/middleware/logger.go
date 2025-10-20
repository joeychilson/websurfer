package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/joeychilson/websurfer/logger"
)

// Logger returns a middleware that logs HTTP requests using the provided logger.
// It logs request method, path, status code, duration, and includes request ID if available.
func Logger(log logger.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			reqID := middleware.GetReqID(r.Context())

			reqLog := log.With(
				"request_id", reqID,
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
			)

			reqLog.Info("request started")

			next.ServeHTTP(ww, r)

			duration := time.Since(start)
			reqLog.Info("request completed",
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", duration.Milliseconds(),
			)
		})
	}
}
