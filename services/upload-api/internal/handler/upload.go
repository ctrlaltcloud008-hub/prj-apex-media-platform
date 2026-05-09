package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/middleware"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/models"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/services"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/validation"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func uploadErrorResponse(err error) *models.ErrorResponse {
	switch {
	case errors.Is(err, services.ErrInvalidUserTier):
		return models.NewErrorResponse(models.StatusInvalidArgument, http.StatusBadRequest, "invalid user tier").WithInternal(err)
	case errors.Is(err, services.ErrFileTooLarge):
		return models.NewErrorResponse(models.StatusInvalidArgument, http.StatusRequestEntityTooLarge, "file too large").WithReason(models.ReasonFileTooLarge).WithInternal(err)
	case errors.Is(err, services.ErrConcurrentUploadLimit):
		return models.NewErrorResponse(models.StatusResourceExhausted, http.StatusTooManyRequests, "concurrent upload limit reached").WithReason(models.ReasonUploadLimitExceeded).WithInternal(err)
	case errors.Is(err, services.ErrHourlyUploadLimit):
		return models.NewErrorResponse(models.StatusResourceExhausted, http.StatusTooManyRequests, "hourly upload rate limit reached").WithReason(models.ReasonHourlyRateExceeded).WithInternal(err)
	case errors.Is(err, services.ErrStorageQuotaExceeded):
		return models.NewErrorResponse(models.StatusPermissionDenied, http.StatusForbidden, "storage quota exceeded").WithReason(models.ReasonStorageQuotaExceeded).WithInternal(err)
	case errors.Is(err, services.ErrUploadServiceUnavailable):
		return models.NewErrorResponse(models.StatusUnavailable, http.StatusServiceUnavailable, "upload service unavailable").WithInternal(err)
	case errors.Is(err, services.ErrSignedURLGeneration):
		return models.NewErrorResponse(models.StatusInternal, http.StatusInternalServerError, "failed to generate signed upload URL").WithInternal(err)
	default:
		return models.NewErrorResponse(models.StatusInternal, http.StatusInternalServerError, "failed to create upload").WithInternal(err)
	}
}

func paramsFromRequest(r *http.Request) (services.Params, error) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		return services.Params{}, fmt.Errorf("missing user id")
	}

	userTier, ok := middleware.UserTierFromContext(r.Context())
	if !ok {
		return services.Params{}, fmt.Errorf("missing user tier")
	}

	requestID, ok := middleware.RequestIDFromContext(r.Context())
	if !ok {
		return services.Params{}, fmt.Errorf("missing request id")
	}

	region, ok := middleware.ClientRegionFromContext(r.Context())
	if !ok {
		return services.Params{}, fmt.Errorf("missing client region")
	}

	return services.Params{
		UserID:    userID,
		UserTier:  userTier,
		RequestID: requestID,
		Region:    region,
	}, nil
}

func uploadLogAttrs(r *http.Request, req *models.UploadRequest) []slog.Attr {
	requestID, _ := middleware.RequestIDFromContext(r.Context())
	userID, _ := middleware.UserIDFromContext(r.Context())
	userTier, _ := middleware.UserTierFromContext(r.Context())
	clientRegion, _ := middleware.ClientRegionFromContext(r.Context())

	attrs := []slog.Attr{
		slog.String("request_id", requestID),
		slog.String("user_id", userID),
		slog.String("user_tier", string(userTier)),
		slog.String("client_region", clientRegion),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
	}

	if req != nil {
		attrs = append(attrs,
			slog.String("filename", req.Filename),
			slog.String("content_type", req.ContentType),
			slog.Int64("file_size_bytes", req.FileSizeBytes),
		)
	}

	return attrs
}

func Upload(logger *logging.Logger, uploadService services.UploadService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := startSpan(r.Context(),
			"upload-api.create-upload",
			attribute.String("http.method", r.Method),
			attribute.String("http.route", r.URL.Path),
		)
		defer span.End()
		r = r.WithContext(ctx)

		var req models.UploadRequest

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&req); err != nil {
			span.AddEvent("upload.request.decode_failed")
			recordSpanError(span, err, "invalid json payload")
			logger.Warn(r.Context(), "upload.request_invalid_json", "Rejected upload request with invalid JSON", uploadLogAttrs(r, nil)...)
			errPayload := models.NewErrorResponse(models.StatusInvalidArgument,
				http.StatusBadRequest, "invalid JSON payload").WithMetadata(map[string]string{"error": err.Error()})
			writeJSON(w, http.StatusBadRequest, errPayload)
			return
		}

		span.SetAttributes(
			attribute.String("video.filename", req.Filename),
			attribute.String("video.content_type", req.ContentType),
			attribute.Int64("video.file_size_bytes", req.FileSizeBytes),
		)

		if !validation.IsAllowedContentType(req.ContentType) {
			span.AddEvent("upload.request.invalid_content_type", trace.WithAttributes(attribute.String("video.content_type", req.ContentType)))
			span.SetStatus(otelcodes.Error, "unsupported content type")
			logger.Warn(r.Context(), "upload.request_invalid_content_type", "Rejected upload request with unsupported content type", uploadLogAttrs(r, &req)...)
			errPayload := models.NewErrorResponse(models.StatusInvalidArgument,
				http.StatusBadRequest, "unsupported content type").WithReason(models.ReasonInvalidContentType).WithMetadata(map[string]string{"content_type": req.ContentType})
			writeJSON(w, http.StatusBadRequest, errPayload)
			return
		}

		if !validation.IsValidFilename(req.Filename) {
			span.AddEvent("upload.request.invalid_filename", trace.WithAttributes(attribute.String("video.filename", req.Filename)))
			span.SetStatus(otelcodes.Error, "invalid filename")
			logger.Warn(r.Context(), "upload.request_invalid_filename", "Rejected upload request with invalid filename", uploadLogAttrs(r, &req)...)
			errPayload := models.NewErrorResponse(models.StatusInvalidArgument,
				http.StatusBadRequest, "invalid filename").WithReason(models.ReasonInvalidFilename).WithMetadata(map[string]string{"filename": req.Filename})
			writeJSON(w, http.StatusBadRequest, errPayload)
			return
		}

		params, err := paramsFromRequest(r)
		if err != nil {
			span.AddEvent("upload.request.missing_context")
			recordSpanError(span, err, "missing request context")
			logger.Warn(r.Context(), "upload.request_missing_context", "Rejected upload request with missing request context", uploadLogAttrs(r, &req)...)
			errPayload := models.NewErrorResponse(models.StatusInvalidArgument,
				http.StatusBadRequest, "missing required parameters").WithMetadata(map[string]string{"error": err.Error()})
			writeJSON(w, http.StatusBadRequest, errPayload)
			return
		}

		span.SetAttributes(
			attribute.String("video.request_id", params.RequestID),
			attribute.String("video.user_id", params.UserID),
			attribute.String("video.user_tier", string(params.UserTier)),
			attribute.String("video.client_region", params.Region),
		)
		span.AddEvent("upload.request.validated")

		logger.Info(r.Context(), "upload.request_received", "Received upload request", uploadLogAttrs(r, &req)...)

		response, err := uploadService.CreateUpload(r.Context(), params, req)
		if err != nil {
			errPayload := uploadErrorResponse(err)
			span.AddEvent("upload.request.failed", trace.WithAttributes(
				attribute.Int("http.status_code", errPayload.HttpStatus),
				attribute.String("error.reason", string(errPayload.Reason)),
			))
			recordSpanError(span, err, "failed to create upload")
			logger.Warn(
				r.Context(),
				"upload.request_failed",
				"Upload request failed",
				append(uploadLogAttrs(r, &req),
					slog.Int("status_code", errPayload.HttpStatus),
					slog.String("error", err.Error()),
					slog.String("reason", string(errPayload.Reason)),
				)...,
			)
			writeJSON(w, errPayload.HttpStatus, errPayload)
			return
		}

		span.SetAttributes(
			attribute.String("video.id", response.VideoID),
			attribute.Int64("video.max_file_size_bytes", response.MaxFileSizeBytes),
		)
		span.AddEvent("upload.request.succeeded")
		span.SetStatus(otelcodes.Ok, "upload url created")

		logger.Info(
			r.Context(),
			"upload.request_succeeded",
			"Upload request succeeded",
			append(uploadLogAttrs(r, &req),
				slog.String("video_id", response.VideoID),
				slog.Int64("max_file_size_bytes", response.MaxFileSizeBytes),
			)...,
		)

		payload := &models.UploadResponse{
			VideoID:             response.VideoID,
			UploadURL:           response.UploadURL,
			UploadURLExpiresAt:  response.UploadURLExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			MaxFileSizeBytes:    response.MaxFileSizeBytes,
			AllowedContentTypes: validation.AllowedContentTypesList(),
		}

		writeJSON(w, http.StatusCreated, payload)
	}
}
