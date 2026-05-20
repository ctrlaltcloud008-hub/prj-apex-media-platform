package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/gcs"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/idempotency"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	pbclient "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/pubsub"
	spannerutil "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/event"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/metadata"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/profile"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/store"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/validation"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
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

type messageProcessor struct {
	handler       *Handler
	msg           *pubsub.Message
	metric        ingestionTelemetry
	startedAt     time.Time
	ctx           context.Context
	span          trace.Span
	logger        *logging.Logger
	finalStatus   string
	failureReason string
	payload       *event.Message
	userID        string
	videoID       string
	generation    int64
	videoMetadata *metadata.VideoMetadata
	profile       string
}

func newMessageProcessor(h *Handler, ctx context.Context, msg *pubsub.Message) *messageProcessor {
	ctx, span := pbclient.StartConsumerSpan(ctx, msg, "ingestion.process")

	return &messageProcessor{
		handler:   h,
		msg:       msg,
		metric:    getTelemetry(),
		startedAt: time.Now(),
		ctx:       ctx,
		span:      span,
		logger:    h.logger.WithSpanContext(ctx),
	}
}

func (h *Handler) Start(ctx context.Context) error {
	h.logger.Info(ctx, "handler.starting", "Starting message handler")
	return h.subscriber.Receive(ctx, h.handleMessage)
}

func (h *Handler) handleMessage(ctx context.Context, msg *pubsub.Message) {
	processor := newMessageProcessor(h, ctx, msg)
	processor.process()
}

func (p *messageProcessor) process() {
	processingCtx := p.ctx

	defer p.span.End()
	defer recordProcessingMetrics(processingCtx, p.startedAt, p.handler.sourceRegion, p.finalStatus, p.failureReason)

	p.logger.Info(p.ctx, "pubsub.message_received", "Received message")

	if !p.parsePayload() {
		return
	}
	if !p.validateObject() {
		return
	}
	if !p.loadVideoContext() {
		return
	}
	if !p.checkDuplicateOrInvalidStatus() {
		return
	}

	signedURL, ok := p.generateSignedURL()
	if !ok {
		return
	}
	if !p.extractMetadata(signedURL) {
		return
	}
	if !p.selectProfile() {
		return
	}
	if !p.commitValidation() {
		return
	}

	p.completeValidation()
}

func (p *messageProcessor) parsePayload() bool {
	parseCtx, parseSpan := startStageSpan(p.ctx, "ingestion.parse-message")
	payload, err := event.ParseGCSFinalizeMessage(p.msg)
	if err != nil {
		recordSpanError(parseSpan, err, "failed to parse gcs finalize message")
		parseSpan.End()
		p.failProcessing(err, "failed to parse gcs finalize message", "message_parse")
		p.logger.Error(p.ctx, "pubsub.message_parse_error", "Failed to parse message", slog.String("error", err.Error()))
		p.msg.Ack()
		return false
	}

	parseSpan.SetAttributes(
		attribute.String("messaging.message.id", payload.MessageID),
		attribute.String("storage.bucket", payload.Notification.Bucket),
		attribute.String("storage.object", payload.Notification.Name),
		attribute.Int64("storage.object.size", payload.Notification.Size),
		attribute.Int64("storage.object.generation", payload.Notification.Generation),
	)
	parseSpan.End()

	p.ctx = parseCtx
	p.payload = payload
	p.metric.fileSizeBytes.Record(
		p.ctx,
		payload.Notification.Size,
		metricapi.WithAttributes(attribute.String("content_type", payload.Notification.ContentType)),
	)

	return true
}

func (p *messageProcessor) validateObject() bool {
	notification := p.payload.Notification
	validateCtx, validateSpan := startStageSpan(
		p.ctx,
		"ingestion.validate-gcs",
		attribute.String("storage.bucket", notification.Bucket),
		attribute.String("storage.object", notification.Name),
		attribute.Int64("storage.object.size", notification.Size),
	)

	exists, err := p.handler.gcsClient.CheckObjectExists(validateCtx, notification.Bucket, notification.Name, notification.Size)
	if err != nil {
		recordSpanError(validateSpan, err, "failed to validate gcs object")
		validateSpan.End()
		p.failProcessing(err, "failed to validate gcs object", "gcs_validation")
		p.logger.Error(
			p.ctx,
			"pubsub.validation_error",
			"Failed to validate object existence",
			slog.String("bucket", notification.Bucket),
			slog.String("object", notification.Name),
			slog.String("error", err.Error()),
		)
		p.msg.Ack()
		return false
	}

	if !exists {
		validateSpan.SetStatus(otelcodes.Error, "gcs object not found")
		validateSpan.End()
		p.failProcessing(nil, "gcs object not found", "object_not_found")
		p.logger.Error(
			p.ctx,
			"pubsub.object_not_found",
			"Object does not exist in GCS",
			slog.String("bucket", notification.Bucket),
			slog.String("object", notification.Name),
		)
		p.msg.Ack()
		return false
	}

	validateSpan.End()
	p.ctx = validateCtx

	return true
}

func (p *messageProcessor) loadVideoContext() bool {
	userID, videoID, err := validation.ParseObjectPath(p.payload.Notification.Name)
	if err != nil {
		p.failProcessing(err, "failed to parse object path", "invalid_object_path")
		p.logger.Error(
			p.ctx,
			"pubsub.invalid_object_path",
			"Failed to parse object path",
			slog.String("objectPath", p.payload.Notification.Name),
			slog.String("error", err.Error()),
		)
		p.msg.Ack()
		return false
	}

	p.userID = userID
	p.videoID = videoID
	p.generation = p.payload.Notification.Generation
	p.logger = p.logger.WithVideoID(videoID)
	p.span.SetAttributes(
		attribute.String("video_id", videoID),
		attribute.String("user_id", userID),
		attribute.String("region", p.handler.sourceRegion),
		attribute.String("storage.bucket", p.payload.Notification.Bucket),
		attribute.String("storage.object", p.payload.Notification.Name),
		attribute.Int64("generation", p.generation),
	)

	return true
}

func (p *messageProcessor) checkDuplicateOrInvalidStatus() bool {
	dedupCtx, dedupSpan := startStageSpan(
		p.ctx,
		"ingestion.dedup-check",
		attribute.String("video_id", p.videoID),
		attribute.Int64("generation", p.generation),
	)

	shouldContinue, err := p.shouldContinueProcessing(dedupCtx, dedupSpan)
	if err != nil {
		recordSpanError(dedupSpan, err, "failed to check gcs generation")
		dedupSpan.End()
		p.failProcessing(err, "failed to check gcs generation", "generation_check")
		p.logger.Error(
			p.ctx,
			"pubsub.generation_check_failed",
			"Failed to check GCS generation or video status",
			slog.Int64("generation", p.generation),
			slog.String("error", err.Error()),
		)
		p.msg.Nack()
		return false
	}

	dedupSpan.End()
	p.ctx = dedupCtx
	if !shouldContinue {
		p.finalStatus = "dedup"
		p.msg.Ack()
		return false
	}

	return true
}

func (p *messageProcessor) shouldContinueProcessing(ctx context.Context, dedupSpan trace.Span) (bool, error) {
	shouldContinue := true

	_, err := spannerutil.RunRW(ctx, p.handler.spanner, func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
		duplicate, err := idempotency.CheckGCSGeneration(ctx, tx, p.videoID, p.generation)
		if err != nil {
			return fmt.Errorf("check gcs generation: %w", err)
		}
		if duplicate {
			shouldContinue = false
			dedupSpan.SetAttributes(
				attribute.Bool("ingestion.is_duplicate", true),
				attribute.String("ingestion.dedup_reason", "generation_match"),
			)
			p.logger.Info(
				ctx,
				"pubsub.duplicate_generation",
				"Skipping duplicate GCS finalize event",
				slog.Int64("generation", p.generation),
			)
			return nil
		}

		row, err := tx.ReadRow(ctx, "videos", spanner.Key{p.videoID}, []string{"status"})
		if err != nil {
			if spanner.ErrCode(err) == codes.NotFound {
				shouldContinue = false
				dedupSpan.SetAttributes(
					attribute.Bool("ingestion.is_duplicate", true),
					attribute.String("ingestion.dedup_reason", "video_not_found"),
				)
				p.logger.Warn(
					ctx,
					"pubsub.video_not_found",
					"Video record not found in database, skipping message",
					slog.String("videoID", p.videoID),
				)
				return nil
			}

			return fmt.Errorf("read video record: %w", err)
		}

		var status string
		if err := row.ColumnByName("status", &status); err != nil {
			return fmt.Errorf("read Status column: %w", err)
		}

		if status != "UPLOADING" {
			shouldContinue = false
			dedupSpan.SetAttributes(
				attribute.Bool("ingestion.is_duplicate", true),
				attribute.String("ingestion.dedup_reason", "invalid_status"),
				attribute.String("video.status", status),
			)
			p.logger.Error(
				ctx,
				"pubsub.invalid_video_status",
				"Received GCS finalize event for video that is not in UPLOADING status",
				slog.String("videoID", p.videoID),
				slog.String("status", status),
			)
			return nil
		}

		dedupSpan.SetAttributes(attribute.Bool("ingestion.is_duplicate", false))
		return nil
	})

	return shouldContinue, err
}

func (p *messageProcessor) generateSignedURL() (string, bool) {
	notification := p.payload.Notification
	url, err := p.handler.gcsClient.GenerateSignedURL(p.ctx, notification.Bucket, notification.Name)
	if err != nil {
		p.failProcessing(err, "failed to generate signed url", "signed_url")
		p.logger.Error(
			p.ctx,
			"pubsub.signed_url_error",
			"Failed to generate signed URL for GCS object",
			slog.String("bucket", notification.Bucket),
			slog.String("object", notification.Name),
			slog.String("error", err.Error()),
		)
		p.msg.Nack()
		return "", false
	}

	return url, true
}

func (p *messageProcessor) extractMetadata(signedURL string) bool {
	ffprobeStartedAt := time.Now()
	ffprobeCtx, ffprobeSpan := startStageSpan(
		p.ctx,
		"ingestion.ffprobe",
		attribute.String("video_id", p.videoID),
	)

	videoMetadata, err := metadata.ExtractMetadata(ffprobeCtx, signedURL)
	if err != nil {
		p.metric.ffprobeDuration.Record(
			ffprobeCtx,
			time.Since(ffprobeStartedAt).Seconds(),
			metricapi.WithAttributes(attribute.String("region", p.handler.sourceRegion), attribute.String("outcome", "error")),
		)
		recordSpanError(ffprobeSpan, err, "failed to extract metadata")
		ffprobeSpan.End()
		p.failProcessing(err, "failed to extract metadata", "metadata_extraction")
		p.logger.Error(
			p.ctx,
			"pubsub.metadata_extraction_failed",
			"Failed to extract metadata from video",
			slog.String("videoID", p.videoID),
			slog.String("error", err.Error()),
		)
		p.msg.Nack()
		return false
	}

	p.metric.ffprobeDuration.Record(
		ffprobeCtx,
		time.Since(ffprobeStartedAt).Seconds(),
		metricapi.WithAttributes(attribute.String("region", p.handler.sourceRegion), attribute.String("outcome", "success")),
	)
	ffprobeSpan.SetAttributes(
		attribute.Int64("duration_ms", videoMetadata.DurationMs),
		attribute.Int("width", videoMetadata.Width),
		attribute.Int("height", videoMetadata.Height),
		attribute.String("codec", videoMetadata.Codec),
	)
	ffprobeSpan.End()

	p.ctx = ffprobeCtx
	p.videoMetadata = videoMetadata

	return true
}

func (p *messageProcessor) selectProfile() bool {
	profileCtx, profileSpan := startStageSpan(
		p.ctx,
		"ingestion.select-profile",
		attribute.String("video_id", p.videoID),
	)

	selectedProfile, err := profile.SelectTranscodeProfile(p.videoMetadata)
	if err != nil {
		recordSpanError(profileSpan, err, "failed to select transcode profile")
		profileSpan.End()
		p.failProcessing(err, "failed to select transcode profile", "profile_selection")
		p.logger.Error(
			p.ctx,
			"pubsub.profile_selection_failed",
			"Failed to select transcode profile from metadata",
			slog.String("error", err.Error()),
		)
		p.msg.Nack()
		return false
	}

	p.metric.profileSelectedTotal.Add(
		profileCtx,
		1,
		metricapi.WithAttributes(attribute.String("profile", selectedProfile)),
	)
	profileSpan.SetAttributes(attribute.String("profile", selectedProfile))
	profileSpan.End()

	p.ctx = profileCtx
	p.profile = selectedProfile
	p.logger.Info(
		p.ctx,
		"pubsub.profile_selected",
		"Selected transcode profile",
		slog.String("profile", selectedProfile),
		slog.Int("width", p.videoMetadata.Width),
		slog.Int("height", p.videoMetadata.Height),
		slog.Float64("fps", p.videoMetadata.FPS),
		slog.Bool("is_hdr", p.videoMetadata.IsHDR),
	)

	return true
}

func (p *messageProcessor) commitValidation() bool {
	commitCtx, commitSpan := startStageSpan(
		p.ctx,
		"spanner.read-write-tx",
		attribute.String("video_id", p.videoID),
		attribute.String("profile", p.profile),
	)

	err := store.CommitValidation(commitCtx, p.handler.spanner, p.validationResult())
	if err != nil {
		if errors.Is(err, store.ErrValidationSkipped) {
			commitSpan.SetAttributes(attribute.Bool("ingestion.validation_skipped", true))
			commitSpan.End()
			p.finalStatus = "dedup"
			p.logger.Info(
				p.ctx,
				"pubsub.validation_skipped",
				"Skipped validation commit",
				slog.String("reason", err.Error()),
			)
			p.msg.Ack()
			return false
		}

		recordSpanError(commitSpan, err, "failed to commit validation result")
		commitSpan.End()
		p.failProcessing(err, "failed to commit validation result", "validation_commit")
		p.logger.Error(
			p.ctx,
			"pubsub.validation_commit_failed",
			"Failed to commit validation result",
			slog.String("error", err.Error()),
		)
		p.msg.Nack()
		return false
	}

	commitSpan.End()
	p.ctx = commitCtx

	return true
}

func (p *messageProcessor) validationResult() *store.ValidationResult {
	uploadCompletedAt := p.payload.PublishTime
	if timeCreated, err := time.Parse(time.RFC3339, p.payload.Notification.TimeCreated); err == nil {
		uploadCompletedAt = timeCreated
	}

	return &store.ValidationResult{
		VideoID:           p.videoID,
		UserID:            p.userID,
		SourceBucket:      p.payload.Notification.Bucket,
		SourceObject:      p.payload.Notification.Name,
		SourceRegion:      p.handler.sourceRegion,
		Generation:        p.generation,
		Meta:              p.videoMetadata,
		Profile:           p.profile,
		UploadCompletedAt: uploadCompletedAt,
		StartedAt:         p.startedAt,
		CompletedAt:       time.Now(),
	}
}

func (p *messageProcessor) completeValidation() {
	p.finalStatus = "validated"
	p.span.SetStatus(otelcodes.Ok, "validated")
	p.logger.Info(
		p.ctx,
		"pubsub.validation_success",
		"Successfully validated video and committed result to database",
		slog.String("videoID", p.videoID),
		slog.String("profile", p.profile),
	)
	p.msg.Ack()
}

func (p *messageProcessor) failProcessing(err error, description, reason string) {
	if err != nil {
		p.span.RecordError(err)
	}

	p.span.SetStatus(otelcodes.Error, description)
	p.finalStatus = "failed"
	p.failureReason = reason
}
