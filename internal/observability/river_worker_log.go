package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	riverWorkerLogEvent   = "river_job_completed"
	riverTraceMetadataKey = "shepherd_trace"

	riverWorkerResultSucceeded = "succeeded"
	riverWorkerResultFailed    = "failed"
	riverWorkerResultSnoozed   = "snoozed"
	riverWorkerResultCancelled = "cancelled"
)

var (
	_ rivertype.JobInsertMiddleware = (*RiverWorkerLogMiddleware)(nil)
	_ rivertype.WorkerMiddleware    = (*RiverWorkerLogMiddleware)(nil)
)

// RiverWorkerLogMiddleware propagates trace IDs into River metadata and emits
// bounded worker completion logs.
type RiverWorkerLogMiddleware struct {
	river.MiddlewareDefaults

	logger *zap.Logger
}

type riverTraceMetadata struct {
	TraceID    string `json:"trace_id,omitempty"`
	SpanID     string `json:"span_id,omitempty"`
	TraceFlags string `json:"trace_flags,omitempty"`
}

// NewRiverWorkerLogMiddleware creates the global River worker log middleware.
func NewRiverWorkerLogMiddleware(log *zap.Logger) *RiverWorkerLogMiddleware {
	if log == nil {
		log = zap.NewNop()
	}
	return &RiverWorkerLogMiddleware{logger: log}
}

// InsertMany preserves existing River metadata and adds Shepherd trace context
// when a job is inserted under a valid OpenTelemetry span.
func (m *RiverWorkerLogMiddleware) InsertMany(
	ctx context.Context,
	manyParams []*rivertype.JobInsertParams,
	doInner func(context.Context) ([]*rivertype.JobInsertResult, error),
) ([]*rivertype.JobInsertResult, error) {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		for _, params := range manyParams {
			if params == nil {
				continue
			}
			params.Metadata = riverMetadataWithTrace(params.Metadata, spanContext)
		}
	}
	return doInner(ctx)
}

// Work emits one completion log after the worker returns. The worker error is
// intentionally returned unchanged so River retry/cancel/snooze semantics stay
// authoritative.
func (m *RiverWorkerLogMiddleware) Work(
	ctx context.Context,
	job *rivertype.JobRow,
	doInner func(context.Context) error,
) error {
	start := time.Now()
	traceID, spanID, traceFlags := riverTraceFromMetadata(job)
	if parent := riverSpanContext(traceID, spanID, traceFlags); parent.IsValid() {
		ctx = trace.ContextWithRemoteSpanContext(ctx, parent)
	}
	ctx, span := StartSpanWithOptions(ctx,
		riverWorkerSpanName(job),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "river"),
			attribute.String("messaging.operation.name", "process"),
			attribute.String("messaging.destination.name", riverJobQueue(job)),
			attribute.String("messaging.message.type", riverJobKind(job)),
			attribute.Int("messaging.message.receive_count", riverJobAttempt(job)),
		),
	)
	err := doInner(ctx)
	if err != nil {
		RecordSpanError(span, err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	spanContext := span.SpanContext()
	span.End()

	fields := riverWorkerLogFields(job)
	fields = append(fields,
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("result", riverWorkerResult(err)),
	)
	if spanContext.IsValid() {
		traceID = spanContext.TraceID().String()
		spanID = spanContext.SpanID().String()
	}
	if traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}
	if spanID != "" {
		fields = append(fields, zap.String("span_id", spanID))
	}
	if err != nil {
		fields = append(fields, zap.String("error_type", riverWorkerErrorType(err)))
		m.logger.Warn(riverWorkerLogEvent, fields...)
		return err
	}

	m.logger.Info(riverWorkerLogEvent, fields...)
	return nil
}

func riverWorkerLogFields(job *rivertype.JobRow) []zap.Field {
	if job == nil {
		return []zap.Field{
			zap.Int64("job_id", 0),
			zap.String("job_kind", ""),
			zap.String("queue", ""),
			zap.Int("attempt", 0),
			zap.Int("max_attempts", 0),
		}
	}
	return []zap.Field{
		zap.Int64("job_id", job.ID),
		zap.String("job_kind", job.Kind),
		zap.String("queue", job.Queue),
		zap.Int("attempt", job.Attempt),
		zap.Int("max_attempts", job.MaxAttempts),
	}
}

func riverMetadataWithTrace(metadata []byte, spanContext trace.SpanContext) []byte {
	if !spanContext.IsValid() {
		return metadata
	}

	envelope := map[string]json.RawMessage{}
	if trimmed := bytes.TrimSpace(metadata); len(trimmed) > 0 {
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return metadata
		}
		if envelope == nil {
			envelope = map[string]json.RawMessage{}
		}
	}

	tracePayload, err := json.Marshal(riverTraceMetadata{
		TraceID:    spanContext.TraceID().String(),
		SpanID:     spanContext.SpanID().String(),
		TraceFlags: spanContext.TraceFlags().String(),
	})
	if err != nil {
		return metadata
	}
	envelope[riverTraceMetadataKey] = tracePayload

	encoded, err := json.Marshal(envelope)
	if err != nil {
		return metadata
	}
	return encoded
}

func riverTraceFromMetadata(job *rivertype.JobRow) (traceID, spanID, traceFlags string) {
	if job == nil || len(bytes.TrimSpace(job.Metadata)) == 0 {
		return "", "", ""
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(job.Metadata, &envelope); err != nil {
		return "", "", ""
	}
	rawTrace, ok := envelope[riverTraceMetadataKey]
	if !ok {
		return "", "", ""
	}

	var metadata riverTraceMetadata
	if err := json.Unmarshal(rawTrace, &metadata); err != nil {
		return "", "", ""
	}
	return strings.TrimSpace(metadata.TraceID), strings.TrimSpace(metadata.SpanID), strings.TrimSpace(metadata.TraceFlags)
}

func riverSpanContext(traceIDHex, spanIDHex, traceFlagsHex string) trace.SpanContext {
	traceID, traceIDErr := trace.TraceIDFromHex(strings.TrimSpace(traceIDHex))
	spanID, spanIDErr := trace.SpanIDFromHex(strings.TrimSpace(spanIDHex))
	if traceIDErr != nil || spanIDErr != nil {
		return trace.SpanContext{}
	}
	traceFlags := trace.FlagsSampled
	if traceFlagsHex != "" {
		if parsed, err := strconv.ParseUint(traceFlagsHex, 16, 8); err == nil {
			traceFlags = trace.TraceFlags(byte(parsed))
		}
	}
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: traceFlags,
		Remote:     true,
	})
}

func riverWorkerSpanName(job *rivertype.JobRow) string {
	kind := riverJobKind(job)
	if kind == "" {
		kind = unknownMetricLabel
	}
	return "river job " + kind
}

func riverJobKind(job *rivertype.JobRow) string {
	if job == nil {
		return ""
	}
	return strings.TrimSpace(job.Kind)
}

func riverJobQueue(job *rivertype.JobRow) string {
	if job == nil {
		return ""
	}
	return strings.TrimSpace(job.Queue)
}

func riverJobAttempt(job *rivertype.JobRow) int {
	if job == nil {
		return 0
	}
	return job.Attempt
}

func riverWorkerResult(err error) string {
	if err == nil {
		return riverWorkerResultSucceeded
	}
	var snoozeErr *rivertype.JobSnoozeError
	if errors.As(err, &snoozeErr) {
		return riverWorkerResultSnoozed
	}
	var cancelErr *rivertype.JobCancelError
	if errors.As(err, &cancelErr) {
		return riverWorkerResultCancelled
	}
	return riverWorkerResultFailed
}

func riverWorkerErrorType(err error) string {
	if err == nil {
		return ""
	}
	errType := reflect.TypeOf(err)
	if errType == nil {
		return ""
	}
	return errType.String()
}
