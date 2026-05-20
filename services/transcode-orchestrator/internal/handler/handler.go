package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/idempotency"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	tier "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/models"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/quota"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"

	spannerutil "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"

	pbclient "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/pubsub"
)

type Handler struct {
	logger       *logging.Logger
	subscriber   *pbclient.Subscriber
	spanner      *spanner.Client
	sourceRegion string
}

func NewHandler(logger *logging.Logger, subscriber *pbclient.Subscriber, spannerClient *spanner.Client, sourceRegion string) *Handler {
	return &Handler{
		logger:       logger,
		subscriber:   subscriber,
		spanner:      spannerClient,
		sourceRegion: sourceRegion,
	}
}

func (h *Handler) Start(ctx context.Context) error {
	h.logger.Info(ctx, "handler.starting", "Starting transcode orchestrator handler")
	return h.subscriber.Receive(ctx, h.processMessage)
}

func (h *Handler) processMessage(ctx context.Context, msg *pubsub.Message) {

	data := msg.Data
	payload, err := parseMessage(data)
	if err != nil {
		h.logger.Error(ctx, "pubsub.message_parse_error", "Failed to parse message", slog.String("error", err.Error()))
		msg.Ack()
		return
	}

	videoID := payload.VideoID
	shouldContinue := true
	userID := payload.UserID
	// TODO: Fetch user tier from Spanner based on userID. For now, we will assume all users are free tier.
	userTier := tier.UserTierFree

	limits, err := tier.GetTierLimits(userTier)
	if err != nil {
		h.logger.Error(
			ctx,
			"upload.invalid_user_tier",
			"Failed to resolve tier limits for transcode orchestration",
			slog.String("user_id", userID),
			slog.String("user_tier", string(userTier)),
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}

	_, err = spannerutil.RunRW(ctx, h.spanner, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {

		result, err := idempotency.CheckDuplicateForTranscoding(ctx, txn, videoID)
		if err != nil {
			return fmt.Errorf("deduplication check failed: %w", err)
		}

		if result.ShouldSkip {
			h.logger.Info(ctx, "deduplication.duplicate_detected", "Skipping transcode orchestration for non-processable video state",
				slog.String("video_id", videoID),
				slog.String("skip_reason", string(result.Reason)),
			)
			shouldContinue = false
			return nil
		}

		activeJobs, err := quota.CheckConcurrentLimit(ctx, txn, payload.UserID)
		if err != nil {
			return fmt.Errorf("concurrent limit check failed: %w", err)
		}

		if activeJobs >= limits.MaxConcurrentUploads {
			return fmt.Errorf("concurrent upload limit reached: user_id=%q max_concurrent_uploads=%d", userID, limits.MaxConcurrentUploads)
		}

		jobID, err := idempotency.TranscodeJobID(videoID, h.sourceRegion, payload.GCSGeneration)
		if err != nil {
			return fmt.Errorf("failed to generate transcoding job ID: %w", err)
		}

		err = h.insertVideoRecord(ctx, txn, videoID, jobID)
		if err != nil {
			return fmt.Errorf("failed to insert video record: %w", err)
		}

		err = video.AppendLifecycleEvents(ctx, txn, videoID, video.LifecycleEventParams{
			FromStatus: video.StatusValidated,
			ToStatus:   video.StatusTranscoding,
			Reason:     "Transcoding started",
			Actor:      "trancode-orchestrator",
			Details:    map[string]any{"job_id": jobID},
		})

		if err != nil {
			return fmt.Errorf("failed to append lifecycle event: %w", err)
		}

		err = video.InsertVideoStageRecord(ctx, txn, video.StageRecordParams{
			VideoID:   videoID,
			Stage:     video.StatusTranscoding,
			Attempt:   1,
			Actor:     "trancode-orchestrator",
			StartedAt: spanner.NullTime{Time: spanner.CommitTimestamp, Valid: true},
		})

		if err != nil {
			return fmt.Errorf("failed to insert video stage record: %w", err)
		}

		return nil

	})
	if err != nil {
		h.logger.Error(ctx, "transcode.process_failed", "Failed to process transcode orchestration message",
			slog.String("video_id", videoID),
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		msg.Nack()
		return
	}

	if !shouldContinue {
		msg.Ack()
		return
	}

	msg.Ack()
}

func parseMessage(data []byte) (*video.VideoValidatedPayload, error) {

	var payload video.VideoValidatedPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message data %q: %w", string(data), err)
	}

	return &payload, nil
}

func (h *Handler) insertVideoRecord(ctx context.Context, txn *spanner.ReadWriteTransaction, videoID, jobID string) error {

	mutation := spanner.InsertOrUpdate("videos",
		[]string{"video_id", "status", "transcoder_job_id", "updated_at"},
		[]any{videoID, video.StatusTranscoding, jobID, spanner.CommitTimestamp},
	)

	return txn.BufferWrite([]*spanner.Mutation{mutation})
}
