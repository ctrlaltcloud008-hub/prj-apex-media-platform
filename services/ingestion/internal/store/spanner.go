package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/outbox"
	spannerutil "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/metadata"
	"google.golang.org/grpc/codes"
)

var ErrValidationSkipped = errors.New("validation skipped")

type ValidationResult struct {
	VideoID           string
	UserID            string
	SourceBucket      string
	SourceObject      string
	SourceRegion      string
	Generation        int64
	Meta              *metadata.VideoMetadata
	Profile           string
	UploadCompletedAt time.Time
	StartedAt         time.Time
	CompletedAt       time.Time
}

func (r *ValidationResult) validate() error {
	if r == nil {
		return fmt.Errorf("validation result is nil")
	}
	if r.VideoID == "" {
		return fmt.Errorf("videoID cannot be empty")
	}
	if r.UserID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	if r.SourceBucket == "" {
		return fmt.Errorf("source bucket cannot be empty")
	}
	if r.SourceObject == "" {
		return fmt.Errorf("source object cannot be empty")
	}
	if r.SourceRegion == "" {
		return fmt.Errorf("source region cannot be empty")
	}
	if r.Generation <= 0 {
		return fmt.Errorf("generation must be a positive integer")
	}
	if r.Meta == nil {
		return fmt.Errorf("metadata cannot be nil")
	}
	if r.Profile == "" {
		return fmt.Errorf("profile cannot be empty")
	}
	if r.UploadCompletedAt.IsZero() {
		return fmt.Errorf("uploadCompletedAt cannot be zero")
	}
	if r.StartedAt.IsZero() {
		return fmt.Errorf("startedAt cannot be zero")
	}
	if r.CompletedAt.IsZero() {
		return fmt.Errorf("completedAt cannot be zero")
	}
	if r.CompletedAt.Before(r.StartedAt) {
		return fmt.Errorf("completedAt cannot be before startedAt")
	}
	return nil
}

func (r *ValidationResult) durationMs() int64 {
	return r.CompletedAt.Sub(r.StartedAt).Milliseconds()
}

func (r *ValidationResult) sourceGCSURI() string {
	return fmt.Sprintf("gs://%s/%s", r.SourceBucket, r.SourceObject)
}

func buildVideoValidatedPayload(params *ValidationResult) video.VideoValidatedPayload {
	return video.VideoValidatedPayload{
		VideoID:          params.VideoID,
		UserID:           params.UserID,
		SourceGCSURI:     params.sourceGCSURI(),
		TranscodeProfile: params.Profile,
		SourceRegion:     params.SourceRegion,
		GCSGeneration:    params.Generation,
		Metadata: video.VideoValidatedMetadataPayload{
			DurationMs:  params.Meta.DurationMs,
			Width:       params.Meta.Width,
			Height:      params.Meta.Height,
			Codec:       params.Meta.Codec,
			FPS:         params.Meta.FPS,
			IsHDR:       params.Meta.IsHDR,
			BitrateKbps: params.Meta.BitrateBps / 1000,
		},
	}
}

func CommitValidation(ctx context.Context, client *spanner.Client, params *ValidationResult) error {
	if err := params.validate(); err != nil {
		return err
	}

	var skipReason string

	_, err := spannerutil.RunRW(ctx, client, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		row, err := txn.ReadRow(ctx, "videos", spanner.Key{params.VideoID}, []string{"status"})
		if err != nil {
			if spanner.ErrCode(err) == codes.NotFound {
				skipReason = fmt.Sprintf("video %q not found", params.VideoID)
				return nil
			}
			return fmt.Errorf("read video status: %w", err)
		}

		var status string
		if err := row.ColumnByName("status", &status); err != nil {
			return fmt.Errorf("read status column: %w", err)
		}

		if status != string(video.StatusUploading) {
			skipReason = fmt.Sprintf("video %q is in status %q", params.VideoID, status)
			return nil
		}

		err = insertVideoRecord(ctx, txn, params)
		if err != nil {
			return fmt.Errorf("buffer video update: %w", err)
		}

		if err := outbox.Write(ctx, txn, []outbox.Entry{{
			VideoID: params.VideoID,
			Topic:   "video.validated",
			Payload: buildVideoValidatedPayload(params),
		}}); err != nil {
			return fmt.Errorf("write outbox entry: %w", err)
		}

		err = completeUploadingStage(ctx, txn, params)
		if err != nil {
			return fmt.Errorf("complete uploading stage: %w", err)
		}

		err = video.AppendLifecycleEvents(ctx, txn, params.VideoID,
			video.LifecycleEventParams{
				FromStatus: video.StatusUploading,
				ToStatus:   video.StatusValidating,
				Actor:      "ingestion",
				Reason:     "validation started",
			},
			video.LifecycleEventParams{
				FromStatus: video.StatusValidating,
				ToStatus:   video.StatusValidated,
				Actor:      "ingestion",
				Reason:     "validation completed",
			},
		)
		if err != nil {
			return fmt.Errorf("append lifecycle events: %w", err)
		}

		err = video.InsertVideoStageRecord(ctx, txn, video.StageRecordParams{
			VideoID:     params.VideoID,
			Stage:       video.StatusValidating,
			Attempt:     1,
			StartedAt:   spanner.NullTime{Time: params.StartedAt.UTC(), Valid: true},
			CompletedAt: spanner.NullTime{Time: params.CompletedAt.UTC(), Valid: true},
			DurationMs:  spanner.NullInt64{Int64: params.durationMs(), Valid: true},
			Outcome:     spanner.NullString{StringVal: "SUCCESS", Valid: true},
			Actor:       "ingestion",
		})
		if err != nil {
			return fmt.Errorf("insert stage record: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}
	if skipReason != "" {
		return fmt.Errorf("%w: %s", ErrValidationSkipped, skipReason)
	}

	return err
}

func completeUploadingStage(ctx context.Context, txn *spanner.ReadWriteTransaction, params *ValidationResult) error {
	row, err := txn.ReadRow(ctx, "video_stages", spanner.Key{params.VideoID, string(video.StatusUploading), 1}, []string{"started_at"})
	if err != nil {
		return fmt.Errorf("read uploading stage: %w", err)
	}

	var startedAt time.Time
	if err := row.ColumnByName("started_at", &startedAt); err != nil {
		return fmt.Errorf("read uploading stage started_at: %w", err)
	}

	completedAt := params.UploadCompletedAt.UTC()
	if completedAt.Before(startedAt) {
		return fmt.Errorf("upload completed at %s before uploading stage start %s", completedAt.Format(time.RFC3339Nano), startedAt.Format(time.RFC3339Nano))
	}

	uploadingStageMutation := spanner.Update("video_stages",
		[]string{"video_id", "stage", "attempt", "completed_at", "duration_ms", "outcome"},
		[]any{params.VideoID, string(video.StatusUploading), 1, completedAt, completedAt.Sub(startedAt).Milliseconds(), spanner.NullString{StringVal: "SUCCEEDED", Valid: true}},
	)

	return txn.BufferWrite([]*spanner.Mutation{uploadingStageMutation})
}

func insertVideoRecord(ctx context.Context, txn *spanner.ReadWriteTransaction, params *ValidationResult) error {

	videoMutation := spanner.Update("videos",
		[]string{"video_id", "status", "gcs_generation", "duration_ms", "source_width", "source_height", "source_codec", "source_fps", "is_hdr", "transcode_profile", "updated_at"},
		[]any{params.VideoID, string(video.StatusValidated), params.Generation, params.Meta.DurationMs, params.Meta.Width, params.Meta.Height, params.Meta.Codec, params.Meta.FPS, params.Meta.IsHDR, params.Profile, spanner.CommitTimestamp},
	)
	return txn.BufferWrite([]*spanner.Mutation{videoMutation})
}
