package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	libopenapi "github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	validatorcache "github.com/pb33f/libopenapi-validator/cache"
	validatorconfig "github.com/pb33f/libopenapi-validator/config"
	validatorerrors "github.com/pb33f/libopenapi-validator/errors"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/specembed"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const openAPIResponseValidationMessage = "response does not conform to OpenAPI contract"
const openAPIRequestValidationMessage = "request does not conform to OpenAPI contract"

// MustOpenAPIValidator creates an OpenAPI runtime validator middleware and panics on setup failure.
func MustOpenAPIValidator(basePath string) gin.HandlerFunc {
	mw, err := NewOpenAPIValidator(basePath)
	if err != nil {
		panic(fmt.Sprintf("init openapi validator: %v", err))
	}
	return mw
}

// NewOpenAPIValidator validates request + response against the canonical OpenAPI spec.
func NewOpenAPIValidator(basePath string) (gin.HandlerFunc, error) {
	document, err := libopenapi.NewDocument(specembed.CanonicalSpec)
	if err != nil {
		return nil, fmt.Errorf("load canonical openapi document: %w", err)
	}
	if _, err := document.BuildV3Model(); err != nil {
		return nil, fmt.Errorf("build canonical openapi model: %w", err)
	}

	runtime := &openAPIRuntimeValidator{
		document:              document,
		basePath:              normalizeBasePath(basePath),
		validateResponse:      gin.Mode() != gin.ReleaseMode,
		exposeValidationError: gin.Mode() != gin.ReleaseMode,
		schemaCache:           validatorcache.NewDefaultCache(),
	}

	return runtime.middleware, nil
}

type openAPIRuntimeValidator struct {
	document              libopenapi.Document
	basePath              string
	validateResponse      bool
	exposeValidationError bool
	schemaCache           validatorcache.SchemaCache
}

func (v *openAPIRuntimeValidator) middleware(c *gin.Context) {
	if websocket.IsWebSocketUpgrade(c.Request) {
		// OpenAPI validation remains strict for the HTTP bootstrap request,
		// but the actual websocket upgrade includes handshake headers and a
		// 101 response that are outside the REST contract.
		c.Next()
		return
	}

	requestIgnorePaths := requestStrictIgnorePaths(c.Request, v.basePath)
	requestValidator, err := v.newValidator(requestIgnorePaths...)
	if err != nil {
		logger.Error("OpenAPI validator setup failed",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Error(err),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":    "OPENAPI_VALIDATOR_UNAVAILABLE",
			"message": "OpenAPI validator could not be initialized",
		})
		return
	}

	restorePath := applyValidationPath(c.Request, v.basePath)
	requestValid, requestErrs := requestValidator.ValidateHttpRequest(c.Request)
	restorePath()

	if !requestValid {
		if allPathMissing(requestErrs) {
			// Route is outside the OpenAPI contract scope.
			c.Next()
			return
		}
		code := "OPENAPI_REQUEST_INVALID"
		if hasRouteValidationError(requestErrs) {
			code = "OPENAPI_ROUTE_INVALID"
		}

		message := summarizeValidationErrors(requestErrs)
		if !v.exposeValidationError {
			message = openAPIRequestValidationMessage
		}

		abortWithOpenAPIError(c, http.StatusBadRequest, code, message)
		return
	}

	buffered := newBufferedResponseWriter(c.Writer)
	c.Writer = buffered
	c.Next()

	// Client disconnected or request context timed out. Avoid validating or
	// rewriting a response that the caller has already abandoned.
	if c.Request != nil && c.Request.Context().Err() != nil {
		return
	}

	if v.validateResponse {
		// Keep strict validation globally, but explicitly allow free-form object
		// subtrees declared by the OpenAPI contract on selected endpoints.
		responseIgnorePaths := responseStrictIgnorePaths(c.Request, v.basePath)

		responseValidator, err := v.newValidator(responseIgnorePaths...)
		if err != nil {
			logger.Error("OpenAPI validator setup failed for response validation",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Error(err),
			)
			buffered.ResetJSON(http.StatusInternalServerError, map[string]string{
				"code":    "OPENAPI_VALIDATOR_UNAVAILABLE",
				"message": "OpenAPI validator could not be initialized",
			})
		} else {
			restorePath = applyValidationPath(c.Request, v.basePath)
			response := buildValidationResponse(c.Request, buffered)
			responseValid, responseErrs := responseValidator.ValidateHttpResponse(c.Request, response)
			if response.Body != nil {
				_ = response.Body.Close()
			}
			restorePath()
			if !responseValid && !allPathMissing(responseErrs) {
				logger.Error("OpenAPI response validation failed",
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.Int("status", buffered.Status()),
					zap.String("reason", summarizeValidationErrors(responseErrs)),
				)
				buffered.ResetJSON(http.StatusInternalServerError, map[string]string{
					"code":    "OPENAPI_RESPONSE_INVALID",
					"message": openAPIResponseValidationMessage,
				})
			}
		}
	}

	if _, err := buffered.FlushToOriginal(); err != nil {
		logger.Warn("failed to flush buffered response",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Error(err),
		)
	}
}

func requestStrictIgnorePaths(request *http.Request, basePath string) []string {
	if request == nil || request.URL == nil {
		return nil
	}

	path := normalizeValidationPath(basePath, request.URL.Path)

	// AuthProvider.config is contractually free-form JSON for plugin-specific
	// settings. Keep strict validation for the rest of the request body.
	switch request.Method {
	case http.MethodPost:
		if path == "/admin/auth-providers" {
			return []string{"$.body.config.**"}
		}
		if strings.HasPrefix(path, "/admin/auth-providers/") &&
			(strings.HasSuffix(path, "/directory/preview") ||
				strings.HasSuffix(path, "/directory/sync")) {
			return []string{"$.body.provider_request.**"}
		}
		if path == "/admin/instance-sizes" {
			return []string{"$.body.spec_overrides.**"}
		}
	case http.MethodPatch:
		if strings.HasPrefix(path, "/admin/auth-providers/") && !strings.Contains(path, "/cohort-mappings") {
			return []string{"$.body.config.**"}
		}
		if strings.HasPrefix(path, "/admin/instance-sizes/") {
			return []string{"$.body.spec_overrides.**"}
		}
	case http.MethodPut:
		if strings.HasPrefix(path, "/auth/preferences/") {
			return []string{"$.body.value.**"}
		}
	}

	return nil
}

func shouldIgnoreDynamicSchemaResponseBody(request *http.Request, basePath string) bool {
	if request == nil || request.URL == nil {
		return false
	}
	if request.Method != http.MethodGet {
		return false
	}
	path := normalizeValidationPath(basePath, request.URL.Path)
	return strings.HasPrefix(path, "/schemas/")
}

func responseStrictIgnorePaths(request *http.Request, basePath string) []string {
	// Error.params is explicitly declared as free-form (additionalProperties: true).
	ignorePaths := []string{"$.body.params.**"}

	if request == nil || request.URL == nil {
		return ignorePaths
	}
	path := normalizeValidationPath(basePath, request.URL.Path)

	if shouldIgnoreDynamicSchemaResponseBody(request, basePath) {
		// Dynamic JSON Schema payload: free-form by design.
		ignorePaths = append(ignorePaths, "$.body.schema")
	}

	if strings.HasPrefix(path, "/admin/auth-providers") {
		// AuthProvider.config is free-form for plugin-specific settings.
		ignorePaths = append(ignorePaths, "$.body.config.**", "$.body.**.config.**")
	}
	if strings.HasPrefix(path, "/admin/instance-sizes") {
		// InstanceSize.spec_overrides is contractually free-form KubeVirt spec JSON.
		ignorePaths = append(ignorePaths, "$.body.spec_overrides.**", "$.body.**.spec_overrides.**")
	}
	if path == "/admin/auth-provider-types" {
		// AuthProviderType.config_schema is free-form JSON Schema content.
		ignorePaths = append(ignorePaths, "$.body.**.config_schema.**")
	}
	if strings.HasPrefix(path, "/admin/auth-providers/") {
		// Directory/runtime descriptors publish provider-owned request schemas,
		// and job details preserve provider-owned request snapshots. Both are
		// contractually free-form JSON objects even under strict mode.
		ignorePaths = append(
			ignorePaths,
			"$.body.request_schema",
			"$.body.request_schema.**",
			"$.body.**.request_schema.**",
			"$.body.provider_request",
			"$.body.provider_request.**",
			"$.body.**.provider_request.**",
			"$.body.request_snapshot",
			"$.body.request_snapshot.**",
			"$.body.**.request_snapshot.**",
		)
	}
	if path == "/admin/users" ||
		(strings.HasPrefix(path, "/systems/") &&
			(strings.HasSuffix(path, "/member-candidates") || strings.HasSuffix(path, "/members"))) {
		// User.profile_attributes is a contractually free-form directory profile map.
		// The system member candidate and member endpoints reuse the same
		// directory-backed profile bag semantics, so they need the same
		// strict-mode exemption as /admin/users.
		ignorePaths = append(ignorePaths, "$.body.**.profile_attributes.**")
	}
	if strings.HasPrefix(path, "/auth/preferences/") {
		// UserPreference.value is a generic per-user UI/runtime preference bag.
		ignorePaths = append(ignorePaths, "$.body.value.**")
	}
	if path == "/tickets" ||
		strings.HasPrefix(path, "/tickets/") ||
		path == "/builtin-approval/tasks" ||
		strings.HasPrefix(path, "/builtin-approval/tasks/") {
		// Ticket.ticket_payload is contextual and intentionally free-form.
		ignorePaths = append(ignorePaths, "$.body.**.ticket_payload.**")
	}
	if path == "/audit-logs" {
		// AuditLog.details is contractually free-form (additionalProperties: true).
		// Audit log details contain action-specific metadata (system_id, cluster_id,
		// environment, etc.) that is intentionally NOT declared in the schema.
		ignorePaths = append(ignorePaths, "$.body.**.details.**")
	}

	return ignorePaths
}

func (v *openAPIRuntimeValidator) newValidator(extraStrictIgnorePaths ...string) (validator.Validator, error) {
	// Browser clients and public forwarding layers may attach runtime/proxy
	// headers that are unrelated to declared API parameters. Keep strict mode
	// for contract governance, but ignore these transport artifacts.
	strictIgnorePaths := []string{"$.cookies.*"}
	if len(extraStrictIgnorePaths) > 0 {
		strictIgnorePaths = append(strictIgnorePaths, extraStrictIgnorePaths...)
	}

	openapiValidator, errs := validator.NewValidator(
		v.document,
		validatorconfig.WithStrictMode(),
		validatorconfig.WithSchemaCache(v.schemaCache),
		validatorconfig.WithStrictIgnoredHeadersExtra(
			"dnt",
			"forwarded",
			"priority",
			"upgrade-insecure-requests",
			"x-forwarded-server",
			"x-forwarded-host",
			"x-forwarded-port",
			"x-original-host",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"sec-fetch-dest",
			"sec-fetch-mode",
			"sec-fetch-site",
			"sec-fetch-user",
		),
		validatorconfig.WithStrictIgnorePaths(strictIgnorePaths...),
	)
	if len(errs) == 0 {
		return openapiValidator, nil
	}

	joined := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			joined = append(joined, err)
		}
	}
	if len(joined) == 0 {
		return nil, fmt.Errorf("create openapi validator: unknown error")
	}
	return nil, fmt.Errorf("create openapi validator: %w", errors.Join(joined...))
}

func normalizeBasePath(basePath string) string {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" || basePath == "/" {
		return ""
	}
	return "/" + strings.Trim(basePath, "/")
}

func normalizeValidationPath(basePath, path string) string {
	if basePath == "" {
		if path == "" {
			return "/"
		}
		return path
	}
	if path == basePath {
		return "/"
	}
	if strings.HasPrefix(path, basePath+"/") {
		return "/" + strings.TrimPrefix(path, basePath+"/")
	}
	return path
}

func applyValidationPath(request *http.Request, basePath string) func() {
	if request == nil || request.URL == nil {
		return func() {}
	}

	origPath := request.URL.Path
	origRawPath := request.URL.RawPath
	request.URL.Path = normalizeValidationPath(basePath, origPath)
	if origRawPath != "" {
		request.URL.RawPath = normalizeValidationPath(basePath, origRawPath)
	}

	return func() {
		request.URL.Path = origPath
		request.URL.RawPath = origRawPath
	}
}

func allPathMissing(errs []*validatorerrors.ValidationError) bool {
	if len(errs) == 0 {
		return false
	}
	for _, err := range errs {
		if err == nil || !err.IsPathMissingError() {
			return false
		}
	}
	return true
}

func hasRouteValidationError(errs []*validatorerrors.ValidationError) bool {
	for _, err := range errs {
		if err == nil {
			continue
		}
		if err.IsOperationMissingError() {
			return true
		}
		if err.ValidationType == "path" && !err.IsPathMissingError() {
			return true
		}
	}
	return false
}

func summarizeValidationErrors(errs []*validatorerrors.ValidationError) string {
	if len(errs) == 0 {
		return "request does not conform to OpenAPI contract"
	}
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		message := strings.TrimSpace(err.Message)
		if message == "" {
			message = strings.TrimSpace(err.Reason)
		}
		if len(err.SchemaValidationErrors) > 0 {
			schemaFailure := err.SchemaValidationErrors[0]
			detail := strings.TrimSpace(schemaFailure.Reason)
			if detail == "" {
				detail = strings.TrimSpace(schemaFailure.Error())
			}
			if detail != "" {
				if fieldPath := strings.TrimSpace(schemaFailure.FieldPath); fieldPath != "" {
					message = fmt.Sprintf("%s at %s", detail, fieldPath)
				} else if keywordLocation := strings.TrimSpace(schemaFailure.KeywordLocation); keywordLocation != "" {
					message = fmt.Sprintf("%s at %s", detail, keywordLocation)
				} else {
					message = detail
				}
			}
		}
		if message != "" {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return "request does not conform to OpenAPI contract"
	}
	if len(messages) == 1 {
		return messages[0]
	}
	return fmt.Sprintf("%s (and %d more validation errors)", messages[0], len(messages)-1)
}

func abortWithOpenAPIError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"code":    code,
		"message": message,
	})
}

func buildValidationResponse(request *http.Request, buffered *bufferedResponseWriter) *http.Response {
	body := io.NopCloser(bytes.NewReader(buffered.body.Bytes()))
	return &http.Response{
		StatusCode:    buffered.Status(),
		Header:        buffered.Header().Clone(),
		Body:          body,
		ContentLength: int64(buffered.body.Len()),
		Request:       request,
	}
}

type bufferedResponseWriter struct {
	gin.ResponseWriter
	body        bytes.Buffer
	statusCode  int
	wroteHeader bool
	size        int
}

func newBufferedResponseWriter(w gin.ResponseWriter) *bufferedResponseWriter {
	return &bufferedResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (w *bufferedResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = code
	w.wroteHeader = true
}

func (w *bufferedResponseWriter) WriteHeaderNow() {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
}

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.body.Write(data)
	w.size += n
	return n, err
}

func (w *bufferedResponseWriter) WriteString(s string) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.body.WriteString(s)
	w.size += n
	return n, err
}

func (w *bufferedResponseWriter) Status() int {
	if !w.wroteHeader {
		return http.StatusOK
	}
	return w.statusCode
}

func (w *bufferedResponseWriter) Size() int {
	return w.size
}

func (w *bufferedResponseWriter) Written() bool {
	return w.wroteHeader
}

func (w *bufferedResponseWriter) ResetJSON(statusCode int, payload map[string]string) {
	w.statusCode = statusCode
	w.wroteHeader = true
	w.body.Reset()
	w.size = 0
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"code":"OPENAPI_RESPONSE_INVALID","message":"response does not conform to OpenAPI contract"}`)
	}
	_, _ = w.Write(data)
}

func (w *bufferedResponseWriter) FlushToOriginal() (int, error) {
	w.ResponseWriter.WriteHeader(w.Status())
	if w.body.Len() == 0 {
		return 0, nil
	}
	return w.ResponseWriter.Write(w.body.Bytes())
}
