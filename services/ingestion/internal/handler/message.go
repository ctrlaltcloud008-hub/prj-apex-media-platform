package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/idempotency"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	pbclient "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/pubsub"
	spannerutil "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/event"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/gcs"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/metadata"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/profile"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/store"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/validation"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	metricapi "go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc/codes"
)

type Handler struct {
	logger       *logging.Logger
	subscriber   *pbclient.Subscriber
	gcsClient    *gcs.Client
	spanner      *spanner.Client
	sourceRegion string
}

func NewHandler(logger *logging.Logger,
	subscriber *pbclient.Subscriber,
	gcsClient *gcs.Client,
	spanner *spanner.Client,
	sourceRegion string) *Handler {
	return &Handler{
		logger:       logger,
		subscriber:   subscriber,
		gcsClient:    gcsClient,
		spanner:      spanner,
		sourceRegion: sourceRegion,
	}
}

func (h *Handler) Start(ctx context.Context) error {
	h.logger.Info(ctx, "handler.starting", "Starting message handler")
	return h.subscriber.Receive(ctx, h.handleMessage)
}

func (h *Handler) handleMessage(ctx context.Context, msg *pubsub.Message) {
	startedAt := time.Now()
	metric := getTelemetry()
	finalStatus := ""
	failureReason := ""

	ctx, span := pbclient.StartConsumerSpan(ctx, msg, "ingestion.process")
	defer span.End()
	defer recordProcessingMetrics(ctx, startedAt, h.sourceRegion, finalStatus, failureReason)

	logger := h.logger.WithSpanContext(ctx)
	logger.Info(ctx, "pubsub.message_received", "Received message")

	parseCtx, parseSpan := startStageSpan(ctx, "ingestion.parse-message")
	payload, err := event.ParseGCSFinalizeMessage(msg)
	if err != nil {
		recordSpanError(parseSpan, err, "failed to parse gcs finalize message")
		parseSpan.End()
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to parse gcs finalize message")
		finalStatus = "failed"
		failureReason = "message_parse"
		logger.Error(ctx, "pubsub.message_parse_error", "Failed to parse message", slog.String("error", err.Error()))
		msg.Ack()
		return
	}
	parseSpan.SetAttributes(
		attribute.String("messaging.message.id", payload.MessageID),
		attribute.String("storage.bucket", payload.Notification.Bucket),
		attribute.String("storage.object", payload.Notification.Name),
		attribute.Int64("storage.object.size", payload.Notification.Size),
		attribute.Int64("storage.object.generation", payload.Notification.Generation),
	)
	parseSpan.End()
	ctx = parseCtx
	metric.fileSizeBytes.Record(ctx, payload.Notification.Size, metricapi.WithAttributes(attribute.String("content_type", payload.Notification.ContentType)))

	validateCtx, validateSpan := startStageSpan(ctx,
		"ingestion.validate-gcs",
		attribute.String("storage.bucket", payload.Notification.Bucket),
		attribute.String("storage.object", payload.Notification.Name),
		attribute.Int64("storage.object.size", payload.Notification.Size),
	)
	exists, err := h.gcsClient.CheckObjectExists(validateCtx, payload.Notification.Bucket, payload.Notification.Name, payload.Notification.Size)
	if err != nil {
		recordSpanError(validateSpan, err, "failed to validate gcs object")
		validateSpan.End()
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to validate gcs object")
		finalStatus = "failed"
		failureReason = "gcs_validation"
		logger.Error(ctx, "pubsub.validation_error", "Failed to validate object existence", slog.String("bucket", payload.Notification.Bucket), slog.String("object", payload.Notification.Name), slog.String("error", err.Error()))
		msg.Ack()
		return
	}

	if !exists {
		validateSpan.SetStatus(otelcodes.Error, "gcs object not found")
		validateSpan.End()
		span.SetStatus(otelcodes.Error, "gcs object not found")
		finalStatus = "failed"
		failureReason = "object_not_found"
		logger.Error(ctx, "pubsub.object_not_found", "Object does not exist in GCS", slog.String("bucket", payload.Notification.Bucket), slog.String("object", payload.Notification.Name))
		msg.Ack()
		return
	}
	validateSpan.End()
	ctx = validateCtx

	userID, videoID, err := validation.ParseObjectPath(payload.Notification.Name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to parse object path")
		finalStatus = "failed"
		failureReason = "invalid_object_path"
		logger.Error(ctx,
			"pubsub.invalid_object_path",
			"Failed to parse object path",
			slog.String("objectPath", payload.Notification.Name),
			slog.String("error", err.Error()))
		msg.Ack()
		return
	}

	logger = logger.WithVideoID(videoID)
	generation := payload.Notification.Generation
	span.SetAttributes(
		attribute.String("video_id", videoID),
		attribute.String("user_id", userID),
		attribute.String("region", h.sourceRegion),
		attribute.String("storage.bucket", payload.Notification.Bucket),
		attribute.String("storage.object", payload.Notification.Name),
		attribute.Int64("generation", generation),
	)
	dedupCtx, dedupSpan := startStageSpan(ctx,
		"ingestion.dedup-check",
		attribute.String("video_id", videoID),
		attribute.Int64("generation", generation),
	)
	shouldContinue := true

	_, err = spannerutil.RunRW(dedupCtx, h.spanner, func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
		duplicate, err := idempotency.CheckGCSGeneration(ctx, tx, videoID, generation)
		if err != nil {
			return fmt.Errorf("check gcs generation: %w", err)
		}
		if duplicate {
			shouldContinue = false
			dedupSpan.SetAttributes(attribute.Bool("ingestion.is_duplicate", true), attribute.String("ingestion.dedup_reason", "generation_match"))
			logger.Info(ctx,
				"pubsub.duplicate_generation",
				"Skipping duplicate GCS finalize event",
				slog.Int64("generation", generation),
			)
			return nil
		}

		// Check Status != UPLOADING
		row, err := tx.ReadRow(ctx, "videos", spanner.Key{videoID}, []string{"status"})
		if err != nil {
			if spanner.ErrCode(err) == codes.NotFound {
				shouldContinue = false
				dedupSpan.SetAttributes(attribute.Bool("ingestion.is_duplicate", true), attribute.String("ingestion.dedup_reason", "video_not_found"))
				logger.Warn(ctx,
					"pubsub.video_not_found",
					"Video record not found in database, skipping message",
					slog.String("videoID", videoID),
				)
				return nil // No record found, skip processing
			}
			return fmt.Errorf("read video record: %w", err)
		}

		var status string
		if err := row.ColumnByName("status", &status); err != nil {
			return fmt.Errorf("read Status column: %w", err)
		}

		if status != "UPLOADING" {
			shouldContinue = false
			dedupSpan.SetAttributes(attribute.Bool("ingestion.is_duplicate", true), attribute.String("ingestion.dedup_reason", "invalid_status"), attribute.String("video.status", status))
			logger.Error(ctx,
				"pubsub.invalid_video_status",
				"Received GCS finalize event for video that is not in UPLOADING status",
				slog.String("videoID", videoID),
				slog.String("status", status),
			)
			return nil
		}

		// Continue with your normal write path here.
		// This is where you should also update videos.gcs_generation = generation
		// so future retries can be deduplicated.
		dedupSpan.SetAttributes(attribute.Bool("ingestion.is_duplicate", false))
		return nil
	})
	if err != nil {
		recordSpanError(dedupSpan, err, "failed to check gcs generation")
		dedupSpan.End()
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to check gcs generation")
		finalStatus = "failed"
		failureReason = "generation_check"
		logger.Error(ctx,
			"pubsub.generation_check_failed",
			"Failed to check GCS generation or video status",
			slog.Int64("generation", generation),
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}
	dedupSpan.End()
	ctx = dedupCtx
	if !shouldContinue {
		finalStatus = "dedup"
		msg.Ack()
		return
	}

	url, err := h.gcsClient.GenerateSignedURL(ctx, payload.Notification.Bucket, payload.Notification.Name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to generate signed url")
		finalStatus = "failed"
		failureReason = "signed_url"
		logger.Error(ctx,
			"pubsub.signed_url_error",
			"Failed to generate signed URL for GCS object",
			slog.String("bucket", payload.Notification.Bucket),
			slog.String("object", payload.Notification.Name),
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}

	ffprobeStartedAt := time.Now()
	ffprobeCtx, ffprobeSpan := startStageSpan(ctx,
		"ingestion.ffprobe",
		attribute.String("video_id", videoID),
	)
	videoMetadata, err := metadata.ExtractMetadata(ffprobeCtx, url)
	if err != nil {
		metric.ffprobeDuration.Record(ffprobeCtx, time.Since(ffprobeStartedAt).Seconds(), metricapi.WithAttributes(attribute.String("region", h.sourceRegion), attribute.String("outcome", "error")))
		recordSpanError(ffprobeSpan, err, "failed to extract metadata")
		ffprobeSpan.End()
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to extract metadata")
		finalStatus = "failed"
		failureReason = "metadata_extraction"
		logger.Error(ctx,
			"pubsub.metadata_extraction_failed",
			"Failed to extract metadata from video",
			slog.String("videoID", videoID),
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}
	metric.ffprobeDuration.Record(ffprobeCtx, time.Since(ffprobeStartedAt).Seconds(), metricapi.WithAttributes(attribute.String("region", h.sourceRegion), attribute.String("outcome", "success")))
	ffprobeSpan.SetAttributes(
		attribute.Int64("duration_ms", videoMetadata.DurationMs),
		attribute.Int("width", videoMetadata.Width),
		attribute.Int("height", videoMetadata.Height),
		attribute.String("codec", videoMetadata.Codec),
	)
	ffprobeSpan.End()
	ctx = ffprobeCtx

	profileCtx, profileSpan := startStageSpan(ctx,
		"ingestion.select-profile",
		attribute.String("video_id", videoID),
	)
	selectedProfile, err := profile.SelectTranscodeProfile(videoMetadata)
	if err != nil {
		recordSpanError(profileSpan, err, "failed to select transcode profile")
		profileSpan.End()
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to select transcode profile")
		finalStatus = "failed"
		failureReason = "profile_selection"
		logger.Error(ctx,
			"pubsub.profile_selection_failed",
			"Failed to select transcode profile from metadata",
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}
	metric.profileSelectedTotal.Add(profileCtx, 1, metricapi.WithAttributes(attribute.String("profile", selectedProfile)))
	profileSpan.SetAttributes(attribute.String("profile", selectedProfile))
	profileSpan.End()
	ctx = profileCtx

	logger.Info(ctx,
		"pubsub.profile_selected",
		"Selected transcode profile",
		slog.String("profile", selectedProfile),
		slog.Int("width", videoMetadata.Width),
		slog.Int("height", videoMetadata.Height),
		slog.Float64("fps", videoMetadata.FPS),
		slog.Bool("is_hdr", videoMetadata.IsHDR),
	)

	result := &store.ValidationResult{
		VideoID:      videoID,
		UserID:       userID,
		SourceBucket: payload.Notification.Bucket,
		SourceObject: payload.Notification.Name,
		SourceRegion: h.sourceRegion,
		Generation:   generation,
		Meta:         videoMetadata,
		Profile:      selectedProfile,
	}

	commitCtx, commitSpan := startStageSpan(ctx,
		"spanner.read-write-tx",
		attribute.String("video_id", videoID),
		attribute.String("profile", selectedProfile),
	)
	err = store.CommitValidation(commitCtx, h.spanner, result)
	if err != nil {
		if errors.Is(err, store.ErrValidationSkipped) {
			commitSpan.SetAttributes(attribute.Bool("ingestion.validation_skipped", true))
			commitSpan.End()
			finalStatus = "dedup"
			logger.Info(ctx,
				"pubsub.validation_skipped",
				"Skipped validation commit",
				slog.String("reason", err.Error()),
			)
			msg.Ack()
			return
		}

		recordSpanError(commitSpan, err, "failed to commit validation result")
		commitSpan.End()
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "failed to commit validation result")
		finalStatus = "failed"
		failureReason = "validation_commit"
		logger.Error(ctx,
			"pubsub.validation_commit_failed",
			"Failed to commit validation result",
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}
	commitSpan.End()
	finalStatus = "validated"
	span.SetStatus(otelcodes.Ok, "validated")
	logger.Info(ctx,
		"pubsub.validation_success",
		"Successfully validated video and committed result to database",
		slog.String("videoID", videoID),
		slog.String("profile", selectedProfile),
	)

	msg.Ack()
}
