package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
)

func TestBindAndValidateJSONReturnsRequestTooLargeForCappedBody(t *testing.T) {
	prevMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(prevMode)
	})

	router := gin.New()
	router.Use(middleware.MaxRequestBodyBytes(4))
	router.POST("/login", func(c *gin.Context) {
		var req generated.LoginRequest
		if !bindAndValidateJSON(c, &req) {
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = -1
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for capped JSON body, got %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "REQUEST_TOO_LARGE") {
		t.Fatalf("response body missing REQUEST_TOO_LARGE: %s", resp.Body.String())
	}
}
