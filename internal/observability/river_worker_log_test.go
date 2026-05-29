package observability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRiverWorkerLogMiddlewareInjectsTraceMetadata(t *testing.T) {
	traceID := trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	spanID := trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	params := []*rivertype.JobInsertParams{{
		Kind:     "vm_create",
		Queue:    "vm_operations",
		Metadata: []byte(`{"existing":{"kept":true}}`),
	}}

	middleware := NewRiverWorkerLogMiddleware(zap.NewNop())
	_, err := middleware.InsertMany(ctx, params, func(context.Context) ([]*rivertype.JobInsertResult, error) {
		return []*rivertype.JobInsertResult{}, nil
	})
	if err != nil {
		t.Fatalf("InsertMany() error = %v", err)
	}

	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(params[0].Metadata, &metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if _, ok := metadata["existing"]; !ok {
		t.Fatalf("existing metadata key was not preserved: %s", params[0].Metadata)
	}

	var traceMetadata riverTraceMetadata
	if err := json.Unmarshal(metadata[riverTraceMetadataKey], &traceMetadata); err != nil {
		t.Fatalf("unmarshal trace metadata: %v", err)
	}
	if traceMetadata.TraceID != traceID.String() {
		t.Fatalf("trace_id = %q, want %q", traceMetadata.TraceID, traceID.String())
	}
	if traceMetadata.SpanID != spanID.String() {
		t.Fatalf("span_id = %q, want %q", traceMetadata.SpanID, spanID.String())
	}
}

func TestRiverWorkerLogMiddlewareLogsCompletionFields(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	middleware := NewRiverWorkerLogMiddleware(zap.New(core))

	err := middleware.Work(context.Background(), riverWorkerLogTestJob(t), func(context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].Message != riverWorkerLogEvent {
		t.Fatalf("message = %q, want %q", entries[0].Message, riverWorkerLogEvent)
	}
	if entries[0].Level != zap.InfoLevel {
		t.Fatalf("level = %v, want info", entries[0].Level)
	}

	fields := entries[0].ContextMap()
	requireAllowedRiverWorkerLogFields(t, fields)
	want := map[string]interface{}{
		"job_id":       int64(42),
		"job_kind":     "vm_create",
		"queue":        "vm_operations",
		"attempt":      int64(2),
		"max_attempts": int64(5),
		"result":       riverWorkerResultSucceeded,
		"trace_id":     "4bf92f3577b34da6a3ce929d0e0e4736",
		"span_id":      "00f067aa0ba902b7",
	}
	for key, value := range want {
		if fields[key] != value {
			t.Fatalf("field %s = %v, want %v", key, fields[key], value)
		}
	}
	if _, ok := fields["duration_ms"]; !ok {
		t.Fatalf("duration_ms field missing in %v", fields)
	}
}

func TestRiverWorkerLogMiddlewareClassifiesNonSuccessResults(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		want      string
		wantLevel zapcore.Level
	}{
		{name: "failed", err: errors.New("example namespace vm-1"), want: riverWorkerResultFailed, wantLevel: zap.WarnLevel},
		{name: "snoozed", err: river.JobSnooze(5 * time.Second), want: riverWorkerResultSnoozed, wantLevel: zap.WarnLevel},
		{name: "cancelled", err: river.JobCancel(errors.New("example ticket")), want: riverWorkerResultCancelled, wantLevel: zap.WarnLevel},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			core, observed := observer.New(zap.InfoLevel)
			middleware := NewRiverWorkerLogMiddleware(zap.New(core))

			err := middleware.Work(context.Background(), riverWorkerLogTestJob(t), func(context.Context) error {
				return tc.err
			})
			if !errors.Is(err, tc.err) {
				t.Fatalf("Work() error = %v, want original error %v", err, tc.err)
			}

			entries := observed.All()
			if len(entries) != 1 {
				t.Fatalf("log entries = %d, want 1", len(entries))
			}
			if entries[0].Level != tc.wantLevel {
				t.Fatalf("level = %v, want %v", entries[0].Level, tc.wantLevel)
			}
			fields := entries[0].ContextMap()
			requireAllowedRiverWorkerLogFields(t, fields)
			if fields["result"] != tc.want {
				t.Fatalf("result = %v, want %s", fields["result"], tc.want)
			}
			if fields["error_type"] == "" {
				t.Fatalf("error_type field missing in %v", fields)
			}
		})
	}
}

func TestRiverWorkerLogMiddlewareDoesNotLogRawErrorOrArgs(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	middleware := NewRiverWorkerLogMiddleware(zap.New(core))
	job := riverWorkerLogTestJob(t)
	job.EncodedArgs = []byte(`{"event_id":"ticket-alpha","namespace":"prod-alpha","vm_name":"vm-alpha"}`)

	rawErr := errors.New("failed namespace prod-alpha vm vm-alpha ticket ticket-alpha")
	err := middleware.Work(context.Background(), job, func(context.Context) error {
		return rawErr
	})
	if !errors.Is(err, rawErr) {
		t.Fatalf("Work() error = %v, want original error", err)
	}

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	requireAllowedRiverWorkerLogFields(t, fields)

	for _, forbidden := range []string{"prod-alpha", "vm-alpha", "ticket-alpha"} {
		for key, value := range fields {
			if value == forbidden {
				t.Fatalf("field %s leaked forbidden value %q in %v", key, forbidden, fields)
			}
		}
	}
	if _, ok := fields["error"]; ok {
		t.Fatalf("raw error field must not be logged: %v", fields)
	}
}

func riverWorkerLogTestJob(t *testing.T) *rivertype.JobRow {
	t.Helper()
	metadata, err := json.Marshal(map[string]riverTraceMetadata{
		riverTraceMetadataKey: {
			TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
			SpanID:  "00f067aa0ba902b7",
		},
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	return &rivertype.JobRow{
		ID:          42,
		Kind:        "vm_create",
		Queue:       "vm_operations",
		Attempt:     2,
		MaxAttempts: 5,
		Metadata:    metadata,
	}
}

func requireAllowedRiverWorkerLogFields(t *testing.T, fields map[string]interface{}) {
	t.Helper()
	allowed := map[string]struct{}{
		"job_id":       {},
		"job_kind":     {},
		"queue":        {},
		"attempt":      {},
		"max_attempts": {},
		"duration_ms":  {},
		"result":       {},
		"trace_id":     {},
		"span_id":      {},
		"error_type":   {},
	}
	for key := range fields {
		if _, ok := allowed[key]; !ok {
			t.Fatalf("unexpected River worker log field %q in %v", key, fields)
		}
	}
}
