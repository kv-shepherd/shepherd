package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
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
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
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
	err := doInner(ctx)

	fields := riverWorkerLogFields(job)
	fields = append(fields,
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("result", riverWorkerResult(err)),
	)
	traceID, spanID := riverTraceFromMetadata(job)
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
		TraceID: spanContext.TraceID().String(),
		SpanID:  spanContext.SpanID().String(),
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

func riverTraceFromMetadata(job *rivertype.JobRow) (traceID, spanID string) {
	if job == nil || len(bytes.TrimSpace(job.Metadata)) == 0 {
		return "", ""
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(job.Metadata, &envelope); err != nil {
		return "", ""
	}
	rawTrace, ok := envelope[riverTraceMetadataKey]
	if !ok {
		return "", ""
	}

	var metadata riverTraceMetadata
	if err := json.Unmarshal(rawTrace, &metadata); err != nil {
		return "", ""
	}
	return strings.TrimSpace(metadata.TraceID), strings.TrimSpace(metadata.SpanID)
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
