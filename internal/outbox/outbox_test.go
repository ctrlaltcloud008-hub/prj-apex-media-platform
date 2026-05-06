package outbox

import (
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestBuildAndParseEnvelope(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier(map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"tracestate":  "rojo=00f067aa0ba902b7,congo=t61rcWkgMzE",
	}))

	payload := map[string]any{"video_id": "video-123", "status": "VALIDATED"}
	env := BuildEnvelope(ctx, payload)

	if env.Traceparent == "" {
		t.Fatal("Traceparent is empty")
	}
	if env.Tracestate == "" {
		t.Fatal("Tracestate is empty")
	}

	encoded, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	decoded, err := ParseEnvelope(encoded)
	if err != nil {
		t.Fatalf("ParseEnvelope() error = %v", err)
	}

	if decoded.Traceparent != env.Traceparent {
		t.Fatalf("Traceparent = %q, want %q", decoded.Traceparent, env.Traceparent)
	}
	if decoded.Tracestate != env.Tracestate {
		t.Fatalf("Tracestate = %q, want %q", decoded.Tracestate, env.Tracestate)
	}

	data, ok := decoded.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data type = %T, want map[string]any", decoded.Data)
	}
	if data["video_id"] != "video-123" {
		t.Fatalf("video_id = %v, want video-123", data["video_id"])
	}
}
