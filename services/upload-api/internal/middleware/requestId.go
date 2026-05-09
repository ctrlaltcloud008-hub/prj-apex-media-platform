package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/models"
	"github.com/google/uuid"
)

func RequestID(logger *logging.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate a unique request ID (e.g., using UUID)
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.NewString()
			} else {
				// Validate the provided request ID (optional)
				if _, err := uuid.Parse(requestID); err != nil {
					logger.Warn(
						r.Context(),
						"request_id.invalid",
						"Rejected request with invalid X-Request-ID header",
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("request_id", requestID),
					)
					errPayload := models.NewErrorResponse(models.StatusInvalidArgument, http.StatusBadRequest, "X-Request-ID must be a valid UUID").WithReason(models.ReasonInvalidRequestID).WithMetadata(map[string]string{"received_value": requestID})
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(errPayload)
					return
				}
				// Use the provided request ID if it's valid
				requestID = requestID
			}

			// Set the request ID in the response header for client reference
			w.Header().Set("X-Request-ID", requestID)

			ctx := WithRequestID(r.Context(), requestID)
			r = r.WithContext(ctx)

			// Call the next handler in the chain
			next.ServeHTTP(w, r)
		})
	}
}
