package video

import (
	"time"

	"cloud.google.com/go/spanner"
)

type Video struct {
	VideoId            string             `spanner:"video_id"`
	UserId             string             `spanner:"user_id"`
	Status             Status             `spanner:"status"`
	SourceBucket       string             `spanner:"source_bucket"`
	SourceObject       string             `spanner:"source_object"`
	GCSGeneration      int64              `spanner:"gcs_generation"`
	MimeType           spanner.NullString `spanner:"mime_type"`
	FileSizeBytes      spanner.NullInt64  `spanner:"file_size_bytes"`
	DurationMs         spanner.NullInt64  `spanner:"duration_ms"`
	SourceWidth        spanner.NullInt64  `spanner:"source_width"`
	SourceHeight       spanner.NullInt64  `spanner:"source_height"`
	SourceCodec        spanner.NullString `spanner:"source_codec"`
	IsHDR              spanner.NullBool   `spanner:"is_hdr"`
	TranscodeProfile   spanner.NullString `spanner:"transcode_profile"`
	TranscoderJobId    spanner.NullString `spanner:"transcoder_job_id"`
	ThumbnailUri       spanner.NullString `spanner:"thumbnail_uri"`
	CaptionUri         spanner.NullString `spanner:"caption_uri"`
	ModerationDecision spanner.NullString `spanner:"moderation_decision"`

	ErrorDetails spanner.NullString `spanner:"error_details"`
	CreatedAt    time.Time          `spanner:"created_at"`
	UpdatedAt    time.Time          `spanner:"updated_at"`
}
