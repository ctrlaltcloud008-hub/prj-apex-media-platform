package error

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/cenkalti/backoff/v5"
	"github.com/google/uuid"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ErrorType string

const (
	Transient ErrorType = "TRANSIENT"
	Permanent ErrorType = "PERMANENT"
	Ambiguous ErrorType = "AMBIGUOUS"
)

type ProcessingError struct {
	VideoID    string
	ErrorType  ErrorType
	Service    string
	Operation  string
	ErrorCode  string
	Message    string
	RetryCount int
	Timestamp  time.Time
}

func (e *ProcessingError) Error() string {
	return fmt.Sprintf("%s: [%s] %s: %s", e.Service, e.ErrorType, e.Operation, e.Message)
}

func classify(err error) ErrorType {
	if err == nil {
		return Permanent
	}

	if spanner.ErrCode(err) == codes.Aborted {
		return Transient
	}

	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
			return Transient
		case codes.InvalidArgument, codes.NotFound, codes.PermissionDenied, codes.AlreadyExists, codes.FailedPrecondition, codes.Unimplemented:
			return Permanent
		}
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
			return Transient
		case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict:
			return Permanent
		}
	}

	return Ambiguous
}

func RetryWithBackoff(ctx context.Context, maxRetries uint, fn func() error) error {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 1 * time.Second
	bo.MaxInterval = 16 * time.Second
	bo.Multiplier = 2

	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		err := fn()
		if err == nil {
			return struct{}{}, nil
		}
		if classify(err) == Transient {
			return struct{}{}, backoff.Permanent(err)
		}

		return struct{}{}, err
	}, backoff.WithBackOff(bo), backoff.WithMaxTries(uint(maxRetries+1)))

	return err

}

func RecordFailure(ctx context.Context, tx *spanner.ReadWriteTransaction, pe *ProcessingError) error {

	if pe.VideoID == "" {
		return fmt.Errorf("VideoID is required")
	}
	if pe.ErrorType != Permanent {
		return fmt.Errorf("Record failure is only for PERMANENT errors, got %s", pe.ErrorType)
	}

	failureID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generating failure ID: %w", err)
	}

	failureMut := spanner.Insert("video_failures",
		[]string{"video_id", "failure_id", "stage", "error_type", "error_code", "message", "service", "operation", "retry_count", "created_at"},
		[]interface{}{pe.VideoID, failureID.String(), pe.Operation, string(pe.ErrorType), pe.ErrorCode, pe.Message, pe.Service, pe.Operation, pe.RetryCount, spanner.CommitTimestamp},
	)

	videoMut := spanner.Update("videos",
		[]string{"video_id", "status", "error_details", "updated_at"},
		[]interface{}{pe.VideoID, "FAILED", spanner.NullJSON{Value: map[string]interface{}{
			"error_type": string(pe.ErrorType),
			"error_code": pe.ErrorCode,
			"message":    pe.Message,
			"service":    pe.Service,
			"operation":  pe.Operation,
		}, Valid: true}, spanner.CommitTimestamp})
	return tx.BufferWrite([]*spanner.Mutation{failureMut, videoMut})
}
