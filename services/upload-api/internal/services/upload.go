package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/apperror"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/gcs"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	spannerutil "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/config"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/models"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrInvalidUserTier          = errors.New("invalid user tier")
	ErrFileTooLarge             = errors.New("file too large")
	ErrConcurrentUploadLimit    = errors.New("concurrent upload limit reached")
	ErrHourlyUploadLimit        = errors.New("hourly upload limit reached")
	ErrStorageQuotaExceeded     = errors.New("storage quota exceeded")
	ErrSignedURLGeneration      = errors.New("signed URL generation failed")
	ErrUploadServiceUnavailable = errors.New("upload service unavailable")
)

const instrumentationName = "services/upload-api/internal/services"

type CreateUploadResult struct {
	VideoID            string
	UploadURL          string
	UploadURLExpiresAt time.Time
	MaxFileSizeBytes   int64
}

type Params struct {
	UserID    string
	UserTier  models.UserTier
	RequestID string
	Region    string
}

type UploadService interface {
	CreateUpload(ctx context.Context, params Params, req models.UploadRequest) (*CreateUploadResult, error)
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

func (s *uploadService) CreateUpload(ctx context.Context, params Params, req models.UploadRequest) (*CreateUploadResult, error) {
	ctx, span := otel.Tracer(instrumentationName).Start(ctx,
		"upload-api.create-upload.service",
		trace.WithAttributes(
			attribute.String("video.request_id", params.RequestID),
			attribute.String("video.user_id", params.UserID),
			attribute.String("video.user_tier", string(params.UserTier)),
			attribute.String("video.client_region", params.Region),
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
		slog.String("client_region", params.Region),
		slog.String("filename", req.Filename),
		slog.String("content_type", req.ContentType),
		slog.Int64("file_size_bytes", req.FileSizeBytes),
	)

	limits, err := s.tierLimits(params.UserTier)
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

	bucket, resolvedRegion := s.resolveRegionAndBucket(params.Region)
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
		VideoID:       videoID,
		UserID:        params.UserID,
		RequestID:     spanner.NullString{StringVal: params.RequestID, Valid: true},
		Status:        video.StatusUploading,
		SourceBucket:  bucket,
		SourceObject:  objectPath,
		GCSGeneration: 0,
		MimeType:      spanner.NullString{StringVal: req.ContentType, Valid: req.ContentType != ""},
		FileSizeBytes: spanner.NullInt64{Int64: req.FileSizeBytes, Valid: req.FileSizeBytes > 0},
	}

	storedVideo, err := s.executeSpannerTransaction(ctx, params.UserID, params.RequestID, limits, videoRecord)
	if err != nil {
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
	if storedVideo == nil {
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

	idempotentReplay := storedVideo.VideoID != videoRecord.VideoID
	videoID = storedVideo.VideoID
	bucket = storedVideo.SourceBucket
	objectPath = s.normalizeObjectPath(bucket, storedVideo.SourceObject)
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
	if storedVideo.MimeType.Valid {
		uploadContentType = storedVideo.MimeType.StringVal
	}

	uploadFileSize := req.FileSizeBytes
	if storedVideo.FileSizeBytes.Valid && storedVideo.FileSizeBytes.Int64 > 0 {
		uploadFileSize = storedVideo.FileSizeBytes.Int64
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

	response := &CreateUploadResult{
		VideoID:            storedVideo.VideoID,
		UploadURL:          uploadURL,
		UploadURLExpiresAt: expiry,
		MaxFileSizeBytes:   limits.MaxFileSizeBytes,
	}
	span.SetAttributes(
		attribute.String("video.id", storedVideo.VideoID),
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
		slog.String("video_id", storedVideo.VideoID),
		slog.String("bucket", bucket),
		slog.String("object_path", objectPath),
		slog.Time("upload_url_expires_at", expiry),
		slog.Int64("max_file_size_bytes", limits.MaxFileSizeBytes),
		slog.Bool("idempotent_replay", idempotentReplay),
	)

	return response, nil
}

func (s *uploadService) tierLimits(userTier models.UserTier) (models.TierLimits, error) {
	limits, ok := models.TierLimitsMap[userTier]
	if !ok {
		return models.TierLimits{}, fmt.Errorf("%w %q", ErrInvalidUserTier, userTier)
	}

	return limits, nil
}

func (s *uploadService) validateFileSizeForTier(limits models.TierLimits, fileSize int64) error {

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
	ctx context.Context, userID, requestID string, limits models.TierLimits, videoRecord *video.Video) (*video.Video, error) {
	ctx, span := otel.Tracer(instrumentationName).Start(ctx,
		"upload-api.spanner.read-write-tx",
		trace.WithAttributes(
			attribute.String("video.request_id", requestID),
			attribute.String("video.user_id", userID),
			attribute.String("video.id", videoRecord.VideoID),
		),
	)
	defer span.End()

	var resultVideo *video.Video
	logger := s.logger.WithSpanContext(ctx).WithVideoID(videoRecord.VideoID)

	_, err := spannerutil.RunRW(ctx, s.spanner, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {

		existingVideo, err := s.checkIdempotency(ctx, txn, userID, requestID)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUploadServiceUnavailable, err)
		}

		if existingVideo != nil {
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
			resultVideo = existingVideo
			return nil
		}

		activeUploads, err := s.checkConcurrentLimit(ctx, txn, userID)
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

		uploadsLastHour, err := s.checkHourlyRateLimit(ctx, txn, userID)
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

		totalStorageUsed, err := s.checkStorageQuota(ctx, txn, userID)
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

		if err := video.InsertLifecycleEvent(ctx, txn, video.LifecycleEventParams{
			VideoID:    videoRecord.VideoID,
			EventSeq:   1,
			FromStatus: nil,
			ToStatus:   video.StatusUploading,
			Actor:      "upload-api",
			Reason:     "upload_created",
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
			slog.String("bucket", videoRecord.SourceBucket),
			slog.String("object_path", videoRecord.SourceObject),
			slog.Int64("gcs_generation", videoRecord.GCSGeneration),
		)
		span.AddEvent("upload.record.persisted", trace.WithAttributes(
			attribute.String("storage.bucket", videoRecord.SourceBucket),
			attribute.String("storage.object", videoRecord.SourceObject),
			attribute.Int64("storage.gcs_generation", videoRecord.GCSGeneration),
		))

		resultVideo = videoRecord

		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "spanner transaction failed")
		return nil, err
	}

	span.SetStatus(otelcodes.Ok, "spanner transaction committed")

	return resultVideo, nil
}

func (s *uploadService) checkIdempotency(ctx context.Context, txn *spanner.ReadWriteTransaction, userID, requestID string) (*video.Video, error) {

	stmt := spanner.Statement{
		SQL: `SELECT video_id, user_id, request_id, status, source_bucket, source_object,
				     gcs_generation, mime_type, file_size_bytes, duration_ms, source_width,
				     source_height, source_codec, source_fps, is_hdr, transcode_profile,
				     transcoder_job_id, thumbnail_uri, caption_uri, moderation_decision,
				     content_rating_hint, error_details, created_at, updated_at
			  FROM videos
			  WHERE user_id = @user_id AND request_id = @request_id
			  ORDER BY created_at DESC
			  LIMIT 1`,
		Params: map[string]any{
			"user_id":    userID,
			"request_id": requestID,
		},
	}

	iter := txn.Query(ctx, stmt)
	defer iter.Stop()

	row, err := iter.Next()
	if err == iterator.Done {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("query existing video for idempotency check: %w", err)
	}

	var existingVideo video.Video
	if err := row.ToStruct(&existingVideo); err != nil {
		return nil, fmt.Errorf("read video row for idempotency check: %w", err)
	}

	return &existingVideo, nil

}

func (s *uploadService) checkConcurrentLimit(ctx context.Context, txn *spanner.ReadWriteTransaction, userID string) (int64, error) {

	stmt := spanner.Statement{
		SQL: `SELECT COUNT(1) AS active_uploads
			  FROM videos
			  WHERE user_id = @user_id AND status = @status`,
		Params: map[string]any{
			"user_id": userID,
			"status":  video.StatusUploading,
		},
	}

	iter := txn.Query(ctx, stmt)
	defer iter.Stop()

	row, err := iter.Next()
	if err == iterator.Done {
		return 0, nil
	}

	if err != nil {
		return 0, fmt.Errorf("query active uploads for concurrent limit check: %w", err)
	}

	var activeUploads int64
	if err := row.ColumnByName("active_uploads", &activeUploads); err != nil {
		return 0, fmt.Errorf("read active uploads count for concurrent limit check: %w", err)
	}

	return activeUploads, nil
}

func (s *uploadService) checkHourlyRateLimit(ctx context.Context, txn *spanner.ReadWriteTransaction, userID string) (int64, error) {

	stmt := spanner.Statement{
		SQL: `SELECT COUNT(1) AS uploads_last_hour
			  FROM videos
			  WHERE user_id = @user_id AND status = @status AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 HOUR)`,
		Params: map[string]any{
			"user_id": userID,
			"status":  video.StatusUploading,
		},
	}

	iter := txn.Query(ctx, stmt)
	defer iter.Stop()

	row, err := iter.Next()
	if err == iterator.Done {
		return 0, nil
	}

	if err != nil {
		return 0, fmt.Errorf("query uploads in last hour for rate limit check: %w", err)
	}

	var uploadsLastHour int64
	if err := row.ColumnByName("uploads_last_hour", &uploadsLastHour); err != nil {
		return 0, fmt.Errorf("read uploads in last hour count for rate limit check: %w", err)
	}

	return uploadsLastHour, nil
}

func (s *uploadService) checkStorageQuota(ctx context.Context, txn *spanner.ReadWriteTransaction, userID string) (int64, error) {

	stmt := spanner.Statement{
		SQL: `SELECT IFNULL(SUM(file_size_bytes), 0) AS total_storage_used
			  FROM videos
			  WHERE user_id = @user_id AND status NOT IN (@status_failed, @status_expired)`,
		Params: map[string]any{
			"user_id":        userID,
			"status_failed":  video.StatusFailed,
			"status_expired": video.StatusExpired,
		},
	}

	iter := txn.Query(ctx, stmt)
	defer iter.Stop()

	row, err := iter.Next()
	if err == iterator.Done {
		return 0, nil
	}

	if err != nil {
		return 0, fmt.Errorf("query total storage used for storage quota check: %w", err)
	}

	var totalStorageUsed int64
	if err := row.ColumnByName("total_storage_used", &totalStorageUsed); err != nil {
		return 0, fmt.Errorf("read total storage used for storage quota check: %w", err)
	}

	return totalStorageUsed, nil
}

func (s *uploadService) insertVideoRecord(ctx context.Context, txn *spanner.ReadWriteTransaction, videoRecord *video.Video) error {

	mutation := spanner.InsertOrUpdate("videos",
		[]string{"video_id", "user_id", "request_id", "status", "source_bucket", "source_object",
			"gcs_generation", "mime_type", "file_size_bytes", "created_at", "updated_at"},
		[]any{videoRecord.VideoID, videoRecord.UserID, videoRecord.RequestID, videoRecord.Status,
			videoRecord.SourceBucket, videoRecord.SourceObject,
			videoRecord.GCSGeneration, videoRecord.MimeType, videoRecord.FileSizeBytes, spanner.CommitTimestamp, spanner.CommitTimestamp},
	)

	return txn.BufferWrite([]*spanner.Mutation{mutation})
}

var _ UploadService = (*uploadService)(nil)
