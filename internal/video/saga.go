package video

import "fmt"

type SagaTaskType string

const (
	SagaTaskTypeThumbnail     SagaTaskType = "THUMBNAIL"
	SagaTaskTypeTranscription SagaTaskType = "TRANSCRIPTION"
	SagaTaskTypeModeration    SagaTaskType = "MODERATION"
)

func (s SagaTaskType) Validate() error {
	switch s {
	case SagaTaskTypeThumbnail, SagaTaskTypeTranscription, SagaTaskTypeModeration:
		return nil
	default:
		return fmt.Errorf("invalid saga task type (%q)", s)
	}
}

type SagaTaskStatus string

const (
	SagaTaskStatusPending    SagaTaskStatus = "PENDING"
	SagaTaskStatusInProgress SagaTaskStatus = "IN_PROGRESS"
	SagaTaskStatusCompleted  SagaTaskStatus = "COMPLETED"
	SagaTaskStatusFailed     SagaTaskStatus = "FAILED"
)

func (s SagaTaskStatus) IsTerminal() bool {
	switch s {
	case SagaTaskStatusCompleted, SagaTaskStatusFailed:
		return true
	default:
		return false
	}
}

func (s SagaTaskStatus) Validate() error {
	switch s {
	case SagaTaskStatusPending, SagaTaskStatusInProgress, SagaTaskStatusCompleted, SagaTaskStatusFailed:
		return nil
	default:
		return fmt.Errorf("invalid saga task status (%q)", s)
	}
}
