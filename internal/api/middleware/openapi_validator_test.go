package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

var testLoggerInit sync.Once

func newOpenAPIValidatorTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	return newOpenAPIValidatorTestRouterWithMode(t, gin.TestMode)
}

func newOpenAPIValidatorTestRouterWithMode(t *testing.T, mode string) *gin.Engine {
	t.Helper()
	testLoggerInit.Do(func() {
		_ = logger.Init("error", "console")
	})
	prevMode := gin.Mode()
	gin.SetMode(mode)
	t.Cleanup(func() {
		gin.SetMode(prevMode)
	})

	router := gin.New()
	router.Use(MustOpenAPIValidator("/api/v1"))
	return router
}

func TestNormalizeValidationPath(t *testing.T) {
	testCases := []struct {
		name     string
		basePath string
		path     string
		want     string
	}{
		{name: "strip prefix", basePath: "/api/v1", path: "/api/v1/vms/request", want: "/vms/request"},
		{name: "root path", basePath: "/api/v1", path: "/api/v1", want: "/"},
		{name: "no match", basePath: "/api/v1", path: "/health", want: "/health"},
		{name: "empty base", basePath: "", path: "/vms", want: "/vms"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeValidationPath(normalizeBasePath(tc.basePath), tc.path)
			if got != tc.want {
				t.Fatalf("normalizeValidationPath mismatch: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestOpenAPIValidatorRejectsInvalidSystemUpdateRequest(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.PATCH("/api/v1/systems/:system_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id":          c.Param("system_id"),
			"name":        "shop",
			"description": "updated",
			"created_by":  "u-1",
			"created_at":  time.Now().Format(time.RFC3339),
			"updated_at":  time.Now().Format(time.RFC3339),
		})
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/systems/sys-1", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid update body, got %d", resp.Code)
	}
	assertErrorCode(t, resp.Body.Bytes(), "OPENAPI_REQUEST_INVALID")
}

func TestOpenAPIValidatorAcceptsValidServiceUpdateRequest(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.PATCH("/api/v1/systems/:system_id/services/:service_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id":                  c.Param("service_id"),
			"name":                "redis",
			"description":         "new description",
			"system_id":           c.Param("system_id"),
			"next_instance_index": 1,
			"created_at":          time.Now().Format(time.RFC3339),
		})
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/systems/sys-1/services/svc-1", bytes.NewBufferString(`{"description":"new description"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid update body, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorRejectsInvalidVMCreateRequest(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/vms/request", func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{
			"ticket_id": "ticket-123",
			"status":    "PENDING",
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vms/request", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid request body, got %d", resp.Code)
	}
	assertErrorCode(t, resp.Body.Bytes(), "OPENAPI_REQUEST_INVALID")
}

func TestOpenAPIValidatorRejectsUndeclaredQueryParamInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live?debug=true", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for undeclared query param in strict mode, got %d body=%s", resp.Code, resp.Body.String())
	}
	assertErrorCode(t, resp.Body.Bytes(), "OPENAPI_REQUEST_INVALID")
}

func TestOpenAPIValidatorAllowsUndeclaredCookieInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	req.AddCookie(&http.Cookie{Name: "__next_hmr_refresh_hash__", Value: "dev"})
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for undeclared cookie in strict mode, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsBrowserRuntimeHeadersInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"token": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Dnt", "1")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="132", "Not=A?Brand";v="99"`)
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("X-Forwarded-Host", "10.1.111.111:3000")
	req.Header.Set("X-Forwarded-Port", "3000")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for browser runtime headers in strict mode, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAcceptsValidVMCreateRequest(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/vms/request", func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{
			"ticket_id": "ticket-123",
			"status":    "PENDING",
		})
	})

	reqBody := `{
		"service_id":"00000000-0000-0000-0000-000000000001",
		"template_id":"00000000-0000-0000-0000-000000000002",
		"instance_size_id":"00000000-0000-0000-0000-000000000003",
		"namespace":"team-a",
		"reason":"need vm for testing"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vms/request", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for valid request body, got %d, body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsNonContractPath(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/internal/metrics", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal/metrics", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected non-contract path to pass through, got %d", resp.Code)
	}
}

func TestOpenAPIValidatorRejectsInvalidResponseBody(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "NOT_OK"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for invalid response schema, got %d body=%s", resp.Code, resp.Body.String())
	}
	assertErrorCode(t, resp.Body.Bytes(), "OPENAPI_RESPONSE_INVALID")
}

func TestOpenAPIValidatorAcceptsReadinessDegradedResponse(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/health/ready", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "degraded",
			"checks": gin.H{"database": "error"},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for readiness degraded response, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAcceptsDynamicSchemaResponse(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/schemas/:entity_type", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"schema": gin.H{
				"$id":         "kv-shepherd:instancesize:kubevirt-v1.7.0",
				"$schema":     "https://json-schema.org/draft/2020-12/schema",
				"title":       "KubeVirt VirtualMachineSpec",
				"type":        "object",
				"description": "schema payload from backend cache",
				"properties": gin.H{
					"spec": gin.H{
						"type": "object",
					},
				},
			},
			"mask": gin.H{
				"quick_fields": []gin.H{
					{
						"path":         "spec.template.spec.domain.cpu.cores",
						"display_name": "CPU Cores",
					},
				},
			},
			"schema_version": "1.7.0",
			"source":         "embedded",
			"degraded":       false,
			"fetched_at":     time.Now().UTC().Format(time.RFC3339),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schemas/instancesize", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic schema response, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorRejectsSchemaFieldOutsideDynamicSchemaEndpoint(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"schema": gin.H{
				"unexpected": true,
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for undeclared schema field outside /schemas/*, got %d body=%s", resp.Code, resp.Body.String())
	}
	assertErrorCode(t, resp.Body.Bytes(), "OPENAPI_RESPONSE_INVALID")
}

func TestOpenAPIValidatorHidesRequestValidationDetailsInReleaseMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouterWithMode(t, gin.ReleaseMode)
	router.PATCH("/api/v1/systems/:system_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id":          c.Param("system_id"),
			"name":        "shop",
			"description": "updated",
			"created_by":  "u-1",
			"created_at":  time.Now().Format(time.RFC3339),
			"updated_at":  time.Now().Format(time.RFC3339),
		})
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/systems/sys-1", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid request body, got %d body=%s", resp.Code, resp.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error payload: %v body=%s", err, resp.Body.String())
	}
	if got := payload["code"]; got != "OPENAPI_REQUEST_INVALID" {
		t.Fatalf("unexpected error code: got=%v payload=%v", got, payload)
	}
	if got := payload["message"]; got != openAPIRequestValidationMessage {
		t.Fatalf("expected generic message in release mode, got=%v want=%q", got, openAPIRequestValidationMessage)
	}
}

func assertErrorCode(t *testing.T, body []byte, wantCode string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode error payload: %v body=%s", err, string(body))
	}
	if got := payload["code"]; got != wantCode {
		t.Fatalf("unexpected error code: got=%v want=%s payload=%v", got, wantCode, payload)
	}
}

func BenchmarkOpenAPIValidator_ValidRequest(b *testing.B) {
	testLoggerInit.Do(func() {
		_ = logger.Init("error", "console")
	})
	prevMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	b.Cleanup(func() {
		gin.SetMode(prevMode)
	})

	router := gin.New()
	router.Use(MustOpenAPIValidator("/api/v1"))
	router.POST("/api/v1/vms/request", func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{
			"ticket_id": "ticket-123",
			"status":    "PENDING",
		})
	})

	body := `{
		"service_id":"00000000-0000-0000-0000-000000000001",
		"template_id":"00000000-0000-0000-0000-000000000002",
		"instance_size_id":"00000000-0000-0000-0000-000000000003",
		"namespace":"team-a",
		"reason":"need vm for testing"
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/vms/request", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			b.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
		}
	}
}
