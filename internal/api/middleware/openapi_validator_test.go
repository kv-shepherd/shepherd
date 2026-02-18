package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

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
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MustOpenAPIValidator("/api/v1"))
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
}

func TestOpenAPIValidatorAcceptsValidServiceUpdateRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MustOpenAPIValidator("/api/v1"))
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
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MustOpenAPIValidator("/api/v1"))
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
}

func TestOpenAPIValidatorAcceptsValidVMCreateRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MustOpenAPIValidator("/api/v1"))
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
