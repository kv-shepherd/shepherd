package observability

import (
	"context"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	dbSystemPostgreSQL = "postgresql"
)

type pgxSpanKey struct{}

// PGXTracer creates low-cardinality DB client spans for pgx, Ent, River, and sqlc.
type PGXTracer struct{}

var _ pgx.QueryTracer = (*PGXTracer)(nil)

// NewPGXTracer returns a pgx tracer that intentionally avoids raw SQL and args.
func NewPGXTracer() *PGXTracer {
	return &PGXTracer{}
}

func (t *PGXTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	operation, collection := summarizeSQL(data.SQL)
	attrs := []attribute.KeyValue{
		attribute.String("db.system.name", dbSystemPostgreSQL),
		attribute.String("db.operation.name", operation),
	}
	if collection != "" {
		attrs = append(attrs, attribute.String("db.collection.name", collection))
	}
	if conn != nil && conn.Config() != nil {
		cfg := conn.Config()
		if cfg.Database != "" {
			attrs = append(attrs, attribute.String("db.namespace", cfg.Database))
		}
		if cfg.Host != "" {
			attrs = append(attrs, attribute.String("server.address", cfg.Host))
		}
		if cfg.Port > 0 {
			attrs = append(attrs, attribute.Int("server.port", int(cfg.Port)))
		}
	}

	ctx, span := StartSpanWithOptions(
		ctx,
		dbSpanName(operation, collection),
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(attrs...),
	)
	return context.WithValue(ctx, pgxSpanKey{}, span)
}

func (t *PGXTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(pgxSpanKey{}).(oteltrace.Span)
	if !ok || span == nil {
		return
	}
	if data.CommandTag.String() != "" {
		span.SetAttributes(attribute.String("db.response.status_code", data.CommandTag.String()))
	}
	RecordSpanError(span, data.Err)
	span.End()
}

func dbSpanName(operation, collection string) string {
	operation = strings.TrimSpace(strings.ToUpper(operation))
	collection = sanitizeSQLIdentifier(collection)
	if operation == "" {
		operation = "SQL"
	}
	if collection == "" {
		return "postgresql " + operation
	}
	return "postgresql " + operation + " " + collection
}

func summarizeSQL(sql string) (operation, collection string) {
	tokens := sqlTokens(sql)
	if len(tokens) == 0 {
		return "SQL", ""
	}
	operation = strings.ToUpper(tokens[0])
	switch operation {
	case "SELECT":
		collection = tokenAfter(tokens, "FROM")
	case "INSERT":
		collection = tokenAfter(tokens, "INTO")
	case "UPDATE":
		if len(tokens) > 1 {
			collection = tokens[1]
		}
	case "DELETE":
		collection = tokenAfter(tokens, "FROM")
	case "WITH":
		operation = "WITH"
		collection = tokenAfter(tokens, "FROM")
	default:
		collection = ""
	}
	return operation, sanitizeSQLIdentifier(collection)
}

func tokenAfter(tokens []string, marker string) string {
	marker = strings.ToUpper(marker)
	for i := 0; i < len(tokens)-1; i++ {
		if strings.EqualFold(tokens[i], marker) {
			return tokens[i+1]
		}
	}
	return ""
}

func sqlTokens(sql string) []string {
	fields := strings.FieldsFunc(sql, func(r rune) bool {
		return unicode.IsSpace(r) || r == '(' || r == ')' || r == ',' || r == ';'
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || strings.HasPrefix(field, "--") {
			continue
		}
		tokens = append(tokens, field)
	}
	return tokens
}

func sanitizeSQLIdentifier(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ""
	}
	identifier = strings.Trim(identifier, `"`)
	if dot := strings.LastIndex(identifier, "."); dot >= 0 && dot < len(identifier)-1 {
		identifier = identifier[dot+1:]
	}
	identifier = strings.Trim(identifier, `"`)

	var b strings.Builder
	for _, r := range identifier {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_':
			b.WriteRune(r)
		default:
			return ""
		}
		if b.Len() > 80 {
			return b.String()
		}
	}
	return b.String()
}
