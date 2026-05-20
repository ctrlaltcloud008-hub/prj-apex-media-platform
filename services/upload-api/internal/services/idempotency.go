package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/video"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/upload-api/internal/domain"
)

var (
	ErrIdempotencyMismatch      = errors.New("idempotency mismatch")
	ErrRequestIDAlreadyConsumed = errors.New("request id already consumed")
)

type IdempotencyMismatchError struct {
	RequestID        string
	MismatchedFields []string
}

func (e *IdempotencyMismatchError) Error() string {
	if len(e.MismatchedFields) == 0 {
		return fmt.Sprintf("%s: request_id=%q", ErrIdempotencyMismatch, e.RequestID)
	}
	return fmt.Sprintf("%s: request_id=%q mismatched_fields=%s", ErrIdempotencyMismatch, e.RequestID, strings.Join(e.MismatchedFields, ","))
}

func (e *IdempotencyMismatchError) Unwrap() error {
	return ErrIdempotencyMismatch
}

type RequestIDAlreadyConsumedError struct {
	RequestID string
	VideoID   string
	Status    string
}

func (e *RequestIDAlreadyConsumedError) Error() string {
	return fmt.Sprintf("%s: request_id=%q video_id=%q status=%s", ErrRequestIDAlreadyConsumed, e.RequestID, e.VideoID, e.Status)
}

func (e *RequestIDAlreadyConsumedError) Unwrap() error {
	return ErrRequestIDAlreadyConsumed
}

type storedUploadRecord struct {
	VideoID       string
	SourceBucket  string
	SourceObject  string
	MimeType      spanner.NullString
	FileSizeBytes spanner.NullInt64
}

type existingUploadRecord struct {
	VideoID       string             `spanner:"video_id"`
	Status        video.Status       `spanner:"status"`
	SourceBucket  string             `spanner:"source_bucket"`
	SourceObject  string             `spanner:"source_object"`
	MimeType      spanner.NullString `spanner:"mime_type"`
	FileSizeBytes spanner.NullInt64  `spanner:"file_size_bytes"`
}

func (r existingUploadRecord) storedUploadRecord() *storedUploadRecord {
	return &storedUploadRecord{
		VideoID:       r.VideoID,
		SourceBucket:  r.SourceBucket,
		SourceObject:  r.SourceObject,
		MimeType:      r.MimeType,
		FileSizeBytes: r.FileSizeBytes,
	}
}

type idempotencyDecisionKind string

const (
	idempotencyDecisionMiss     idempotencyDecisionKind = "miss"
	idempotencyDecisionReplay   idempotencyDecisionKind = "replay"
	idempotencyDecisionMismatch idempotencyDecisionKind = "mismatch"
	idempotencyDecisionConsumed idempotencyDecisionKind = "consumed"
)

type idempotencyDecision struct {
	kind             idempotencyDecisionKind
	record           *existingUploadRecord
	mismatchedFields []string
}

func (s *uploadService) checkIdempotency(ctx context.Context, txn *spanner.ReadWriteTransaction, userID, requestID string, req domain.CreateUploadRequest) (idempotencyDecision, error) {
	stmt := spanner.Statement{
		SQL: `SELECT video_id, status, source_bucket, source_object, mime_type, file_size_bytes
			  FROM videos
			  WHERE user_id = @user_id AND request_id = @request_id
			  ORDER BY created_at DESC
			  LIMIT 1`,
		Params: map[string]any{
			"user_id":    userID,
			"request_id": requestID,
		},
	}

	iter := txn.Query(ctx, stmt)
	defer iter.Stop()

	row, err := iter.Next()
	if err == iterator.Done {
		return idempotencyDecision{kind: idempotencyDecisionMiss}, nil
	}

	if err != nil {
		return idempotencyDecision{}, fmt.Errorf("query existing video for idempotency check: %w", err)
	}

	var existing existingUploadRecord
	if err := row.ToStruct(&existing); err != nil {
		return idempotencyDecision{}, fmt.Errorf("read existing upload for idempotency check: %w", err)
	}

	return s.idempotencyDecision(existing, req), nil
}

func (s *uploadService) idempotencyDecision(existing existingUploadRecord, req domain.CreateUploadRequest) idempotencyDecision {
	if existing.Status != video.StatusUploading {
		return idempotencyDecision{
			kind:   idempotencyDecisionConsumed,
			record: &existing,
		}
	}

	mismatchedFields := s.idempotencyMismatchedFields(existing, req)
	if len(mismatchedFields) > 0 {
		return idempotencyDecision{
			kind:             idempotencyDecisionMismatch,
			record:           &existing,
			mismatchedFields: mismatchedFields,
		}
	}

	return idempotencyDecision{
		kind:   idempotencyDecisionReplay,
		record: &existing,
	}
}

func (s *uploadService) idempotencyMismatchedFields(existing existingUploadRecord, req domain.CreateUploadRequest) []string {
	mismatchedFields := make([]string, 0, 3)

	if s.filenameFromSourceObject(existing.SourceBucket, existing.SourceObject) != req.Filename {
		mismatchedFields = append(mismatchedFields, "filename")
	}

	if !existing.MimeType.Valid || existing.MimeType.StringVal != req.ContentType {
		mismatchedFields = append(mismatchedFields, "content_type")
	}

	if !existing.FileSizeBytes.Valid || existing.FileSizeBytes.Int64 != req.FileSizeBytes {
		mismatchedFields = append(mismatchedFields, "file_size_bytes")
	}

	return mismatchedFields
}

func (s *uploadService) filenameFromSourceObject(bucket, sourceObject string) string {
	normalized := strings.TrimSuffix(s.normalizeObjectPath(bucket, sourceObject), "/")
	if normalized == "" {
		return ""
	}

	parts := strings.Split(normalized, "/")
	return parts[len(parts)-1]
}

func storedUploadRecordFromVideo(videoRecord *video.Video) *storedUploadRecord {
	return &storedUploadRecord{
		VideoID:       videoRecord.VideoID,
		SourceBucket:  videoRecord.SourceBucket,
		SourceObject:  videoRecord.SourceObject,
		MimeType:      videoRecord.MimeType,
		FileSizeBytes: videoRecord.FileSizeBytes,
	}
}
