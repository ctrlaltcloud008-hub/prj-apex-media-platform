package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/models"
)

func TestRequestIDInvalidHeaderReturnsJSON(t *testing.T) {
	logger := logging.New("upload-api", "asia-south1", "test")
	handler := RequestID(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req.Header.Set("X-Request-ID", "not-a-uuid")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}

	var payload models.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != models.StatusInvalidArgument {
		t.Fatalf("status = %q, want %q", payload.Status, models.StatusInvalidArgument)
	}
	if payload.Reason != models.ReasonInvalidRequestID {
		t.Fatalf("reason = %q, want %q", payload.Reason, models.ReasonInvalidRequestID)
	}
	if payload.Metadata["received_value"] != "not-a-uuid" {
		t.Fatalf("received_value metadata = %q, want not-a-uuid", payload.Metadata["received_value"])
	}
}
