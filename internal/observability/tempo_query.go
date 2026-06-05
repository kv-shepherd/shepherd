package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	DefaultTraceQueryURL      = "http://tempo:3200"
	DefaultTraceQueryTimeout  = 3 * time.Second
	DefaultTraceQueryLimit    = 100
	DefaultTraceQueryLookback = time.Hour

	traceSourceTempo           = "tempo"
	tempoTraceDetailConcurrent = 8
)

// TraceSummaryFilter limits the trace summary query.
type TraceSummaryFilter struct {
	Lookback time.Duration
	Limit    int
	Route    string
}

// TraceSummaryProvider provides administrator-facing trace aggregates.
type TraceSummaryProvider interface {
	TraceSummary(context.Context, TraceSummaryFilter) (TraceSummary, error)
}

// TraceSummary is a compact, UI-ready summary of recent traces.
type TraceSummary struct {
	GeneratedAt   time.Time
	Source        string
	Status        string
	WindowSeconds int64
	Endpoints     []TraceEndpointSummary
	SlowTraces    []TraceSample
	Dependencies  []TraceSpanGroupSummary
}

// TraceEndpointSummary aggregates HTTP server spans by normalized route.
type TraceEndpointSummary struct {
	Route          string
	RequestCount   int
	ErrorCount     int
	ErrorRate      float64
	P95Ms          float64
	AvgMs          float64
	MaxMs          float64
	SlowestTraceID string
}

// TraceSample identifies one slow or failed trace sample.
type TraceSample struct {
	TraceID    string
	RootName   string
	Route      string
	DurationMs float64
	StatusCode int
	Error      bool
	StartedAt  time.Time
}

// TraceSpanGroupSummary aggregates non-ingress spans by category and name.
type TraceSpanGroupSummary struct {
	Category   string
	Name       string
	SpanCount  int
	ErrorCount int
	P95Ms      float64
	MaxMs      float64
}

// TempoTraceQueryOptions configures the Tempo query client.
type TempoTraceQueryOptions struct {
	BaseURL     string
	ServiceName string
	Timeout     time.Duration
	Limit       int
	Lookback    time.Duration
	HTTPClient  *http.Client
	Now         func() time.Time
}

// TempoTraceQueryClient queries Tempo's HTTP API and returns Shepherd trace summaries.
type TempoTraceQueryClient struct {
	baseURL     *url.URL
	serviceName string
	timeout     time.Duration
	limit       int
	lookback    time.Duration
	httpClient  *http.Client
	now         func() time.Time
}

// NewTempoTraceQueryClient creates a Tempo-backed trace summary provider.
func NewTempoTraceQueryClient(options TempoTraceQueryOptions) (*TempoTraceQueryClient, error) {
	rawURL := strings.TrimSpace(options.BaseURL)
	if rawURL == "" {
		rawURL = DefaultTraceQueryURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse tempo trace query url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("tempo trace query url must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return nil, fmt.Errorf("tempo trace query url must include a host")
	}

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultTraceQueryTimeout
	}
	limit := options.Limit
	if limit <= 0 {
		limit = DefaultTraceQueryLimit
	}
	lookback := options.Lookback
	if lookback <= 0 {
		lookback = DefaultTraceQueryLookback
	}
	serviceName := strings.TrimSpace(options.ServiceName)
	if serviceName == "" {
		serviceName = "shepherd"
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &TempoTraceQueryClient{
		baseURL:     parsed,
		serviceName: serviceName,
		timeout:     timeout,
		limit:       limit,
		lookback:    lookback,
		httpClient:  httpClient,
		now:         now,
	}, nil
}

// TraceSummary returns recent route and dependency aggregates from Tempo.
func (c *TempoTraceQueryClient) TraceSummary(ctx context.Context, filter TraceSummaryFilter) (TraceSummary, error) {
	if c == nil {
		return TraceSummary{}, fmt.Errorf("tempo trace query client is nil")
	}
	if c.httpClient == nil {
		return TraceSummary{}, fmt.Errorf("tempo trace query http client is nil")
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	lookback := filter.Lookback
	if lookback <= 0 {
		lookback = c.lookback
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = c.limit
	}
	routeFilter := strings.TrimSpace(filter.Route)
	now := c.now().UTC()
	start := now.Add(-lookback)
	end := now

	traces, err := c.searchTraces(ctx, start, end, limit, routeFilter)
	if err != nil {
		return TraceSummary{}, err
	}

	acc := newTraceAccumulator(routeFilter)
	details, err := c.getTraceDetails(ctx, traces, start, end)
	if err != nil {
		return TraceSummary{}, err
	}
	for i := range details {
		item := &details[i]
		if item.detail == nil {
			continue
		}
		acc.addTrace(item.search, item.detail)
	}

	return TraceSummary{
		GeneratedAt:   now,
		Source:        traceSourceTempo,
		Status:        "ok",
		WindowSeconds: int64(lookback.Seconds()),
		Endpoints:     acc.endpointSummaries(),
		SlowTraces:    acc.slowTraceSamples(),
		Dependencies:  acc.dependencySummaries(),
	}, nil
}

type tempoSearchResponse struct {
	Traces []tempoSearchTrace `json:"traces"`
}

type tempoSearchTrace struct {
	TraceID            string  `json:"traceID"`
	RootServiceName    string  `json:"rootServiceName"`
	RootTraceName      string  `json:"rootTraceName"`
	StartTimeUnixNano  string  `json:"startTimeUnixNano"`
	DurationMs         float64 `json:"durationMs"`
	ServiceStats       any     `json:"serviceStats"`
	SpanSet            any     `json:"spanSet"`
	SpanSets           any     `json:"spanSets"`
	MatchedSpanCount   int     `json:"matchedSpanCount"`
	MatchedSpanSetSize int     `json:"matchedSpanSetSize"`
}

func (c *TempoTraceQueryClient) searchTraces(ctx context.Context, start, end time.Time, limit int, routeFilter string) ([]tempoSearchTrace, error) {
	searchURL := c.urlFor("/api/search")
	query := searchURL.Query()
	query.Set("tags", tempoSearchTags(c.serviceName, routeFilter))
	query.Set("limit", strconv.Itoa(limit))
	query.Set("start", strconv.FormatInt(start.Unix(), 10))
	query.Set("end", strconv.FormatInt(end.Unix(), 10))
	searchURL.RawQuery = query.Encode()

	var resp tempoSearchResponse
	if err := c.getJSON(ctx, searchURL.String(), &resp); err != nil {
		return nil, fmt.Errorf("search tempo traces: %w", err)
	}
	return resp.Traces, nil
}

type tempoTraceDetail struct {
	search tempoSearchTrace
	detail any
}

func (c *TempoTraceQueryClient) getTraceDetails(ctx context.Context, traces []tempoSearchTrace, start, end time.Time) ([]tempoTraceDetail, error) {
	details := make([]tempoTraceDetail, len(traces))
	group, groupCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, tempoTraceDetailConcurrent)

	for i := range traces {
		i := i
		traceID := strings.TrimSpace(traces[i].TraceID)
		if traceID == "" {
			continue
		}
		group.Go(func() error {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-groupCtx.Done():
				return groupCtx.Err()
			}

			detail, err := c.getTrace(groupCtx, traceID, start, end)
			if err != nil {
				return err
			}
			details[i] = tempoTraceDetail{
				search: traces[i],
				detail: detail,
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}
	return details, nil
}

func (c *TempoTraceQueryClient) getTrace(ctx context.Context, traceID string, start, end time.Time) (any, error) {
	traceURL := c.urlFor("/api/traces/" + url.PathEscape(traceID))
	query := traceURL.Query()
	if !start.IsZero() {
		query.Set("start", strconv.FormatInt(start.Unix(), 10))
	}
	if !end.IsZero() {
		query.Set("end", strconv.FormatInt(end.Unix(), 10))
	}
	traceURL.RawQuery = query.Encode()

	var payload any
	if err := c.getJSON(ctx, traceURL.String(), &payload); err != nil {
		return nil, fmt.Errorf("get tempo trace %s: %w", traceID, err)
	}
	return payload, nil
}

func (c *TempoTraceQueryClient) getJSON(ctx context.Context, rawURL string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return err
	}
	// #nosec G704 -- rawURL is built from the operator-configured Tempo base URL
	// and fixed API paths; user input is only used after path/query escaping.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("tempo returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return err
	}
	return nil
}

func (c *TempoTraceQueryClient) urlFor(path string) *url.URL {
	next := *c.baseURL
	basePath := strings.TrimRight(next.Path, "/")
	next.Path = basePath + path
	return &next
}

func tempoSearchTags(serviceName, routeFilter string) string {
	tags := []tempoSearchTag{{key: "service.name", value: serviceName}}
	method, route := splitTempoRouteFilter(routeFilter)
	if method != "" {
		tags = append(tags, tempoSearchTag{key: "http.request.method", value: method})
	}
	if route != "" {
		tags = append(tags, tempoSearchTag{key: "http.route", value: route})
	}

	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		if strings.TrimSpace(tag.value) == "" {
			continue
		}
		parts = append(parts, tag.key+"="+logfmtValue(tag.value))
	}
	return strings.Join(parts, " ")
}

type tempoSearchTag struct {
	key   string
	value string
}

func splitTempoRouteFilter(routeFilter string) (method, route string) {
	routeFilter = strings.TrimSpace(routeFilter)
	if routeFilter == "" {
		return "", ""
	}
	parts := strings.Fields(routeFilter)
	if len(parts) >= 2 {
		candidateMethod := strings.ToUpper(parts[0])
		if isHTTPMethodToken(candidateMethod) {
			return candidateMethod, strings.Join(parts[1:], " ")
		}
	}
	return "", routeFilter
}

func isHTTPMethodToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func logfmtValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\r\n\"=") {
		return strconv.Quote(value)
	}
	return value
}

type traceAccumulator struct {
	routeFilter  string
	endpoints    map[string]*endpointAccumulator
	dependencies map[string]*spanGroupAccumulator
	slowTraces   []TraceSample
}

type endpointAccumulator struct {
	route          string
	count          int
	errorCount     int
	sumMs          float64
	maxMs          float64
	slowestTraceID string
	durationsMs    []float64
}

type spanGroupAccumulator struct {
	category    string
	name        string
	count       int
	errorCount  int
	maxMs       float64
	durationsMs []float64
}

type parsedTraceSpan struct {
	Name       string
	Kind       string
	Attrs      map[string]string
	StatusCode string
	StartNanos int64
	EndNanos   int64
}

func newTraceAccumulator(routeFilter string) *traceAccumulator {
	return &traceAccumulator{
		routeFilter:  strings.TrimSpace(routeFilter),
		endpoints:    make(map[string]*endpointAccumulator),
		dependencies: make(map[string]*spanGroupAccumulator),
	}
}

func (a *traceAccumulator) addTrace(search tempoSearchTrace, detail any) {
	traceID := strings.TrimSpace(search.TraceID)
	rootName := strings.TrimSpace(search.RootTraceName)
	startedAt := timeFromUnixNano(parseInt64(search.StartTimeUnixNano))

	spans := parseTraceSpans(detail)
	for _, span := range spans {
		durationMs := span.durationMs()
		if durationMs <= 0 {
			continue
		}
		if startedAt.IsZero() && span.StartNanos > 0 {
			startedAt = timeFromUnixNano(span.StartNanos)
		}
		if isServerSpan(span.Kind) {
			route := routeName(span)
			if a.routeFilter == "" && isOperationalTraceRoute(route) {
				continue
			}
			if !a.acceptRoute(route) {
				continue
			}
			statusCode := httpStatusCode(span.Attrs)
			isError := spanHasError(span, statusCode)
			a.addEndpoint(route, traceID, durationMs, isError)
			a.slowTraces = append(a.slowTraces, TraceSample{
				TraceID:    traceID,
				RootName:   firstNonEmpty(rootName, route),
				Route:      route,
				DurationMs: roundMillis(durationMs),
				StatusCode: statusCode,
				Error:      isError,
				StartedAt:  startedAt,
			})
			continue
		}
		category := spanCategory(span)
		name := strings.TrimSpace(span.Name)
		if name == "" {
			name = unknownMetricLabel
		}
		a.addDependency(category, name, durationMs, spanHasError(span, 0))
	}
}

func (a *traceAccumulator) acceptRoute(route string) bool {
	filter := strings.TrimSpace(a.routeFilter)
	if filter == "" {
		return true
	}
	route = strings.TrimSpace(route)
	if strings.EqualFold(route, filter) {
		return true
	}
	filterMethod, filterPath := splitTempoRouteFilter(filter)
	routeMethod, routePath := splitTempoRouteFilter(route)
	if filterMethod != "" && !strings.EqualFold(filterMethod, routeMethod) {
		return false
	}
	if filterPath != "" && routePath != "" {
		return strings.EqualFold(filterPath, routePath)
	}
	return false
}

func (a *traceAccumulator) addEndpoint(route, traceID string, durationMs float64, isError bool) {
	route = strings.TrimSpace(route)
	if route == "" {
		route = unknownMetricLabel
	}
	acc := a.endpoints[route]
	if acc == nil {
		acc = &endpointAccumulator{route: route}
		a.endpoints[route] = acc
	}
	acc.count++
	if isError {
		acc.errorCount++
	}
	acc.sumMs += durationMs
	acc.durationsMs = append(acc.durationsMs, durationMs)
	if durationMs > acc.maxMs {
		acc.maxMs = durationMs
		acc.slowestTraceID = traceID
	}
}

func (a *traceAccumulator) addDependency(category, name string, durationMs float64, isError bool) {
	category = strings.TrimSpace(category)
	if category == "" {
		category = "internal"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = unknownMetricLabel
	}
	key := category + "\x00" + name
	acc := a.dependencies[key]
	if acc == nil {
		acc = &spanGroupAccumulator{category: category, name: name}
		a.dependencies[key] = acc
	}
	acc.count++
	if isError {
		acc.errorCount++
	}
	acc.durationsMs = append(acc.durationsMs, durationMs)
	if durationMs > acc.maxMs {
		acc.maxMs = durationMs
	}
}

func (a *traceAccumulator) endpointSummaries() []TraceEndpointSummary {
	items := make([]TraceEndpointSummary, 0, len(a.endpoints))
	for _, acc := range a.endpoints {
		if acc.count == 0 {
			continue
		}
		items = append(items, TraceEndpointSummary{
			Route:          acc.route,
			RequestCount:   acc.count,
			ErrorCount:     acc.errorCount,
			ErrorRate:      roundRatio(float64(acc.errorCount) / float64(acc.count)),
			P95Ms:          roundMillis(percentile(acc.durationsMs, 0.95)),
			AvgMs:          roundMillis(acc.sumMs / float64(acc.count)),
			MaxMs:          roundMillis(acc.maxMs),
			SlowestTraceID: acc.slowestTraceID,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ErrorRate != items[j].ErrorRate {
			return items[i].ErrorRate > items[j].ErrorRate
		}
		if items[i].P95Ms != items[j].P95Ms {
			return items[i].P95Ms > items[j].P95Ms
		}
		return items[i].Route < items[j].Route
	})
	return items
}

func (a *traceAccumulator) slowTraceSamples() []TraceSample {
	items := append([]TraceSample(nil), a.slowTraces...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Error != items[j].Error {
			return items[i].Error
		}
		if items[i].DurationMs != items[j].DurationMs {
			return items[i].DurationMs > items[j].DurationMs
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	if len(items) > 20 {
		items = items[:20]
	}
	return items
}

func (a *traceAccumulator) dependencySummaries() []TraceSpanGroupSummary {
	items := make([]TraceSpanGroupSummary, 0, len(a.dependencies))
	for _, acc := range a.dependencies {
		if acc.count == 0 {
			continue
		}
		items = append(items, TraceSpanGroupSummary{
			Category:   acc.category,
			Name:       acc.name,
			SpanCount:  acc.count,
			ErrorCount: acc.errorCount,
			P95Ms:      roundMillis(percentile(acc.durationsMs, 0.95)),
			MaxMs:      roundMillis(acc.maxMs),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ErrorCount != items[j].ErrorCount {
			return items[i].ErrorCount > items[j].ErrorCount
		}
		if items[i].P95Ms != items[j].P95Ms {
			return items[i].P95Ms > items[j].P95Ms
		}
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].Name < items[j].Name
	})
	if len(items) > 50 {
		items = items[:50]
	}
	return items
}

func parseTraceSpans(payload any) []parsedTraceSpan {
	var spans []parsedTraceSpan
	for _, resourceSpan := range collectResourceSpans(payload) {
		resourceObj := objectValue(resourceSpan)
		scopeSpans := arrayField(resourceObj, "scopeSpans")
		scopeSpans = append(scopeSpans, arrayField(resourceObj, "instrumentationLibrarySpans")...)
		for _, scopeSpan := range scopeSpans {
			scopeObj := objectValue(scopeSpan)
			for _, rawSpan := range arrayField(scopeObj, "spans") {
				if span := parseSpan(objectValue(rawSpan)); span.Name != "" || span.Kind != "" {
					spans = append(spans, span)
				}
			}
		}
	}
	return spans
}

func collectResourceSpans(payload any) []any {
	switch value := payload.(type) {
	case []any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, collectResourceSpans(item)...)
		}
		return items
	case map[string]any:
		if resourceSpans := arrayField(value, "resourceSpans"); len(resourceSpans) > 0 {
			return resourceSpans
		}
		if batches := arrayField(value, "batches"); len(batches) > 0 {
			return batches
		}
	}
	return nil
}

func parseSpan(raw map[string]any) parsedTraceSpan {
	status := objectField(raw, "status")
	return parsedTraceSpan{
		Name:       stringField(raw, "name"),
		Kind:       strings.ToUpper(stringValue(raw["kind"])),
		Attrs:      attributeMap(arrayField(raw, "attributes")),
		StatusCode: strings.ToUpper(stringValue(status["code"])),
		StartNanos: parseInt64(raw["startTimeUnixNano"]),
		EndNanos:   parseInt64(raw["endTimeUnixNano"]),
	}
}

func (s parsedTraceSpan) durationMs() float64 {
	if s.EndNanos <= s.StartNanos || s.StartNanos <= 0 {
		return 0
	}
	return float64(s.EndNanos-s.StartNanos) / 1_000_000
}

func attributeMap(attributes []any) map[string]string {
	result := make(map[string]string, len(attributes))
	for _, raw := range attributes {
		obj := objectValue(raw)
		key := strings.TrimSpace(stringField(obj, "key"))
		if key == "" {
			continue
		}
		result[key] = attributeValueString(obj["value"])
	}
	return result
}

func attributeValueString(raw any) string {
	switch value := raw.(type) {
	case map[string]any:
		for _, key := range []string{
			"stringValue", "intValue", "doubleValue", "boolValue",
			"string_value", "int_value", "double_value", "bool_value",
		} {
			if rawValue, ok := value[key]; ok {
				return stringValue(rawValue)
			}
		}
		return ""
	default:
		return stringValue(value)
	}
}

func isServerSpan(kind string) bool {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	return kind == "SPAN_KIND_SERVER" || kind == "SERVER" || kind == "2"
}

func spanCategory(span parsedTraceSpan) string {
	name := strings.ToLower(strings.TrimSpace(span.Name))
	if strings.HasPrefix(name, "business.") {
		return "business"
	}
	if strings.HasPrefix(name, "postgresql") || strings.EqualFold(span.Attrs["db.system.name"], "postgresql") {
		return "database"
	}
	if strings.HasPrefix(name, "kubernetes.") ||
		strings.HasPrefix(name, "provider.kubevirt.") ||
		strings.EqualFold(span.Attrs["shepherd.provider"], "kubevirt") ||
		strings.EqualFold(span.Attrs["rpc.system"], "kubernetes") {
		return "kubevirt"
	}
	kind := strings.ToUpper(strings.TrimSpace(span.Kind))
	if strings.Contains(name, "river") || kind == "SPAN_KIND_CONSUMER" || kind == "CONSUMER" || kind == "5" {
		return "worker"
	}
	if kind == "SPAN_KIND_CLIENT" || kind == "CLIENT" || kind == "3" {
		return "provider"
	}
	return "internal"
}

func routeName(span parsedTraceSpan) string {
	method := firstNonEmpty(
		strings.ToUpper(strings.TrimSpace(span.Attrs["http.request.method"])),
		strings.ToUpper(strings.TrimSpace(span.Attrs["http.method"])),
	)
	route := firstNonEmpty(
		strings.TrimSpace(span.Attrs["http.route"]),
		strings.TrimSpace(span.Name),
	)
	if method != "" && strings.HasPrefix(route, "/") {
		return method + " " + route
	}
	return route
}

func isOperationalTraceRoute(route string) bool {
	route = strings.TrimSpace(route)
	if route == "" {
		return true
	}
	parts := strings.Fields(route)
	path := route
	if len(parts) > 1 {
		path = parts[len(parts)-1]
	}
	switch path {
	case "/metrics", "/healthz", "/readyz":
		return true
	}
	return strings.Contains(path, "/health/live") || strings.Contains(path, "/health/ready")
}

func httpStatusCode(attrs map[string]string) int {
	for _, key := range []string{"http.response.status_code", "http.status_code"} {
		if value, ok := attrs[key]; ok {
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func spanHasError(span parsedTraceSpan, statusCode int) bool {
	if statusCode >= http.StatusInternalServerError {
		return true
	}
	code := strings.ToUpper(strings.TrimSpace(span.StatusCode))
	return code == "STATUS_CODE_ERROR" || code == "ERROR" || code == "2"
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func roundMillis(value float64) float64 {
	return math.Round(value*100) / 100
}

func roundRatio(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func timeFromUnixNano(nanos int64) time.Time {
	if nanos <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
}

func arrayField(obj map[string]any, key string) []any {
	if obj == nil {
		return nil
	}
	items, ok := obj[key].([]any)
	if !ok {
		return nil
	}
	return items
}

func objectField(obj map[string]any, key string) map[string]any {
	if obj == nil {
		return nil
	}
	return objectValue(obj[key])
}

func objectValue(raw any) map[string]any {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return obj
}

func stringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	return stringValue(obj[key])
}

func stringValue(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		if value {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func parseInt64(raw any) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
