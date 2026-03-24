package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
			"system_name":         "system-name",
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live?debug=true", http.NoBody)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", http.NoBody)
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

	req := httptest.NewRequest(http.MethodGet, "/internal/metrics", http.NoBody)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", http.NoBody)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", http.NoBody)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schemas/instancesize", http.NoBody)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for undeclared schema field outside /schemas/*, got %d body=%s", resp.Code, resp.Body.String())
	}
	assertErrorCode(t, resp.Body.Bytes(), "OPENAPI_RESPONSE_INVALID")
}

func TestOpenAPIValidatorSkipsResponseValidationForCanceledRequest(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/health/live", func(c *gin.Context) {
		// Intentionally invalid against Health schema to verify validator is skipped.
		c.JSON(http.StatusOK, gin.H{"status": "NOT_OK"})
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", http.NoBody).WithContext(ctx)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if strings.Contains(resp.Body.String(), "OPENAPI_RESPONSE_INVALID") {
		t.Fatalf("did not expect OPENAPI_RESPONSE_INVALID for canceled request, got body=%s", resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicAuthProviderConfigInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/auth-providers", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":        "provider-1",
					"name":      "corp-ldap",
					"auth_type": "ldap",
					"enabled":   true,
					"config": gin.H{
						"host": "ldap.example.com",
						"port": 389,
						"nested": gin.H{
							"base_dn": "dc=example,dc=com",
						},
					},
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth-providers", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic auth provider config, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicAuthProviderConfigInRequestBody(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/admin/auth-providers", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{
			"id":        "provider-1",
			"name":      "corp-oidc",
			"auth_type": "oidc",
			"enabled":   true,
			"config": gin.H{
				"issuer": "https://idp.example.com",
			},
		})
	})

	reqBody := `{
		"name":"corp-oidc",
		"auth_type":"oidc",
		"enabled":true,
		"config":{
			"test_endpoint":"https://idp.example.com/healthz",
			"nested":{"tenant":"team-a"}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth-providers", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 for request with dynamic auth provider config, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicInstanceSizeSpecOverridesInResponse(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/instance-sizes", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":            "size-1",
					"name":          "gpu-large",
					"display_name":  "GPU Large",
					"cpu_cores":     8,
					"memory_gi":     16,
					"catalog_scope": "all",
					"enabled":       true,
					"spec_overrides": gin.H{
						"spec": gin.H{
							"template": gin.H{
								"spec": gin.H{
									"domain": gin.H{
										"devices": gin.H{
											"gpus": []gin.H{
												{
													"name":       "gpu0",
													"deviceName": "nvidia.com/A10",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/instance-sizes", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for instance size response with dynamic spec_overrides, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicInstanceSizeSpecOverridesInRequestBody(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/admin/instance-sizes", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{
			"id":            "size-1",
			"name":          "gpu-large",
			"display_name":  "GPU Large",
			"cpu_cores":     8,
			"memory_gi":     16,
			"catalog_scope": "all",
			"enabled":       true,
			"spec_overrides": gin.H{
				"spec": gin.H{
					"template": gin.H{
						"spec": gin.H{
							"domain": gin.H{
								"devices": gin.H{
									"gpus": []gin.H{
										{
											"name":       "gpu0",
											"deviceName": "nvidia.com/A10",
										},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	reqBody := `{
		"name":"gpu-large",
		"display_name":"GPU Large",
		"cpu_cores":8,
		"memory_gi":16,
		"catalog_scope":"all",
		"enabled":true,
		"spec_overrides":{
			"spec":{
				"template":{
					"spec":{
						"domain":{
							"devices":{
								"gpus":[{"name":"gpu0","deviceName":"nvidia.com/A10"}]
							}
						}
					}
				}
			}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/instance-sizes", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 for instance size request with dynamic spec_overrides, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorStillRejectsUndeclaredAuthProviderRequestTopLevelField(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/admin/auth-providers", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{
			"id":        "provider-1",
			"name":      "corp-oidc",
			"auth_type": "oidc",
			"enabled":   true,
			"config":    gin.H{},
		})
	})

	reqBody := `{
		"name":"corp-oidc",
		"auth_type":"oidc",
		"rogue":"must-fail",
		"config":{"test_endpoint":"https://idp.example.com/healthz"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth-providers", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for undeclared top-level request field, got %d body=%s", resp.Code, resp.Body.String())
	}
	assertErrorCode(t, resp.Body.Bytes(), "OPENAPI_REQUEST_INVALID")
}

func TestOpenAPIValidatorAllowsDynamicAuthProviderTypeConfigSchemaInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/auth-provider-types", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"type":         "ldap",
					"display_name": "LDAP",
					"built_in":     true,
					"config_schema": gin.H{
						"type":                 "object",
						"required":             []string{"host"},
						"additionalProperties": false,
						"properties": gin.H{
							"host": gin.H{"type": "string"},
							"port": gin.H{"type": "integer"},
						},
					},
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth-provider-types", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic auth provider type config_schema, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicTicketPayloadInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/builtin-approval/tasks", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":        "ticket-1",
					"event_id":  "evt-1",
					"status":    "PENDING",
					"requester": "u-1",
					"ticket_payload": gin.H{
						"template_id":      "tpl-1",
						"instance_size_id": "size-1",
						"vm_name":          "vm-dev-1",
						"instance": gin.H{
							"name": "vm-dev-1",
						},
					},
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/builtin-approval/tasks", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic ticket payload, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicAuditLogDetailsInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/audit-logs", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":            "log-1",
					"action":        "system.create",
					"resource_type": "system",
					"resource_id":   "sys-1",
					"actor":         "admin",
					"created_at":    "2026-01-01T00:00:00Z",
					"details": gin.H{
						"system_id": "sys-1",
						"name":      "shop",
						"nested": gin.H{
							"environment": "prod",
						},
					},
				},
				{
					"id":            "log-2",
					"action":        "cluster.upsert_policy",
					"resource_type": "cluster_policy",
					"resource_id":   "policy-1",
					"actor":         "admin",
					"created_at":    "2026-01-01T00:00:00Z",
					"details": gin.H{
						"cluster_id":  "cl-1",
						"environment": "test",
					},
				},
			},
			"pagination": gin.H{
				"page":        1,
				"per_page":    20,
				"total":       2,
				"total_pages": 1,
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for audit log with dynamic details, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicErrorParamsInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/systems", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "REFERENCE_NOT_FOUND",
			"message": "referenced cluster does not exist",
			"params": gin.H{
				"resource_id": "cluster-1",
				"resource":    "cluster",
				"meta": gin.H{
					"scope": "test",
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/systems", bytes.NewBufferString(`{"name":"shop"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for error response with dynamic params, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAcceptsClusterCreateBadRequestResponse(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/admin/clusters", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "INVALID_KUBECONFIG",
			"message": "kubeconfig is invalid",
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/clusters", bytes.NewBufferString(`{"name":"c1","kubeconfig":"a3ViZWNvbmZpZw=="}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for cluster bad request response, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorStillRejectsUndeclaredAuthProviderTopLevelField(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/auth-providers", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":        "provider-1",
					"name":      "corp-ldap",
					"auth_type": "ldap",
					"enabled":   true,
					"rogue":     "must-fail",
					"config": gin.H{
						"host": "ldap.example.com",
						"port": 389,
					},
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth-providers", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for undeclared top-level field, got %d body=%s", resp.Code, resp.Body.String())
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
