package models

import "fmt"

type Status string

const (
	StatusInvalidArgument   Status = "INVALID_ARGUMENT"
	StatusUnauthenticated   Status = "UNAUTHENTICATED"
	StatusPermissionDenied  Status = "PERMISSION_DENIED"
	StatusNotFound          Status = "NOT_FOUND"
	StatusConflict          Status = "CONFLICT"
	StatusInternal          Status = "INTERNAL"
	StatusResourceExhausted Status = "RESOURCE_EXHAUSTED"
	StatusUnavailable       Status = "UNAVAILABLE"
)

type Reason string

const (
	ReasonInvalidFilename            Reason = "INVALID_FILENAME"
	ReasonInvalidContentType         Reason = "INVALID_CONTENT_TYPE"
	ReasonFileTooLarge               Reason = "FILE_TOO_LARGE"
	ReasonUploadLimitExceeded        Reason = "UPLOAD_LIMIT_EXCEEDED"
	ReasonHourlyRateExceeded         Reason = "HOURLY_RATE_EXCEEDED"
	ReasonStorageQuotaExceeded       Reason = "STORAGE_QUOTA_EXCEEDED"
	ReasonIdempotencyMismatch        Reason = "IDEMPOTENCY_MISMATCH"
	ReasonRequestIDAlreadyConsumed   Reason = "REQUEST_ID_ALREADY_CONSUMED"
	ReasonInvalidRequestID           Reason = "INVALID_REQUEST_ID"
	ReasonMissingAuthorizationHeader Reason = "MISSING_AUTHORIZATION_HEADER"
	ReasonInvalidAuthorizationHeader Reason = "INVALID_AUTHORIZATION_HEADER"
	ReasonMissingBearerToken         Reason = "MISSING_BEARER_TOKEN"
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

func NewMissingAuthorizationHeaderError() *ErrorResponse {
	return NewErrorResponse(StatusUnauthenticated, 401, "missing Authorization header").WithReason(ReasonMissingAuthorizationHeader).WithMetadata(map[string]string{"header": "Authorization"})
}

func NewInvalidAuthorizationHeaderError() *ErrorResponse {
	return NewErrorResponse(StatusUnauthenticated, 401, "invalid Authorization header format").WithReason(ReasonInvalidAuthorizationHeader).WithMetadata(map[string]string{"header": "Authorization"})
}

func NewMissingBearerTokenError() *ErrorResponse {
	return NewErrorResponse(StatusUnauthenticated, 401, "missing token in Authorization header").WithReason(ReasonMissingBearerToken).WithMetadata(map[string]string{"header": "Authorization"})
}

func NewInvalidRequestIDError(received string) *ErrorResponse {
	return NewErrorResponse(StatusInvalidArgument, 400, "X-Request-ID must be a valid UUID").WithReason(ReasonInvalidRequestID).WithMetadata(map[string]string{"received_value": received})
}
