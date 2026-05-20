package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/spanner"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/apperror"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/gcs"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	tier "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/models"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/quota"
	spannerutil "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/config"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/domain"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrFileTooLarge             = errors.New("file too large")
	ErrConcurrentUploadLimit    = errors.New("concurrent upload limit reached")
	ErrHourlyUploadLimit        = errors.New("hourly upload limit reached")
	ErrStorageQuotaExceeded     = errors.New("storage quota exceeded")
	ErrSignedURLGeneration      = errors.New("signed URL generation failed")
	ErrUploadServiceUnavailable = errors.New("upload service unavailable")
)

const instrumentationName = "services/upload-api/internal/services"

type Params struct {
	UserID           string
	UserTier         tier.UserTier
	RequestID        string
	ClientRegionHint string
}

type UploadService interface {
	CreateUpload(ctx context.Context, params Params, req domain.CreateUploadRequest) (*domain.CreateUploadResult, error)
}

type uploadService struct {
	logger  *logging.Logger
	cfg     *config.UploadConfig
	gcs     *gcs.Client
	spanner *spanner.Client
}

func NewUploadService(logger *logging.Logger,
	cfg *config.UploadConfig,
	spanner *spanner.Client,
	gcs *gcs.Client) UploadService {
	return &uploadService{
		logger:  logger,
		cfg:     cfg,
		spanner: spanner,
		gcs:     gcs,
	}
}

func (s *uploadService) CreateUpload(ctx context.Context, params Params, req domain.CreateUploadRequest) (*domain.CreateUploadResult, error) {
	ctx, span := otel.Tracer(instrumentationName).Start(ctx,
		"upload-api.create-upload.service",
		trace.WithAttributes(
			attribute.String("video.request_id", params.RequestID),
			attribute.String("video.user_id", params.UserID),
			attribute.String("video.user_tier", string(params.UserTier)),
			attribute.String("video.client_region_hint", params.ClientRegionHint),
			attribute.String("video.filename", req.Filename),
			attribute.String("video.content_type", req.ContentType),
			attribute.Int64("video.file_size_bytes", req.FileSizeBytes),
		),
	)
	defer span.End()

	logger := s.logger.WithSpanContext(ctx)
	logger.Info(
		ctx,
		"upload.create_requested",
		"Processing upload creation request",
		slog.String("request_id", params.RequestID),
		slog.String("user_id", params.UserID),
		slog.String("user_tier", string(params.UserTier)),
		slog.String("client_region_hint", params.ClientRegionHint),
		slog.String("filename", req.Filename),
		slog.String("content_type", req.ContentType),
		slog.Int64("file_size_bytes", req.FileSizeBytes),
	)

	limits, err := tier.GetTierLimits(params.UserTier)
	if err != nil {
		span.AddEvent("upload.limits.invalid_user_tier")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "invalid user tier")
		logger.Warn(
			ctx,
			"upload.invalid_user_tier",
			"Rejected upload request for invalid user tier",
			slog.String("request_id", params.RequestID),
			slog.String("user_id", params.UserID),
			slog.String("user_tier", string(params.UserTier)),
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	err = s.validateFileSizeForTier(limits, req.FileSizeBytes)
	if err != nil {
		span.AddEvent("upload.limits.file_size_rejected", trace.WithAttributes(
			attribute.Int64("video.max_file_size_bytes", limits.MaxFileSizeBytes),
		))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "file size exceeds limit")
		logger.Warn(
			ctx,
			"upload.file_size_rejected",
			"Rejected upload request that exceeds tier file size limit",
			slog.String("request_id", params.RequestID),
			slog.String("user_id", params.UserID),
			slog.Int64("file_size_bytes", req.FileSizeBytes),
			slog.Int64("max_file_size_bytes", limits.MaxFileSizeBytes),
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	bucket, resolvedRegion := s.resolveRegionAndBucket(params.ClientRegionHint)
	videoID := s.generateVideoID()
	objectPath := s.buildObjectPath(params.UserID, videoID, req.Filename)
	span.SetAttributes(
		attribute.String("video.id", videoID),
		attribute.String("storage.bucket", bucket),
		attribute.String("storage.object", objectPath),
		attribute.String("video.resolved_region", resolvedRegion),
	)
	span.AddEvent("upload.storage.target_resolved")

	videoRecord := &video.Video{
		VideoID:               videoID,
		UserID:                params.UserID,
		RequestID:             spanner.NullString{StringVal: params.RequestID, Valid: true},
		Status:                video.StatusUploading,
		SourceBucket:          bucket,
		SourceObject:          objectPath,
		GCSGeneration:         0,
		MimeType:              spanner.NullString{StringVal: req.ContentType, Valid: req.ContentType != ""},
		FileSizeBytes:         spanner.NullInt64{Int64: req.FileSizeBytes, Valid: req.FileSizeBytes > 0},
		LastLifecycleEventSeq: spanner.NullInt64{Int64: 0, Valid: true},
	}

	storedUpload, err := s.executeSpannerTransaction(ctx, params.UserID, params.RequestID, req, limits, videoRecord)
	if err != nil {
		if errors.Is(err, ErrIdempotencyMismatch) || errors.Is(err, ErrRequestIDAlreadyConsumed) {
			span.AddEvent("upload.idempotency.rejected")
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "request id conflict")
			logger.Warn(
				ctx,
				"upload.request_id_conflict",
				"Rejected upload request due to request ID conflict",
				slog.String("request_id", params.RequestID),
				slog.String("user_id", params.UserID),
				slog.String("video_id", videoID),
				slog.String("error", err.Error()),
			)
			return nil, err
		}

		span.AddEvent("upload.transaction.failed")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to persist upload request")
		logger.Warn(
			ctx,
			"upload.transaction_failed",
			"Failed to persist upload request state",
			slog.String("request_id", params.RequestID),
			slog.String("user_id", params.UserID),
			slog.String("video_id", videoID),
			slog.String("bucket", bucket),
			slog.String("object_path", objectPath),
			slog.String("error", err.Error()),
		)
		return nil, err
	}
	if storedUpload == nil {
		missingResultErr := fmt.Errorf("%w: missing stored video result", ErrUploadServiceUnavailable)
		span.AddEvent("upload.transaction.missing_result")
		span.RecordError(missingResultErr)
		span.SetStatus(otelcodes.Error, "missing stored video result")
		logger.Error(
			ctx,
			"upload.transaction_missing_result",
			"Upload transaction completed without stored video result",
			slog.String("request_id", params.RequestID),
			slog.String("user_id", params.UserID),
			slog.String("video_id", videoID),
		)
		return nil, missingResultErr
	}

	idempotentReplay := storedUpload.VideoID != videoRecord.VideoID
	videoID = storedUpload.VideoID
	bucket = storedUpload.SourceBucket
	objectPath = s.normalizeObjectPath(bucket, storedUpload.SourceObject)
	span.SetAttributes(
		attribute.String("video.id", videoID),
		attribute.String("storage.bucket", bucket),
		attribute.String("storage.object", objectPath),
		attribute.Bool("upload.idempotent_replay", idempotentReplay),
	)
	span.AddEvent("upload.record.ready_for_signing")
	logger.Info(
		ctx,
		"upload.record_ready",
		"Upload record ready for signed URL generation",
		slog.String("request_id", params.RequestID),
		slog.String("user_id", params.UserID),
		slog.String("video_id", videoID),
		slog.String("bucket", bucket),
		slog.String("object_path", objectPath),
		slog.String("resolved_region", resolvedRegion),
		slog.Bool("idempotent_replay", idempotentReplay),
	)

	uploadContentType := req.ContentType
	if storedUpload.MimeType.Valid {
		uploadContentType = storedUpload.MimeType.StringVal
	}

	uploadFileSize := req.FileSizeBytes
	if storedUpload.FileSizeBytes.Valid && storedUpload.FileSizeBytes.Int64 > 0 {
		uploadFileSize = storedUpload.FileSizeBytes.Int64
	}

	var uploadURL string
	var expiry time.Time

	err = apperror.RetryWithBackoff(ctx, 3, func() error {
		generatedURL, generatedExpiry, err := s.gcs.GenerateSignedResumableUploadInitURL(ctx, bucket, objectPath, uploadContentType, uploadFileSize, int(limits.SignedURLExpirationHours))
		if err != nil {
			return err
		}
		uploadURL = generatedURL
		expiry = generatedExpiry
		return nil
	})
	if err != nil {
		span.AddEvent("upload.signed_url.generation_failed")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to generate signed upload url")
		logger.Error(
			ctx,
			"upload.signed_url_generation_failed",
			"Failed to generate signed upload URL",
			slog.String("request_id", params.RequestID),
			slog.String("user_id", params.UserID),
			slog.String("video_id", videoID),
			slog.String("bucket", bucket),
			slog.String("object_path", objectPath),
			slog.String("content_type", uploadContentType),
			slog.Int64("file_size_bytes", uploadFileSize),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("%w: %w", ErrSignedURLGeneration, err)
	}

	response := &domain.CreateUploadResult{
		VideoID:            storedUpload.VideoID,
		UploadURL:          uploadURL,
		UploadURLExpiresAt: expiry,
		MaxFileSizeBytes:   limits.MaxFileSizeBytes,
	}
	span.SetAttributes(
		attribute.String("video.id", storedUpload.VideoID),
		attribute.Int64("video.max_file_size_bytes", limits.MaxFileSizeBytes),
		attribute.String("video.upload_url_expires_at", expiry.UTC().Format(time.RFC3339)),
	)
	span.AddEvent("upload.signed_url.generated")
	span.SetStatus(otelcodes.Ok, "signed upload url generated")
	logger.Info(
		ctx,
		"upload.signed_url_generated",
		"Generated signed upload URL",
		slog.String("request_id", params.RequestID),
		slog.String("user_id", params.UserID),
		slog.String("video_id", storedUpload.VideoID),
		slog.String("bucket", bucket),
		slog.String("object_path", objectPath),
		slog.Time("upload_url_expires_at", expiry),
		slog.Int64("max_file_size_bytes", limits.MaxFileSizeBytes),
		slog.Bool("idempotent_replay", idempotentReplay),
	)

	return response, nil
}

func (s *uploadService) validateFileSizeForTier(limits tier.TierLimits, fileSize int64) error {

	if fileSize > limits.MaxFileSizeBytes {
		return fmt.Errorf("%w: max_file_size_bytes=%d", ErrFileTooLarge, limits.MaxFileSizeBytes)
	}

	return nil
}

func (s *uploadService) resolveRegionAndBucket(region string) (string, string) {
	if region == "" {
		region = defaultRegion
	}

	bucket, ok := s.cfg.Buckets()[region]
	if !ok {
		region = defaultRegion
		bucket = s.cfg.Buckets()[defaultRegion]
	}

	return bucket, region
}

func (s *uploadService) buildObjectPath(userID, videoID, fileName string) string {
	return fmt.Sprintf("%s/%s/%s", userID, videoID, fileName)
}

func (s *uploadService) normalizeObjectPath(bucket, sourceObject string) string {
	prefix := fmt.Sprintf("gs://%s/", bucket)
	return strings.TrimPrefix(sourceObject, prefix)
}

func (s *uploadService) generateVideoID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

func (s *uploadService) executeSpannerTransaction(
	ctx context.Context, userID, requestID string, req domain.CreateUploadRequest, limits tier.TierLimits, videoRecord *video.Video) (*storedUploadRecord, error) {
	ctx, span := otel.Tracer(instrumentationName).Start(ctx,
		"upload-api.spanner.read-write-tx",
		trace.WithAttributes(
			attribute.String("video.request_id", requestID),
			attribute.String("video.user_id", userID),
			attribute.String("video.id", videoRecord.VideoID),
		),
	)
	defer span.End()

	var resultUpload *storedUploadRecord
	logger := s.logger.WithSpanContext(ctx)

	_, err := spannerutil.RunRW(ctx, s.spanner, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {

		decision, err := s.checkIdempotency(ctx, txn, userID, requestID, req)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUploadServiceUnavailable, err)
		}

		switch decision.kind {
		case idempotencyDecisionReplay:
			existingVideo := decision.record
			span.AddEvent("upload.idempotency.hit", trace.WithAttributes(
				attribute.String("video.existing_id", existingVideo.VideoID),
				attribute.String("video.status", string(existingVideo.Status)),
			))
			logger.Info(
				ctx,
				"upload.idempotency_hit",
				"Reused existing upload for request ID",
				slog.String("request_id", requestID),
				slog.String("user_id", userID),
				slog.String("existing_video_id", existingVideo.VideoID),
				slog.String("status", string(existingVideo.Status)),
			)
			resultUpload = existingVideo.storedUploadRecord()
			return nil
		case idempotencyDecisionMismatch:
			existingVideo := decision.record
			span.AddEvent("upload.idempotency.mismatch", trace.WithAttributes(
				attribute.String("video.existing_id", existingVideo.VideoID),
				attribute.String("upload.mismatched_fields", strings.Join(decision.mismatchedFields, ",")),
			))
			logger.Warn(
				ctx,
				"upload.idempotency_mismatch",
				"Rejected upload request with mismatched idempotent payload",
				slog.String("request_id", requestID),
				slog.String("user_id", userID),
				slog.String("existing_video_id", existingVideo.VideoID),
				slog.String("mismatched_fields", strings.Join(decision.mismatchedFields, ",")),
			)
			return &IdempotencyMismatchError{RequestID: requestID, MismatchedFields: decision.mismatchedFields}
		case idempotencyDecisionConsumed:
			existingVideo := decision.record
			span.AddEvent("upload.idempotency.consumed", trace.WithAttributes(
				attribute.String("video.existing_id", existingVideo.VideoID),
				attribute.String("video.status", string(existingVideo.Status)),
			))
			logger.Warn(
				ctx,
				"upload.request_id_already_consumed",
				"Rejected upload request for already-consumed request ID",
				slog.String("request_id", requestID),
				slog.String("user_id", userID),
				slog.String("existing_video_id", existingVideo.VideoID),
				slog.String("status", string(existingVideo.Status)),
			)
			return &RequestIDAlreadyConsumedError{RequestID: requestID, VideoID: existingVideo.VideoID, Status: string(existingVideo.Status)}
		}

		activeUploads, err := quota.CheckConcurrentLimit(ctx, txn, userID)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUploadServiceUnavailable, err)
		}

		if activeUploads >= limits.MaxConcurrentUploads {
			span.AddEvent("upload.limits.concurrent_exceeded", trace.WithAttributes(
				attribute.Int64("upload.active_uploads", activeUploads),
				attribute.Int64("upload.max_concurrent_uploads", limits.MaxConcurrentUploads),
			))
			logger.Warn(
				ctx,
				"upload.concurrent_limit_exceeded",
				"Rejected upload due to concurrent upload limit",
				slog.String("request_id", requestID),
				slog.String("user_id", userID),
				slog.Int64("active_uploads", activeUploads),
				slog.Int64("max_concurrent_uploads", limits.MaxConcurrentUploads),
			)
			return fmt.Errorf("%w: user_id=%q max_concurrent_uploads=%d", ErrConcurrentUploadLimit, userID, limits.MaxConcurrentUploads)
		}

		uploadsLastHour, err := quota.CheckHourlyRateLimit(ctx, txn, userID)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUploadServiceUnavailable, err)
		}

		if uploadsLastHour >= limits.MaxUploadsPerHour {
			span.AddEvent("upload.limits.hourly_exceeded", trace.WithAttributes(
				attribute.Int64("upload.uploads_last_hour", uploadsLastHour),
				attribute.Int64("upload.max_uploads_per_hour", limits.MaxUploadsPerHour),
			))
			logger.Warn(
				ctx,
				"upload.hourly_limit_exceeded",
				"Rejected upload due to hourly upload limit",
				slog.String("request_id", requestID),
				slog.String("user_id", userID),
				slog.Int64("uploads_last_hour", uploadsLastHour),
				slog.Int64("max_uploads_per_hour", limits.MaxUploadsPerHour),
			)
			return fmt.Errorf("%w: user_id=%q max_uploads_per_hour=%d", ErrHourlyUploadLimit, userID, limits.MaxUploadsPerHour)
		}

		totalStorageUsed, err := quota.CheckStorageQuota(ctx, txn, userID)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUploadServiceUnavailable, err)
		}

		totalStorageUsed += videoRecord.FileSizeBytes.Int64

		if totalStorageUsed > limits.StorageQuotaBytes {
			span.AddEvent("upload.limits.storage_quota_exceeded", trace.WithAttributes(
				attribute.Int64("upload.total_storage_used_bytes", totalStorageUsed),
				attribute.Int64("upload.storage_quota_bytes", limits.StorageQuotaBytes),
			))
			logger.Warn(
				ctx,
				"upload.storage_quota_exceeded",
				"Rejected upload due to storage quota limit",
				slog.String("request_id", requestID),
				slog.String("user_id", userID),
				slog.Int64("total_storage_used_bytes", totalStorageUsed),
				slog.Int64("storage_quota_bytes", limits.StorageQuotaBytes),
			)
			return fmt.Errorf("%w: user_id=%q storage_quota_bytes=%d", ErrStorageQuotaExceeded, userID, limits.StorageQuotaBytes)
		}

		if err := s.insertVideoRecord(ctx, txn, videoRecord); err != nil {
			return fmt.Errorf("%w: %w", ErrUploadServiceUnavailable, err)
		}

		if err := video.AppendLifecycleEvents(ctx, txn, videoRecord.VideoID, video.LifecycleEventParams{
			ToStatus: video.StatusUploading,
			Actor:    "upload-api",
			Reason:   "upload_created",
		}); err != nil {
			return fmt.Errorf("%w: %w", ErrUploadServiceUnavailable, err)
		}

		if err := video.InsertVideoStageRecord(ctx, txn, video.StageRecordParams{
			VideoID: videoRecord.VideoID,
			Stage:   video.StatusUploading,
			Attempt: 1,
			StartedAt: spanner.NullTime{
				Time:  spanner.CommitTimestamp,
				Valid: true,
			},
			CompletedAt: spanner.NullTime{
				Valid: false,
			},
			DurationMs: spanner.NullInt64{
				Valid: false,
			},
			Outcome: spanner.NullString{
				Valid: false,
			},
			Actor: "upload-api",
			ErrorID: spanner.NullString{
				Valid: false,
			},
		}); err != nil {
			return fmt.Errorf("%w: %w", ErrUploadServiceUnavailable, err)
		}

		logger.Info(
			ctx,
			"upload.record_persisted",
			"Persisted upload record and lifecycle state",
			slog.String("request_id", requestID),
			slog.String("user_id", userID),
			slog.String("video_id", videoRecord.VideoID),
			slog.String("bucket", videoRecord.SourceBucket),
			slog.String("object_path", videoRecord.SourceObject),
			slog.Int64("gcs_generation", videoRecord.GCSGeneration),
		)
		span.AddEvent("upload.record.persisted", trace.WithAttributes(
			attribute.String("storage.bucket", videoRecord.SourceBucket),
			attribute.String("storage.object", videoRecord.SourceObject),
			attribute.Int64("storage.gcs_generation", videoRecord.GCSGeneration),
		))

		resultUpload = storedUploadRecordFromVideo(videoRecord)

		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "spanner transaction failed")
		return nil, err
	}

	span.SetStatus(otelcodes.Ok, "spanner transaction committed")

	return resultUpload, nil
}

func (s *uploadService) insertVideoRecord(ctx context.Context, txn *spanner.ReadWriteTransaction, videoRecord *video.Video) error {

	mutation := spanner.InsertOrUpdate("videos",
		[]string{"video_id", "user_id", "request_id", "status", "source_bucket", "source_object",
			"gcs_generation", "mime_type", "file_size_bytes", "last_lifecycle_event_seq", "created_at", "updated_at"},
		[]any{videoRecord.VideoID, videoRecord.UserID, videoRecord.RequestID, videoRecord.Status,
			videoRecord.SourceBucket, videoRecord.SourceObject,
			videoRecord.GCSGeneration, videoRecord.MimeType, videoRecord.FileSizeBytes, videoRecord.LastLifecycleEventSeq, spanner.CommitTimestamp, spanner.CommitTimestamp},
	)

	return txn.BufferWrite([]*spanner.Mutation{mutation})
}

var _ UploadService = (*uploadService)(nil)
