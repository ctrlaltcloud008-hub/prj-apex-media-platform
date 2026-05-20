package services

import (
	"reflect"
	"testing"

	"cloud.google.com/go/spanner"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/domain"
)

func TestIdempotencyDecisionReplay(t *testing.T) {
	svc := &uploadService{}

	decision := svc.idempotencyDecision(existingUploadRecord{
		VideoID:      "video-123",
		Status:       video.StatusUploading,
		SourceBucket: "bucket-a",
		SourceObject: "gs://bucket-a/user-456/video-123/my-video.mp4",
		MimeType: spanner.NullString{
			StringVal: "video/mp4",
			Valid:     true,
		},
		FileSizeBytes: spanner.NullInt64{
			Int64: 1024,
			Valid: true,
		},
	}, domain.CreateUploadRequest{
		Filename:      "my-video.mp4",
		ContentType:   "video/mp4",
		FileSizeBytes: 1024,
	})

	if decision.kind != idempotencyDecisionReplay {
		t.Fatalf("idempotencyDecision() kind = %q, want %q", decision.kind, idempotencyDecisionReplay)
	}
	if decision.record == nil || decision.record.VideoID != "video-123" {
		t.Fatalf("idempotencyDecision() record = %#v, want video-123", decision.record)
	}
}

func TestIdempotencyDecisionMismatch(t *testing.T) {
	svc := &uploadService{}

	decision := svc.idempotencyDecision(existingUploadRecord{
		VideoID:      "video-123",
		Status:       video.StatusUploading,
		SourceBucket: "bucket-a",
		SourceObject: "user-456/video-123/original.mp4",
		MimeType: spanner.NullString{
			StringVal: "video/mp4",
			Valid:     true,
		},
		FileSizeBytes: spanner.NullInt64{
			Int64: 1024,
			Valid: true,
		},
	}, domain.CreateUploadRequest{
		Filename:      "different.mp4",
		ContentType:   "video/webm",
		FileSizeBytes: 2048,
	})

	wantFields := []string{"filename", "content_type", "file_size_bytes"}
	if decision.kind != idempotencyDecisionMismatch {
		t.Fatalf("idempotencyDecision() kind = %q, want %q", decision.kind, idempotencyDecisionMismatch)
	}
	if !reflect.DeepEqual(decision.mismatchedFields, wantFields) {
		t.Fatalf("idempotencyDecision() mismatchedFields = %#v, want %#v", decision.mismatchedFields, wantFields)
	}
}

func TestIdempotencyDecisionConsumed(t *testing.T) {
	svc := &uploadService{}

	decision := svc.idempotencyDecision(existingUploadRecord{
		VideoID:      "video-123",
		Status:       video.StatusValidated,
		SourceBucket: "bucket-a",
		SourceObject: "user-456/video-123/my-video.mp4",
		MimeType: spanner.NullString{
			StringVal: "video/mp4",
			Valid:     true,
		},
		FileSizeBytes: spanner.NullInt64{
			Int64: 1024,
			Valid: true,
		},
	}, domain.CreateUploadRequest{
		Filename:      "my-video.mp4",
		ContentType:   "video/mp4",
		FileSizeBytes: 1024,
	})

	if decision.kind != idempotencyDecisionConsumed {
		t.Fatalf("idempotencyDecision() kind = %q, want %q", decision.kind, idempotencyDecisionConsumed)
	}
	if decision.record == nil || decision.record.Status != video.StatusValidated {
		t.Fatalf("idempotencyDecision() record = %#v, want status VALIDATED", decision.record)
	}
}
