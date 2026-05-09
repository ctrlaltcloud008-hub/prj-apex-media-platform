package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func RequestLogging(logger *logging.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rw, r)

			requestID, _ := RequestIDFromContext(r.Context())
			userID, _ := UserIDFromContext(r.Context())
			userTier, _ := UserTierFromContext(r.Context())
			clientRegion, _ := ClientRegionFromContext(r.Context())
			logger.Info(
				r.Context(),
				"http.request",
				"Handled HTTP request",
				slog.String("request_id", requestID),
				slog.String("user_id", userID),
				slog.String("user_tier", string(userTier)),
				slog.String("client_region", clientRegion),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status_code", rw.statusCode),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}
