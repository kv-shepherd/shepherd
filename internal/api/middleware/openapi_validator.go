package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const openAPIResponseValidationMessage = "response does not conform to OpenAPI contract"

// MustOpenAPIValidator creates an OpenAPI runtime validator middleware and panics on setup failure.
func MustOpenAPIValidator(basePath string) gin.HandlerFunc {
	mw, err := NewOpenAPIValidator(basePath)
	if err != nil {
		panic(fmt.Sprintf("init openapi validator: %v", err))
	}
	return mw
}

// NewOpenAPIValidator validates request + response against the generated OpenAPI spec.
func NewOpenAPIValidator(basePath string) (gin.HandlerFunc, error) {
	swagger, err := generated.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load generated swagger: %w", err)
	}

	router, err := gorillamux.NewRouter(swagger)
	if err != nil {
		return nil, fmt.Errorf("create swagger router: %w", err)
	}

	basePath = normalizeBasePath(basePath)

	return func(c *gin.Context) {
		origPath := c.Request.URL.Path
		origRawPath := c.Request.URL.RawPath

		route, pathParams, routeErr := findRouteWithFallback(router, c.Request, basePath)
		if routeErr != nil {
			c.Request.URL.Path = origPath
			c.Request.URL.RawPath = origRawPath
			// Route resolution mismatch should not break non-OpenAPI paths.
			if isPathNotFoundError(routeErr) {
				c.Next()
				return
			}
			abortWithOpenAPIError(c, http.StatusBadRequest, "OPENAPI_ROUTE_INVALID", routeErr.Error())
			return
		}

		reqValidationInput := &openapi3filter.RequestValidationInput{
			Request:    c.Request,
			PathParams: pathParams,
			Route:      route,
			Options: &openapi3filter.Options{
				AuthenticationFunc: func(context.Context, *openapi3filter.AuthenticationInput) error {
					// JWT/RBAC are handled by dedicated middleware in router chain.
					return nil
				},
			},
		}
		if err := openapi3filter.ValidateRequest(c.Request.Context(), reqValidationInput); err != nil {
			c.Request.URL.Path = origPath
			c.Request.URL.RawPath = origRawPath
			abortWithOpenAPIError(c, http.StatusBadRequest, "OPENAPI_REQUEST_INVALID", err.Error())
			return
		}

		c.Request.URL.Path = origPath
		c.Request.URL.RawPath = origRawPath

		buffered := newBufferedResponseWriter(c.Writer)
		c.Writer = buffered
		c.Next()

		respValidationInput := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: reqValidationInput,
			Status:                 buffered.Status(),
			Header:                 buffered.Header().Clone(),
			Options: &openapi3filter.Options{
				AuthenticationFunc: func(context.Context, *openapi3filter.AuthenticationInput) error { return nil },
			},
		}
		if buffered.Size() > 0 {
			respValidationInput.SetBodyBytes(buffered.body.Bytes())
		}

		if err := openapi3filter.ValidateResponse(c.Request.Context(), respValidationInput); err != nil {
			logger.Error("OpenAPI response validation failed",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Int("status", buffered.Status()),
				zap.Error(err),
			)
			buffered.ResetJSON(http.StatusInternalServerError, map[string]string{
				"code":    "OPENAPI_RESPONSE_INVALID",
				"message": openAPIResponseValidationMessage,
			})
		}

		if _, err := buffered.FlushToOriginal(); err != nil {
			logger.Warn("failed to flush buffered response",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Error(err),
			)
		}
	}, nil
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

func findRouteWithFallback(
	router routers.Router,
	req *http.Request,
	basePath string,
) (*routers.Route, map[string]string, error) {
	origPath := req.URL.Path
	origRawPath := req.URL.RawPath

	candidates := [][2]string{{origPath, origRawPath}}
	normalizedPath := normalizeValidationPath(basePath, origPath)
	normalizedRawPath := origRawPath
	if origRawPath != "" {
		normalizedRawPath = normalizeValidationPath(basePath, origRawPath)
	}
	if normalizedPath != origPath || normalizedRawPath != origRawPath {
		candidates = append(candidates, [2]string{normalizedPath, normalizedRawPath})
	}

	var lastErr error
	for _, candidate := range candidates {
		req.URL.Path = candidate[0]
		req.URL.RawPath = candidate[1]

		route, pathParams, err := router.FindRoute(req)
		if err == nil {
			return route, pathParams, nil
		}
		if !isPathNotFoundError(err) {
			return nil, nil, err
		}
		lastErr = err
	}

	req.URL.Path = origPath
	req.URL.RawPath = origRawPath
	return nil, nil, lastErr
}

func isPathNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if err == routers.ErrPathNotFound {
		return true
	}
	if strings.Contains(err.Error(), routers.ErrPathNotFound.Error()) {
		return true
	}
	if routeErr, ok := err.(*routers.RouteError); ok && strings.Contains(routeErr.Reason, routers.ErrPathNotFound.Error()) {
		return true
	}
	return false
}

func abortWithOpenAPIError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"code":    code,
		"message": message,
	})
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
	return w.Write([]byte(s))
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
