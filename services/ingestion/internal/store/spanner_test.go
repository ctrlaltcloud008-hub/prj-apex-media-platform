package store

import (
	"testing"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/metadata"
)

func TestBuildVideoValidatedPayload(t *testing.T) {
	params := &ValidationResult{
		VideoID:      "video-123",
		UserID:       "user-456",
		SourceBucket: "uploads-bucket",
		SourceObject: "user-456/video-123/source.mp4",
		SourceRegion: "asia-south1",
		Generation:   123456789,
		Meta: &metadata.VideoMetadata{
			DurationMs: 61000,
			Width:      1920,
			Height:     1080,
			Codec:      "h264",
			FPS:        29.97,
			BitrateBps: 8400000,
			IsHDR:      true,
		},
		Profile: "1080p-hdr",
	}

	payload := buildVideoValidatedPayload(params)

	if payload.VideoID != params.VideoID {
		t.Fatalf("VideoID = %q, want %q", payload.VideoID, params.VideoID)
	}
	if payload.UserID != params.UserID {
		t.Fatalf("UserID = %q, want %q", payload.UserID, params.UserID)
	}
	if payload.SourceGCSURI != "gs://uploads-bucket/user-456/video-123/source.mp4" {
		t.Fatalf("SourceGCSURI = %q", payload.SourceGCSURI)
	}
	if payload.SourceRegion != params.SourceRegion {
		t.Fatalf("SourceRegion = %q, want %q", payload.SourceRegion, params.SourceRegion)
	}
	if payload.TranscodeProfile != params.Profile {
		t.Fatalf("TranscodeProfile = %q, want %q", payload.TranscodeProfile, params.Profile)
	}
	if payload.Metadata.BitrateKbps != 8400 {
		t.Fatalf("BitrateKbps = %d, want 8400", payload.Metadata.BitrateKbps)
	}
	if !payload.Metadata.IsHDR {
		t.Fatal("IsHDR = false, want true")
	}
}
