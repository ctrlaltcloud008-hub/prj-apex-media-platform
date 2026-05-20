package middleware

import (
	"context"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/models"
)

type contextKey string

const (
	UserKey             contextKey = "user"
	TierKey             contextKey = "tier"
	RequestIDKey        contextKey = "request_id"
	ClientRegionHintKey contextKey = "client_region_hint"
)

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserKey, userID)
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserKey).(string)
	return userID, ok && userID != ""
}

func WithUserTier(ctx context.Context, tier models.UserTier) context.Context {
	return context.WithValue(ctx, TierKey, tier)
}

func UserTierFromContext(ctx context.Context) (models.UserTier, bool) {
	tier, ok := ctx.Value(TierKey).(models.UserTier)
	return tier, ok && tier != ""
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

func RequestIDFromContext(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(RequestIDKey).(string)
	return requestID, ok && requestID != ""
}

func WithClientRegionHint(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, ClientRegionHintKey, region)
}

func ClientRegionHintFromContext(ctx context.Context) (string, bool) {
	region, ok := ctx.Value(ClientRegionHintKey).(string)
	return region, ok && region != ""
}
