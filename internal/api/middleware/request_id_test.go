package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRequestIDPreservesValidHeader(t *testing.T) {
	t.Parallel()

	router := newRequestIDTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set(RequestIDHeader, "req-login_audit.2026:05")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if got := w.Header().Get(RequestIDHeader); got != "req-login_audit.2026:05" {
		t.Fatalf("response request id = %q, want valid incoming id", got)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "req-login_audit.2026:05" {
		t.Fatalf("handler request id = %q, want valid incoming id", got)
	}
}

func TestRequestIDReplacesInvalidHeader(t *testing.T) {
	t.Parallel()

	router := newRequestIDTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set(RequestIDHeader, "bad value")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	got := w.Header().Get(RequestIDHeader)
	if got == "" || got == "bad value" {
		t.Fatalf("response request id = %q, want generated replacement", got)
	}
	if _, err := uuid.Parse(got); err != nil {
		t.Fatalf("response request id = %q, want UUID: %v", got, err)
	}
	if strings.ContainsAny(got, " \r\n\t") {
		t.Fatalf("response request id contains unsafe characters: %q", got)
	}
	if body := strings.TrimSpace(w.Body.String()); body != got {
		t.Fatalf("handler request id = %q, want %q", body, got)
	}
}

func TestRequestIDReplacesOversizedHeader(t *testing.T) {
	t.Parallel()

	router := newRequestIDTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set(RequestIDHeader, strings.Repeat("a", maxRequestIDLength+1))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	got := w.Header().Get(RequestIDHeader)
	if _, err := uuid.Parse(got); err != nil {
		t.Fatalf("response request id = %q, want UUID: %v", got, err)
	}
}

func TestValidRequestIDRejectsControlCharacters(t *testing.T) {
	t.Parallel()

	if validRequestID("bad\r\nX-Evil: 1") {
		t.Fatal("validRequestID() = true, want false for control characters")
	}
}

func newRequestIDTestRouter(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, GetRequestID(c.Request.Context()))
	})
	return router
}
