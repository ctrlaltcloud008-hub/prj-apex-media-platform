package idempotency

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"
	"google.golang.org/grpc/codes"
)

type TranscodeSkipReason string

const (
	TranscodeSkipReasonProcessable        TranscodeSkipReason = "processable"
	TranscodeSkipReasonMissingVideo       TranscodeSkipReason = "missing_video"
	TranscodeSkipReasonStatusNotValidated TranscodeSkipReason = "status_not_validated"
	TranscodeSkipReasonJobAlreadySet      TranscodeSkipReason = "job_already_set"
)

type TranscodeDedupResult struct {
	ShouldSkip bool
	Reason     TranscodeSkipReason
}

func CheckDuplicateForTranscoding(ctx context.Context, txn *spanner.ReadWriteTransaction, videoID string) (TranscodeDedupResult, error) {
	if videoID == "" {
		return TranscodeDedupResult{}, fmt.Errorf("videoID cannot be empty")
	}

	row, err := txn.ReadRow(ctx, "videos", spanner.Key{videoID}, []string{"status", "transcoder_job_id"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return TranscodeDedupResult{ShouldSkip: true, Reason: TranscodeSkipReasonMissingVideo}, nil
		}
		return TranscodeDedupResult{}, fmt.Errorf("failed to read video record: %w", err)
	}

	var status, jobID string
	if err := row.ColumnByName("status", &status); err != nil {
		return TranscodeDedupResult{}, fmt.Errorf("failed to read status: %w", err)
	}
	if err := row.ColumnByName("transcoder_job_id", &jobID); err != nil {
		return TranscodeDedupResult{}, fmt.Errorf("failed to read transcoder_job_id: %w", err)
	}

	return transcodeDedupDecision(status, jobID), nil
}

func transcodeDedupDecision(status, jobID string) TranscodeDedupResult {
	if status != string(video.StatusValidated) {
		return TranscodeDedupResult{ShouldSkip: true, Reason: TranscodeSkipReasonStatusNotValidated}
	}
	if jobID != "" {
		return TranscodeDedupResult{ShouldSkip: true, Reason: TranscodeSkipReasonJobAlreadySet}
	}

	return TranscodeDedupResult{ShouldSkip: false, Reason: TranscodeSkipReasonProcessable}
}

func CheckGCSGeneration(ctx context.Context, txn *spanner.ReadWriteTransaction, videoID string, generation int64) (bool, error) {
	if videoID == "" {
		return false, fmt.Errorf("videoID cannot be empty")
	}

	if generation <= 0 {
		return false, fmt.Errorf("generation must be a positive integer")
	}

	row, err := txn.ReadRow(ctx, "videos", spanner.Key{videoID}, []string{"gcs_generation"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return false, nil // No existing record, so it's not a duplicate
		}
		return false, fmt.Errorf("failed to read video record: %w", err)
	}

	var storedGeneration int64
	if err := row.ColumnByName("gcs_generation", &storedGeneration); err != nil {
		return false, fmt.Errorf("failed to read gcs_generation: %w", err)
	}

	return storedGeneration == generation, nil
}

func TranscodeJobID(videoID, profile string, generation int64) (string, error) {
	if videoID == "" {
		return "", fmt.Errorf("videoID cannot be empty")
	}

	if profile == "" {
		return "", fmt.Errorf("profile cannot be empty")
	}

	if generation <= 0 {
		return "", fmt.Errorf("generation must be a positive integer")
	}

	return fmt.Sprintf("%s-%s-%d", videoID, profile, generation), nil
}

func VertexAIJobName(videoID, taskType string, generation int64) (string, error) {
	if videoID == "" {
		return "", fmt.Errorf("videoID cannot be empty")
	}

	if taskType == "" {
		return "", fmt.Errorf("taskType cannot be empty")
	}

	if generation <= 0 {
		return "", fmt.Errorf("generation must be a positive integer")
	}

	return fmt.Sprintf("%s-%s-%d", taskType, videoID, generation), nil
}
