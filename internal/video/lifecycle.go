package video

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
)

type LifecycleEventParams struct {
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

type lifecycleEventInsert struct {
	EventSeq int64
	Params   LifecycleEventParams
}

func AppendLifecycleEvents(ctx context.Context, txn *spanner.ReadWriteTransaction, videoID string, events ...LifecycleEventParams) error {
	if len(events) == 0 {
		return nil
	}

	baseSeq, err := currentLifecycleEventSeq(ctx, txn, videoID)
	if err != nil {
		return err
	}

	assigned, lastSeq := assignLifecycleEventSeqs(baseSeq, events)
	mutations := make([]*spanner.Mutation, 0, len(assigned)+1)
	for _, event := range assigned {
		mutations = append(mutations, spanner.Insert("video_lifecycle_events",
			[]string{"video_id", "event_seq", "from_status", "to_status", "actor", "reason", "details", "created_at"},
			[]any{videoID, event.EventSeq, event.Params.FromStatus, event.Params.ToStatus, event.Params.Actor, event.Params.Reason, event.Params.Details, spanner.CommitTimestamp},
		))
	}
	mutations = append(mutations, spanner.InsertOrUpdate("videos",
		[]string{"video_id", "last_lifecycle_event_seq"},
		[]any{videoID, lastSeq},
	))
	mutations[len(mutations)-1] = spanner.Update("videos",
		[]string{"video_id", "last_lifecycle_event_seq"},
		[]any{videoID, lastSeq},
	)

	return txn.BufferWrite(mutations)
}

func currentLifecycleEventSeq(ctx context.Context, txn *spanner.ReadWriteTransaction, videoID string) (int64, error) {
	row, err := txn.ReadRow(ctx, "videos", spanner.Key{videoID}, []string{"last_lifecycle_event_seq"})
	if err == nil {
		var lastSeq spanner.NullInt64
		if err := row.ColumnByName("last_lifecycle_event_seq", &lastSeq); err != nil {
			return 0, fmt.Errorf("read last lifecycle event sequence: %w", err)
		}
		if lastSeq.Valid {
			return lastSeq.Int64, nil
		}
		return maxLifecycleEventSeq(ctx, txn, videoID)
	}
	if spanner.ErrCode(err) != codes.NotFound {
		return 0, fmt.Errorf("read video lifecycle counter: %w", err)
	}

	return maxLifecycleEventSeq(ctx, txn, videoID)
}

func maxLifecycleEventSeq(ctx context.Context, txn *spanner.ReadWriteTransaction, videoID string) (int64, error) {
	stmt := spanner.Statement{
		SQL: `SELECT IFNULL(MAX(event_seq), 0) AS last_lifecycle_event_seq
		      FROM video_lifecycle_events
		      WHERE video_id = @video_id`,
		Params: map[string]any{
			"video_id": videoID,
		},
	}

	iter := txn.Query(ctx, stmt)
	defer iter.Stop()

	row, err := iter.Next()
	if err == iterator.Done {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("query max lifecycle event sequence: %w", err)
	}

	var lastSeq int64
	if err := row.ColumnByName("last_lifecycle_event_seq", &lastSeq); err != nil {
		return 0, fmt.Errorf("read max lifecycle event sequence: %w", err)
	}

	return lastSeq, nil
}

func assignLifecycleEventSeqs(baseSeq int64, events []LifecycleEventParams) ([]lifecycleEventInsert, int64) {
	assigned := make([]lifecycleEventInsert, 0, len(events))
	nextSeq := baseSeq
	for _, event := range events {
		nextSeq++
		assigned = append(assigned, lifecycleEventInsert{
			EventSeq: nextSeq,
			Params:   event,
		})
	}

	return assigned, nextSeq
}

func InsertVideoStageRecord(ctx context.Context, txn *spanner.ReadWriteTransaction, params StageRecordParams) error {
	mutation := spanner.Insert("video_stages",
		[]string{"video_id", "stage", "attempt", "started_at", "completed_at", "duration_ms", "actor", "outcome", "error_id"},
		[]any{params.VideoID, params.Stage, params.Attempt, params.StartedAt, params.CompletedAt, params.DurationMs, params.Actor, params.Outcome, params.ErrorID},
	)

	return txn.BufferWrite([]*spanner.Mutation{mutation})
}
