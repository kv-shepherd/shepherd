package observability

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"

	"kv-shepherd.io/shepherd/internal/pkg/httpmethod"
)

const (
	TracingExporterOTLPHTTP = "otlp_http"
	TracingExporterStdout   = "stdout"

	tracingInstrumentationName = "kv-shepherd.io/shepherd/internal/observability"
)

// TracingOptions configures the default-off OpenTelemetry tracing baseline.
type TracingOptions struct {
	Enabled         bool
	ServiceName     string
	Exporter        string
	SampleRatio     float64
	ShutdownTimeout time.Duration
}

// Tracing owns the OpenTelemetry tracer provider for application lifetime.
type Tracing struct {
	serviceName     string
	provider        *sdktrace.TracerProvider
	tracer          oteltrace.Tracer
	shutdownTimeout time.Duration
}

// NewTracing initializes OpenTelemetry tracing when enabled.
func NewTracing(ctx context.Context, options TracingOptions) (*Tracing, error) {
	if !options.Enabled {
		return nil, nil
	}
	if options.ServiceName == "" {
		return nil, fmt.Errorf("tracing service name must not be empty")
	}
	if options.SampleRatio < 0 || options.SampleRatio > 1 {
		return nil, fmt.Errorf("tracing sample ratio must be between 0.0 and 1.0")
	}

	exporter, err := newSpanExporter(ctx, options.Exporter)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.name", options.ServiceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("create tracing resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(options.SampleRatio))),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracing{
		serviceName:     options.ServiceName,
		provider:        provider,
		tracer:          provider.Tracer(tracingInstrumentationName),
		shutdownTimeout: options.ShutdownTimeout,
	}, nil
}

func newSpanExporter(ctx context.Context, exporter string) (sdktrace.SpanExporter, error) {
	switch exporter {
	case TracingExporterOTLPHTTP:
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create otlp http trace exporter: %w", err)
		}
		return exp, nil
	case TracingExporterStdout:
		exp, err := stdouttrace.New()
		if err != nil {
			return nil, fmt.Errorf("create stdout trace exporter: %w", err)
		}
		return exp, nil
	default:
		return nil, fmt.Errorf("unsupported tracing exporter %q", exporter)
	}
}

// Middleware returns Gin HTTP ingress tracing middleware.
func (t *Tracing) Middleware() gin.HandlerFunc {
	if t == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	tracer := t.tracer
	if tracer == nil {
		tracer = otel.Tracer(tracingInstrumentationName)
	}
	return func(c *gin.Context) {
		if c == nil || c.Request == nil {
			c.Next()
			return
		}

		originalContext := c.Request.Context()
		ctx := otel.GetTextMapPropagator().Extract(originalContext, propagation.HeaderCarrier(c.Request.Header))
		method := httpmethod.NormalizeForObservability(c.Request.Method)
		ctx, span := tracer.Start(
			ctx,
			traceSpanName(method, ""),
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithAttributes(attribute.String("http.request.method", method)),
		)
		defer span.End()
		defer func() {
			c.Request = c.Request.WithContext(originalContext)
		}()
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		route := normalizedTraceRoute(c)
		span.SetName(traceSpanName(method, route))
		span.SetAttributes(
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", c.Writer.Status()),
		)
		if c.Writer.Status() >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, "")
		}
	}
}

// Shutdown flushes and shuts down the tracer provider.
func (t *Tracing) Shutdown(ctx context.Context) error {
	if t == nil || t.provider == nil {
		return nil
	}
	if t.shutdownTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.shutdownTimeout)
		defer cancel()
	}
	if err := t.provider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown tracing provider: %w", err)
	}
	return nil
}

func normalizedTraceRoute(c *gin.Context) string {
	if c == nil {
		return unmatchedRoute
	}
	route := strings.TrimSpace(c.FullPath())
	if route == "" {
		return unmatchedRoute
	}
	return route
}

func traceSpanName(method, route string) string {
	method = httpmethod.NormalizeForObservability(method)
	route = strings.TrimSpace(route)
	if route == "" {
		return method
	}
	return method + " " + route
}
