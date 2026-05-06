package store

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/outbox"
	spannerutil "github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/metadata"
	"google.golang.org/grpc/codes"
)

var ErrValidationSkipped = errors.New("validation skipped")

type ValidationResult struct {
	VideoID      string
	UserID       string
	SourceBucket string
	SourceObject string
	SourceRegion string
	Generation   int64
	Meta         *metadata.VideoMetadata
	Profile      string
}

type videoValidatedPayload struct {
	VideoID          string                        `json:"video_id"`
	UserID           string                        `json:"user_id"`
	SourceGCSURI     string                        `json:"source_gcs_uri"`
	TranscodeProfile string                        `json:"transcode_profile"`
	SourceRegion     string                        `json:"source_region"`
	GCSGeneration    int64                         `json:"gcs_generation"`
	Metadata         videoValidatedMetadataPayload `json:"metadata"`
}

type videoValidatedMetadataPayload struct {
	DurationMs  int64   `json:"duration_ms"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Codec       string  `json:"codec"`
	FPS         float64 `json:"fps"`
	IsHDR       bool    `json:"is_hdr"`
	BitrateKbps int64   `json:"bitrate_kbps"`
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
	return nil
}

func (r *ValidationResult) sourceGCSURI() string {
	return fmt.Sprintf("gs://%s/%s", r.SourceBucket, r.SourceObject)
}

func buildVideoValidatedPayload(params *ValidationResult) videoValidatedPayload {
	return videoValidatedPayload{
		VideoID:          params.VideoID,
		UserID:           params.UserID,
		SourceGCSURI:     params.sourceGCSURI(),
		TranscodeProfile: params.Profile,
		SourceRegion:     params.SourceRegion,
		GCSGeneration:    params.Generation,
		Metadata: videoValidatedMetadataPayload{
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

		videoMutation := spanner.Update("videos",
			[]string{"video_id", "status", "gcs_generation", "duration_ms", "source_width", "source_height", "source_codec", "source_fps", "is_hdr", "transcode_profile", "updated_at"},
			[]any{params.VideoID, string(video.StatusValidated), params.Generation, params.Meta.DurationMs, params.Meta.Width, params.Meta.Height, params.Meta.Codec, params.Meta.FPS, params.Meta.IsHDR, params.Profile, spanner.CommitTimestamp},
		)
		if err := txn.BufferWrite([]*spanner.Mutation{videoMutation}); err != nil {
			return fmt.Errorf("buffer video update: %w", err)
		}

		if err := outbox.Write(ctx, txn, []outbox.Entry{{
			VideoID: params.VideoID,
			Topic:   "video.validated",
			Payload: buildVideoValidatedPayload(params),
		}}); err != nil {
			return fmt.Errorf("write outbox entry: %w", err)
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
