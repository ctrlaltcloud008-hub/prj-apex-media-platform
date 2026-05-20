package middleware

import (
	"log/slog"
	"net/http"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	tier "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/models"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/models"
)

func Authentication(logger *logging.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID, _ := RequestIDFromContext(r.Context())
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.Warn(
					r.Context(),
					"auth.missing_header",
					"Rejected request without Authorization header",
					slog.String("request_id", requestID),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				models.WriteError(w, models.NewMissingAuthorizationHeaderError())
				return
			}

			// Here you would typically validate the token or credentials in the authHeader.
			// For simplicity, we'll just check if it starts with "Bearer " and has a token.
			if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
				logger.Warn(
					r.Context(),
					"auth.invalid_header_format",
					"Rejected request with invalid Authorization header format",
					slog.String("request_id", requestID),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				models.WriteError(w, models.NewInvalidAuthorizationHeaderError())
				return
			}

			token := authHeader[7:]
			if token == "" {
				logger.Warn(
					r.Context(),
					"auth.missing_token",
					"Rejected request with empty bearer token",
					slog.String("request_id", requestID),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				models.WriteError(w, models.NewMissingBearerTokenError())
				return
			}

			clientRegionHint := r.Header.Get("X-Client-Region")

			// If the token is valid, you can set user information in the request context here.
			ctx := WithUserID(r.Context(), "exampleUser")
			ctx = WithUserTier(ctx, tier.UserTierFree)
			ctx = WithClientRegionHint(ctx, clientRegionHint)
			logger.Info(
				ctx,
				"auth.authenticated",
				"Authenticated upload request",
				slog.String("request_id", requestID),
				slog.String("user_id", "exampleUser"),
				slog.String("user_tier", string(tier.UserTierFree)),
				slog.String("client_region_hint", clientRegionHint),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)

			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}
