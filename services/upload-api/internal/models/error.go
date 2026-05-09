package models

import "fmt"

type Status string

const (
	StatusInvalidArgument   Status = "INVALID_ARGUMENT"
	StatusUnauthenticated   Status = "UNAUTHENTICATED"
	StatusPermissionDenied  Status = "PERMISSION_DENIED"
	StatusNotFound          Status = "NOT_FOUND"
	StatusInternal          Status = "INTERNAL"
	StatusResourceExhausted Status = "RESOURCE_EXHAUSTED"
	StatusUnavailable       Status = "UNAVAILABLE"
)

type Reason string

const (
	ReasonInvalidFilename      Reason = "INVALID_FILENAME"
	ReasonInvalidContentType   Reason = "INVALID_CONTENT_TYPE"
	ReasonFileTooLarge         Reason = "FILE_TOO_LARGE"
	ReasonUploadLimitExceeded  Reason = "UPLOAD_LIMIT_EXCEEDED"
	ReasonHourlyRateExceeded   Reason = "HOURLY_RATE_EXCEEDED"
	ReasonStorageQuotaExceeded Reason = "STORAGE_QUOTA_EXCEEDED"
	ReasonInvalidRequestID     Reason = "INVALID_REQUEST_ID"
)

type ErrorResponse struct {
	HttpStatus int               `json:"-"`
	Code       int               `json:"code"`
	Message    string            `json:"message"`
	Status     Status            `json:"status"`
	Reason     Reason            `json:"reason,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Internal   error             `json:"-"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("[%s] %s", e.Status, e.Message)
}

func (e *ErrorResponse) Unwrap() error {
	return e.Internal
}

func NewErrorResponse(status Status, httpStatus int, message string) *ErrorResponse {
	return &ErrorResponse{
		HttpStatus: httpStatus,
		Code:       httpStatus,
		Message:    message,
		Status:     status,
	}
}

func (e *ErrorResponse) WithReason(reason Reason) *ErrorResponse {
	e.Reason = reason
	return e
}

func (e *ErrorResponse) WithMetadata(metadata map[string]string) *ErrorResponse {
	e.Metadata = metadata
	return e
}

func (e *ErrorResponse) WithInternal(err error) *ErrorResponse {
	e.Internal = err
	return e
}
