package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/logging"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/models"
)

func TestAuthenticationMissingAuthorizationHeaderReturnsJSON(t *testing.T) {
	logger := logging.New("upload-api", "asia-south1", "test")
	handler := Authentication(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}

	var payload models.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != models.StatusUnauthenticated {
		t.Fatalf("status = %q, want %q", payload.Status, models.StatusUnauthenticated)
	}
	if payload.Reason != models.ReasonMissingAuthorizationHeader {
		t.Fatalf("reason = %q, want %q", payload.Reason, models.ReasonMissingAuthorizationHeader)
	}
	if payload.Metadata["header"] != "Authorization" {
		t.Fatalf("header metadata = %q, want Authorization", payload.Metadata["header"])
	}
}
