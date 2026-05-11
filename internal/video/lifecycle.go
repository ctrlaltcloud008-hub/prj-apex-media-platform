package video

import (
	"context"

	"cloud.google.com/go/spanner"
)

type LifecycleEventParams struct {
	VideoID    string
	EventSeq   int64
	FromStatus Status
	ToStatus   Status
	Actor      string
	Reason     string
	Details    any
}
type StageRecordParams struct {
	VideoID     string
	Stage       Status
	Attempt     int64
	StartedAt   spanner.NullTime
	CompletedAt spanner.NullTime
	DurationMs  spanner.NullInt64
	Outcome     spanner.NullString
	ErrorID     spanner.NullString
	Actor       string
}

func InsertLifecycleEvent(ctx context.Context, txn *spanner.ReadWriteTransaction, params LifecycleEventParams) error {

	mutation := spanner.Insert("video_lifecycle_events",
		[]string{"video_id", "event_seq", "from_status", "to_status", "actor", "reason", "details", "created_at"},
		[]any{params.VideoID, params.EventSeq, params.FromStatus, params.ToStatus, params.Actor, params.Reason, params.Details, spanner.CommitTimestamp},
	)

	return txn.BufferWrite([]*spanner.Mutation{mutation})
}

func InsertVideoStageRecord(ctx context.Context, txn *spanner.ReadWriteTransaction, params StageRecordParams) error {
	mutation := spanner.Insert("video_stages",
		[]string{"video_id", "stage", "attempt", "started_at", "completed_at", "duration_ms", "actor", "outcome", "error_id"},
		[]any{params.VideoID, params.Stage, params.Attempt, params.StartedAt, params.CompletedAt, params.DurationMs, params.Actor, params.Outcome, params.ErrorID},
	)

	return txn.BufferWrite([]*spanner.Mutation{mutation})
}

