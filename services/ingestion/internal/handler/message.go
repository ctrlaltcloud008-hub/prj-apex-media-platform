package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	ctx, span := pbclient.StartConsumerSpan(ctx, msg, "ingestion.handle_message")
	defer span.End()

	logger := h.logger.WithSpanContext(ctx)
	logger.Info(ctx, "pubsub.message_received", "Received message")

	payload, err := event.ParseGCSFinalizeMessage(msg)
	if err != nil {
		logger.Error(ctx, "pubsub.message_parse_error", "Failed to parse message", slog.String("error", err.Error()))
		msg.Ack()
		return
	}

	exists, err := h.gcsClient.CheckObjectExists(ctx, payload.Notification.Bucket, payload.Notification.Name, payload.Notification.Size)
	if err != nil {
		logger.Error(ctx, "pubsub.validation_error", "Failed to validate object existence", slog.String("bucket", payload.Notification.Bucket), slog.String("object", payload.Notification.Name), slog.String("error", err.Error()))
		msg.Ack()
		return
	}

	if !exists {
		logger.Error(ctx, "pubsub.object_not_found", "Object does not exist in GCS", slog.String("bucket", payload.Notification.Bucket), slog.String("object", payload.Notification.Name))
		msg.Ack()
		return
	}

	userID, videoID, err := validation.ParseObjectPath(payload.Notification.Name)
	if err != nil {
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
	shouldContinue := true

	_, err = spannerutil.RunRW(ctx, h.spanner, func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
		duplicate, err := idempotency.CheckGCSGeneration(ctx, tx, videoID, generation)
		if err != nil {
			return fmt.Errorf("check gcs generation: %w", err)
		}
		if duplicate {
			shouldContinue = false
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
		return nil
	})
	if err != nil {
		logger.Error(ctx,
			"pubsub.generation_check_failed",
			"Failed to check GCS generation or video status",
			slog.Int64("generation", generation),
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}
	if !shouldContinue {
		msg.Ack()
		return
	}

	url, err := h.gcsClient.GenerateSignedURL(ctx, payload.Notification.Bucket, payload.Notification.Name)
	if err != nil {
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

	videoMetadata, err := metadata.ExtractMetadata(ctx, url)
	if err != nil {
		logger.Error(ctx,
			"pubsub.metadata_extraction_failed",
			"Failed to extract metadata from video",
			slog.String("videoID", videoID),
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}

	selectedProfile, err := profile.SelectTranscodeProfile(videoMetadata)
	if err != nil {
		logger.Error(ctx,
			"pubsub.profile_selection_failed",
			"Failed to select transcode profile from metadata",
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}

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

	err = store.CommitValidation(ctx, h.spanner, result)
	if err != nil {
		if errors.Is(err, store.ErrValidationSkipped) {
			logger.Info(ctx,
				"pubsub.validation_skipped",
				"Skipped validation commit",
				slog.String("reason", err.Error()),
			)
			msg.Ack()
			return
		}

		logger.Error(ctx,
			"pubsub.validation_commit_failed",
			"Failed to commit validation result",
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}

	msg.Ack()
}
