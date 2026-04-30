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

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

var (
	testLoggerInit sync.Once

	testOpenAPIValidatorMu    sync.Mutex
	testOpenAPIValidatorCache = map[string]gin.HandlerFunc{}
)

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
	router.Use(cachedOpenAPIValidatorTestMiddleware(t, "/api/v1"))
	return router
}

func cachedOpenAPIValidatorTestMiddleware(t *testing.T, basePath string) gin.HandlerFunc {
	t.Helper()

	key := gin.Mode() + "\x00" + normalizeBasePath(basePath)
	testOpenAPIValidatorMu.Lock()
	defer testOpenAPIValidatorMu.Unlock()

	if middleware, ok := testOpenAPIValidatorCache[key]; ok {
		return middleware
	}

	middleware, err := NewOpenAPIValidator(basePath)
	if err != nil {
		t.Fatalf("init openapi validator: %v", err)
	}
	testOpenAPIValidatorCache[key] = middleware
	return middleware
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

func TestOpenAPIValidatorDoesNotBufferResponsesInReleaseMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouterWithMode(t, gin.ReleaseMode)
	router.GET("/api/v1/health/live", func(c *gin.Context) {
		if _, ok := c.Writer.(*bufferedResponseWriter); ok {
			c.JSON(http.StatusInternalServerError, gin.H{"buffered": true})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 without response buffering in release mode, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorPreservesRequestTooLargeStatus(t *testing.T) {
	router := gin.New()
	router.Use(MaxRequestBodyBytes(4))
	router.Use(cachedOpenAPIValidatorTestMiddleware(t, "/api/v1"))
	router.POST("/api/v1/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"token": "unused"})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for capped body before OpenAPI validation, got %d body=%s", resp.Code, resp.Body.String())
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
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid update body, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAcceptsForbiddenServiceOverviewResponse(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/services", func(c *gin.Context) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    "FORBIDDEN",
			"message": "forbidden",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for declared forbidden service overview response, got %d body=%s", resp.Code, resp.Body.String())
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

func TestOpenAPIValidatorAcceptsExternalAuthCallbackPostWithExtraFormFields(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/auth/providers/:provider_id/callback", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("ok"))
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/providers/provider-1/callback",
		strings.NewReader("state=state-1&token=callback-token"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for callback form body, got %d body=%s", resp.Code, resp.Body.String())
	}
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
	req.AddCookie(&http.Cookie{Name: "tunnel_phishing_protection", Value: "demo-codespaces-tunnel"})
	req.AddCookie(&http.Cookie{Name: ".Tunnels.Relay.WebForwarding.Cookies", Value: "demo-forwarding-cookie"})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en,zh-CN;q=0.9,zh-TW;q=0.8,zh;q=0.7")
	req.Header.Set("Dnt", "1")
	req.Header.Set("Origin", "https://shepherd-demo-3000.app.github.dev")
	req.Header.Set("Referer", "https://shepherd-demo-3000.app.github.dev/login")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="132", "Not=A?Brand";v="99"`)
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("X-Forwarded-For", "198.51.100.24")
	req.Header.Set("X-Forwarded-Host", "shepherd-demo-3000.app.github.dev")
	req.Header.Set("X-Forwarded-Port", "3000")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Forwarded", "for=198.51.100.24;proto=https;host=shepherd-demo-3000.app.github.dev")
	req.Header.Set("X-Forwarded-Server", "codespaces-proxy")
	req.Header.Set("X-Original-Host", "shepherd-demo-3000.app.github.dev")
	req.Header.Set("X-Real-Ip", "198.51.100.24")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for browser runtime headers in strict mode, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsProxyTransportHeadersOnNonAuthRoutes(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/health/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", http.NoBody)
	req.Host = "demo.kv-shepherd.io"
	req.AddCookie(&http.Cookie{Name: "shepherd-demo-session", Value: "signed-session"})
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en,zh-CN;q=0.9,zh-TW;q=0.8,zh;q=0.7")
	req.Header.Set("Origin", "https://demo.kv-shepherd.io")
	req.Header.Set("Referer", "https://demo.kv-shepherd.io/dashboard")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="132", "Not=A?Brand";v="99"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Linux"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Forwarded", "for=198.51.100.24;proto=https;host=demo.kv-shepherd.io")
	req.Header.Set("X-Forwarded-For", "198.51.100.24")
	req.Header.Set("X-Forwarded-Host", "demo.kv-shepherd.io")
	req.Header.Set("X-Forwarded-Port", "443")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Server", "edge-proxy")
	req.Header.Set("X-Original-Host", "demo.kv-shepherd.io")
	req.Header.Set("X-Real-Ip", "198.51.100.24")
	req.Header.Set("Cdn-Loop", "cloudflare")
	req.Header.Set("Cf-Connecting-Ip", "198.51.100.24")
	req.Header.Set("Cf-Ray", "demo-ray")
	req.Header.Set("Cf-Visitor", `{"scheme":"https"}`)
	req.Header.Set("Cf-Ipcountry", "US")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for proxy transport headers on non-auth route, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsForwardedHeadersOnPublicAuthDiscovery(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/auth/providers", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":        "provider-1",
					"name":      "Corp SSO",
					"auth_type": "oauth2",
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "tunnel_phishing_protection", Value: "demo-codespaces-tunnel"})
	req.AddCookie(&http.Cookie{Name: ".Tunnels.Relay.WebForwarding.Cookies", Value: "demo-forwarding-cookie"})
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en,zh-CN;q=0.9,zh-TW;q=0.8,zh;q=0.7")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Dnt", "1")
	req.Header.Set("Forwarded", "for=198.51.100.24;proto=https;host=shepherd-demo-3000.app.github.dev")
	req.Header.Set("X-Forwarded-For", "198.51.100.24")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Server", "codespaces-proxy")
	req.Header.Set("X-Real-Ip", "198.51.100.24")
	req.Header.Set("X-Original-Host", "shepherd-demo-3000.app.github.dev")
	req.Header.Set("Referer", "https://shepherd-demo-3000.app.github.dev/login")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for forwarded auth discovery request, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsRuntimeQueryMetadataOnPublicAuthDiscovery(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/auth/providers", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":        "provider-1",
					"name":      "Corp SSO",
					"auth_type": "oauth2",
				},
			},
		})
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/auth/providers?tunnel=1&rd=%2Flogin&id=jolly-horse-f9h5cv8&port=3000&name=potential-halibut&cluster=use",
		http.NoBody,
	)
	req.AddCookie(&http.Cookie{Name: "tunnel_phishing_protection", Value: "demo-codespaces-tunnel"})
	req.AddCookie(&http.Cookie{Name: ".Tunnels.Relay.WebForwarding.Cookies", Value: "demo-forwarding-cookie"})
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Forwarded", "for=198.51.100.24;proto=https;host=shepherd-demo-3000.app.github.dev")
	req.Header.Set("X-Forwarded-Host", "shepherd-demo-3000.app.github.dev")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Server", "codespaces-proxy")
	req.Header.Set("Referer", "https://shepherd-demo-3000.app.github.dev/login")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for public auth discovery with runtime query metadata, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsRuntimeQueryMetadataOnPublicAuthLogin(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"token": "ok"})
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/login?tunnel=1&rd=%2Flogin&id=jolly-horse-f9h5cv8&port=3000&name=potential-halibut&cluster=use",
		bytes.NewBufferString(`{"username":"admin","password":"admin"}`),
	)
	req.AddCookie(&http.Cookie{Name: "tunnel_phishing_protection", Value: "demo-codespaces-tunnel"})
	req.AddCookie(&http.Cookie{Name: ".Tunnels.Relay.WebForwarding.Cookies", Value: "demo-forwarding-cookie"})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://shepherd-demo-3000.app.github.dev")
	req.Header.Set("Referer", "https://shepherd-demo-3000.app.github.dev/login")
	req.Header.Set("X-Forwarded-Host", "shepherd-demo-3000.app.github.dev")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Server", "codespaces-proxy")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for public auth login with runtime query metadata, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsRuntimeQueryMetadataOnExternalAuthCallback(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/auth/providers/:provider_id/callback", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("ok"))
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/auth/providers/provider-1/callback?code=code-1&state=state-1&tunnel=1&iss=https%3A%2F%2Fissuer.example.com&session_state=session-1",
		http.NoBody,
	)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Referer", "https://potential-halibut-wrppqw7rj9q7cg5xj-3000.app.github.dev/login")
	req.Header.Set("X-Forwarded-Host", "potential-halibut-wrppqw7rj9q7cg5xj-3000.app.github.dev")
	req.Header.Set("X-Forwarded-Proto", "https")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for external auth callback with runtime query metadata, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorRejectsTunnelQueryOnNonAuthRoutes(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vms/request?tunnel=1", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-auth request with tunnel query, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorStillRejectsRuntimeQueryMetadataOnAuthenticatedAuthRoutes(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/auth/me", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id":       "user-1",
			"username": "admin",
			"roles":    []string{"platform:admin"},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me?tunnel=1&rd=%2Flogin&id=jolly-horse-f9h5cv8&port=3000&name=potential-halibut&cluster=use", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for authenticated auth route with runtime query metadata, got %d body=%s", resp.Code, resp.Body.String())
	}
	assertErrorCode(t, resp.Body.Bytes(), "OPENAPI_REQUEST_INVALID")
}

func TestOpenAPIValidatorSkipsWebSocketUpgradeRequests(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/vms/:vm_id/vnc", func(c *gin.Context) {
		c.Status(http.StatusSwitchingProtocols)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vms/vm-1/vnc", http.NoBody)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "c2hlcGhlcmQtdm5jLXByb2Jl")
	req.Header.Set("Origin", "http://127.0.0.1:3000")
	req.AddCookie(&http.Cookie{Name: "vnc_bootstrap", Value: "token"})
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 for websocket upgrade request, got %d body=%s", resp.Code, resp.Body.String())
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
	req.Header.Set("Authorization", "Bearer test-token")
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
				"$id":         "kv-shepherd:instancesize:kubevirt-v1.8.0",
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
			"schema_version": "1.8.0",
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
	req.Header.Set("Authorization", "Bearer test-token")
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
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 for request with dynamic auth provider config, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicDirectoryProviderRequestInRequestBody(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.POST("/api/v1/admin/auth-providers/provider-1/directory/preview", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"total_count": 0,
			"items":       []gin.H{},
		})
	})

	reqBody := `{
		"provider_request":{
			"department_names":["Engineering","Finance"],
			"include_nested":true,
			"limit":50
		},
		"conflict_resolution":"skip"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth-providers/provider-1/directory/preview", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for request with dynamic directory provider_request, got %d body=%s", resp.Code, resp.Body.String())
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
	req.Header.Set("Authorization", "Bearer test-token")
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
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 for instance size request with dynamic spec_overrides, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsCatalogDeleteConflictResponses(t *testing.T) {
	testCases := []struct {
		name   string
		path   string
		code   string
		params map[string]interface{}
	}{
		{
			name: "template delete conflict",
			path: "/api/v1/admin/templates/tpl-active",
			code: "TEMPLATE_HAS_ACTIVE_REQUESTS",
			params: map[string]interface{}{
				"active_request_count": 1,
			},
		},
		{
			name: "instance size delete conflict",
			path: "/api/v1/admin/instance-sizes/size-active",
			code: "INSTANCE_SIZE_HAS_ACTIVE_REQUESTS",
			params: map[string]interface{}{
				"active_request_count": 2,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := newOpenAPIValidatorTestRouter(t)
			router.DELETE(tc.path, func(c *gin.Context) {
				c.JSON(http.StatusConflict, generated.Error{
					Code:    tc.code,
					Message: "resource is referenced by active VM create requests",
					Params:  tc.params,
				})
			})

			req := httptest.NewRequest(http.MethodDelete, tc.path, http.NoBody)
			req.Header.Set("Authorization", "Bearer test-token")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusConflict {
				t.Fatalf("expected 409 for %s, got %d body=%s", tc.path, resp.Code, resp.Body.String())
			}
			assertErrorCode(t, resp.Body.Bytes(), tc.code)
		})
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
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic auth provider type config_schema, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicDirectorySyncDescriptorRequestSchemaInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/auth-providers/provider-1/directory/descriptor", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"display_name":     "Directory enrichment",
			"description":      "sync and enrich users from an external directory",
			"supports_preview": false,
			"request_schema": gin.H{
				"type":                 "object",
				"required":             []string{"department_ids"},
				"additionalProperties": false,
				"properties": gin.H{
					"department_ids": gin.H{
						"type":  "array",
						"items": gin.H{"type": "string"},
					},
					"include_nested": gin.H{"type": "boolean"},
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth-providers/provider-1/directory/descriptor", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic directory request_schema, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicDirectoryScheduleProviderRequestInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/auth-providers/provider-1/directory/schedule", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"supported":         true,
			"enabled":           true,
			"mode":              "enrich_existing_only",
			"join_key_type":     "username",
			"schedule_cron":     "0 * * * *",
			"schedule_timezone": "Asia/Shanghai",
			"provider_request": gin.H{
				"department_names": []string{"Engineering", "Finance"},
				"include_nested":   true,
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth-providers/provider-1/directory/schedule", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic directory schedule provider_request, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicDirectorySyncJobRequestSnapshotInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/auth-providers/provider-1/directory/sync-jobs/job-1", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id":                  "job-1",
			"provider_id":         "provider-1",
			"status":              "completed",
			"conflict_resolution": "skip",
			"sync_mode":           "scheduled_enrichment",
			"join_key_type":       "username",
			"total_entries":       116,
			"result_summary": gin.H{
				"create_count":  0,
				"update_count":  0,
				"blocked_count": 116,
			},
			"error_count":  0,
			"errors":       []string{},
			"triggered_by": "system",
			"created_at":   "2026-03-30T08:10:00Z",
			"updated_at":   "2026-03-30T08:10:00Z",
			"completed_at": "2026-03-30T08:10:00Z",
			"request_snapshot": gin.H{
				"department_ids": []string{"1001", "1002"},
				"include_nested": true,
				"filters": gin.H{
					"status": "active",
				},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth-providers/provider-1/directory/sync-jobs/job-1", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic directory request_snapshot, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicUserProfileAttributesInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":         "user-1",
					"username":   "alice@example.com",
					"enabled":    true,
					"created_at": "2026-03-31T00:00:00Z",
					"profile_attributes": gin.H{
						"department":   "Engineering",
						"section":      "Platform",
						"phone_number": "13800000000",
					},
				},
			},
			"profile_fields": []gin.H{
				{
					"key":        "department",
					"label":      "Department",
					"searchable": true,
				},
			},
			"pagination": gin.H{
				"page":        1,
				"per_page":    20,
				"total":       1,
				"total_pages": 1,
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic user profile_attributes, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicSystemMemberCandidateProfileAttributesInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/systems/:system_id/member-candidates", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"id":         "user-1",
					"username":   "alice@example.com",
					"enabled":    true,
					"created_at": "2026-03-31T00:00:00Z",
					"profile_attributes": gin.H{
						"department":       "Engineering",
						"section":          "Platform",
						"external_cohorts": []any{gin.H{"kind": "department", "key": "2953"}},
					},
				},
			},
			"profile_fields": []gin.H{
				{
					"key":        "department",
					"label":      "Department",
					"searchable": true,
				},
			},
			"pagination": gin.H{
				"page":        1,
				"per_page":    20,
				"total":       1,
				"total_pages": 1,
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/systems/system-1/member-candidates", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic system member candidate profile_attributes, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicSystemMemberProfileAttributesInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/systems/:system_id/members", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []gin.H{
				{
					"user_id":    "user-1",
					"username":   "alice@example.com",
					"created_at": "2026-03-31T00:00:00Z",
					"role":       "member",
					"profile_attributes": gin.H{
						"department":       "Engineering",
						"section":          "Platform",
						"external_cohorts": []any{gin.H{"kind": "department", "key": "2953"}},
					},
				},
			},
			"profile_fields": []gin.H{
				{
					"key":        "department",
					"label":      "Department",
					"searchable": true,
				},
			},
			"pagination": gin.H{
				"page":        1,
				"per_page":    20,
				"total":       1,
				"total_pages": 1,
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/systems/system-1/members", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic system member profile_attributes, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestOpenAPIValidatorAllowsDynamicUserPreferenceValueInStrictMode(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.PUT("/api/v1/auth/preferences/:key", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"key":        c.Param("key"),
			"updated_at": "2026-03-31T00:00:00Z",
			"value": gin.H{
				"columns": []string{"profile:department", "roles", "status"},
			},
		})
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/preferences/admin.users.columns.v1", strings.NewReader(`{
		"value": {
			"columns": ["profile:department", "roles", "status"]
		}
	}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for dynamic user preference value, got %d body=%s", resp.Code, resp.Body.String())
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
	req.Header.Set("Authorization", "Bearer test-token")
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
					"message_i18n": gin.H{
						"key": "audit.message.generic",
						"params": gin.H{
							"action":          "system.create",
							"actor":           "admin",
							"actorDisplay":    "admin",
							"resourceType":    "system",
							"resourceId":      "sys-1",
							"resourceDisplay": "sys-1",
						},
					},
					"created_at": "2026-01-01T00:00:00Z",
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
					"message_i18n": gin.H{
						"key": "audit.message.generic",
						"params": gin.H{
							"action":          "cluster.upsert_policy",
							"actor":           "admin",
							"actorDisplay":    "admin",
							"resourceType":    "cluster_policy",
							"resourceId":      "policy-1",
							"resourceDisplay": "policy-1",
						},
					},
					"created_at": "2026-01-01T00:00:00Z",
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

func TestOpenAPIValidatorAcceptsDirectorySyncJobDetailWithRequestSnapshot(t *testing.T) {
	router := newOpenAPIValidatorTestRouter(t)
	router.GET("/api/v1/admin/auth-providers/:provider_id/directory/sync-jobs/:job_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id":                  "019d3ea6-c4dd-7656-961d-9d0a771ff490",
			"provider_id":         "019d3cd8-dd29-7b61-9227-f7aad59505a7",
			"status":              "completed",
			"conflict_resolution": "skip",
			"sync_mode":           "scheduled_enrichment",
			"join_key_type":       "username",
			"total_entries":       116,
			"result_summary": gin.H{
				"blocked_count": 116,
				"create_count":  0,
				"update_count":  0,
			},
			"error_count":  0,
			"errors":       []string{},
			"triggered_by": "system:directory-enrichment-scheduler",
			"started_at":   "2026-03-30T20:10:10.110159+08:00",
			"completed_at": "2026-03-30T20:10:34.465529+08:00",
			"created_at":   "2026-03-30T20:10:10.013429+08:00",
			"updated_at":   "2026-03-30T20:10:34.465533+08:00",
			"request_snapshot": gin.H{
				"include_nested":   true,
				"department_names": []string{"Engineering"},
			},
		})
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/auth-providers/019d3cd8-dd29-7b61-9227-f7aad59505a7/directory/sync-jobs/019d3ea6-c4dd-7656-961d-9d0a771ff490",
		http.NoBody,
	)
	req.Header.Set("Authorization", "Bearer test-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for directory sync job detail, got %d body=%s", resp.Code, resp.Body.String())
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
	req.Header.Set("Authorization", "Bearer test-token")
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
