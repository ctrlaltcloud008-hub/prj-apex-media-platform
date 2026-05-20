package idempotency

import (
	"testing"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"
)

func TestTranscodeDedupDecision(t *testing.T) {
	tests := []struct {
		name       string
		status     video.Status
		jobID      string
		wantSkip   bool
		wantReason TranscodeSkipReason
	}{
		{name: "processable validated without job id", status: video.StatusValidated, jobID: "", wantSkip: false, wantReason: TranscodeSkipReasonProcessable},
		{name: "skip when status not validated", status: video.StatusTranscoding, jobID: "", wantSkip: true, wantReason: TranscodeSkipReasonStatusNotValidated},
		{name: "skip when job already set", status: video.StatusValidated, jobID: "job-123", wantSkip: true, wantReason: TranscodeSkipReasonJobAlreadySet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transcodeDedupDecision(string(tt.status), tt.jobID)
			if got.ShouldSkip != tt.wantSkip {
				t.Fatalf("ShouldSkip = %v, want %v", got.ShouldSkip, tt.wantSkip)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestTranscodeDedupDecisionMissingVideo(t *testing.T) {
	got := TranscodeDedupResult{ShouldSkip: true, Reason: TranscodeSkipReasonMissingVideo}
	if !got.ShouldSkip {
		t.Fatal("ShouldSkip = false, want true")
	}
	if got.Reason != TranscodeSkipReasonMissingVideo {
		t.Fatalf("Reason = %q, want %q", got.Reason, TranscodeSkipReasonMissingVideo)
	}
}
