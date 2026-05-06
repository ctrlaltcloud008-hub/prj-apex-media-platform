package pubsub

import (
	"context"
	"testing"

	cloudpubsub "cloud.google.com/go/pubsub/v2"
	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/internal/outbox"
)

func TestPublishFromOutboxCopiesStoredTraceAttributes(t *testing.T) {
	p := &Publisher{}
	msg := &cloudpubsub.Message{}
	env := outbox.Envelope{
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		Tracestate:  "rojo=00f067aa0ba902b7,congo=t61rcWkgMzE",
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic from nil publisher")
		}

		if msg.Attributes["traceparent"] != env.Traceparent {
			t.Fatalf("traceparent = %q, want %q", msg.Attributes["traceparent"], env.Traceparent)
		}
		if msg.Attributes["tracestate"] != env.Tracestate {
			t.Fatalf("tracestate = %q, want %q", msg.Attributes["tracestate"], env.Tracestate)
		}
	}()

	p.PublishFromOutbox(context.Background(), env, msg)
}
