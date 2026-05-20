package video

import "testing"

func TestAssignLifecycleEventSeqs(t *testing.T) {
	events := []LifecycleEventParams{
		{FromStatus: StatusUploading, ToStatus: StatusValidating, Actor: "ingestion", Reason: "validation started"},
		{FromStatus: StatusValidating, ToStatus: StatusValidated, Actor: "ingestion", Reason: "validation completed"},
	}

	assigned, lastSeq := assignLifecycleEventSeqs(1, events)
	if lastSeq != 3 {
		t.Fatalf("lastSeq = %d, want 3", lastSeq)
	}
	if len(assigned) != 2 {
		t.Fatalf("len(assigned) = %d, want 2", len(assigned))
	}
	if assigned[0].EventSeq != 2 {
		t.Fatalf("assigned[0].EventSeq = %d, want 2", assigned[0].EventSeq)
	}
	if assigned[1].EventSeq != 3 {
		t.Fatalf("assigned[1].EventSeq = %d, want 3", assigned[1].EventSeq)
	}
}

func TestAssignLifecycleEventSeqsFromZero(t *testing.T) {
	assigned, lastSeq := assignLifecycleEventSeqs(0, []LifecycleEventParams{{ToStatus: StatusUploading, Actor: "upload-api", Reason: "upload_created"}})
	if lastSeq != 1 {
		t.Fatalf("lastSeq = %d, want 1", lastSeq)
	}
	if len(assigned) != 1 {
		t.Fatalf("len(assigned) = %d, want 1", len(assigned))
	}
	if assigned[0].EventSeq != 1 {
		t.Fatalf("assigned[0].EventSeq = %d, want 1", assigned[0].EventSeq)
	}
}
