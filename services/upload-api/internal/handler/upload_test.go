package handler

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/models"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/services"
)

func TestUploadErrorResponseIdempotencyMismatch(t *testing.T) {
	errPayload := uploadErrorResponse(&services.IdempotencyMismatchError{
		RequestID:        "req-123",
		MismatchedFields: []string{"filename", "content_type"},
	})

	if errPayload.HttpStatus != http.StatusConflict {
		t.Fatalf("uploadErrorResponse() status = %d, want %d", errPayload.HttpStatus, http.StatusConflict)
	}
	if errPayload.Status != models.StatusConflict {
		t.Fatalf("uploadErrorResponse() api status = %q, want %q", errPayload.Status, models.StatusConflict)
	}
	if errPayload.Reason != models.ReasonIdempotencyMismatch {
		t.Fatalf("uploadErrorResponse() reason = %q, want %q", errPayload.Reason, models.ReasonIdempotencyMismatch)
	}
	wantMetadata := map[string]string{"request_id": "req-123", "mismatched_fields": "filename,content_type"}
	if !reflect.DeepEqual(errPayload.Metadata, wantMetadata) {
		t.Fatalf("uploadErrorResponse() metadata = %#v, want %#v", errPayload.Metadata, wantMetadata)
	}
}

func TestUploadErrorResponseRequestIDAlreadyConsumed(t *testing.T) {
	errPayload := uploadErrorResponse(&services.RequestIDAlreadyConsumedError{
		RequestID: "req-123",
		VideoID:   "video-123",
		Status:    "VALIDATED",
	})

	if errPayload.HttpStatus != http.StatusConflict {
		t.Fatalf("uploadErrorResponse() status = %d, want %d", errPayload.HttpStatus, http.StatusConflict)
	}
	if errPayload.Status != models.StatusConflict {
		t.Fatalf("uploadErrorResponse() api status = %q, want %q", errPayload.Status, models.StatusConflict)
	}
	if errPayload.Reason != models.ReasonRequestIDAlreadyConsumed {
		t.Fatalf("uploadErrorResponse() reason = %q, want %q", errPayload.Reason, models.ReasonRequestIDAlreadyConsumed)
	}
	wantMetadata := map[string]string{"request_id": "req-123", "video_id": "video-123", "status": "VALIDATED"}
	if !reflect.DeepEqual(errPayload.Metadata, wantMetadata) {
		t.Fatalf("uploadErrorResponse() metadata = %#v, want %#v", errPayload.Metadata, wantMetadata)
	}
}
