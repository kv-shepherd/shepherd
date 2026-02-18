package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func TestErrorHandler_AppErrorIncludesFieldErrors(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	_ = logger.Init("error", "console")

	router := gin.New()
	router.Use(ErrorHandler())
	router.GET("/test", func(c *gin.Context) {
		c.Error(apperrors.BadRequest("INVALID_REQUEST", "invalid input").WithFieldErrors([]apperrors.FieldError{
			{Field: "name", Code: "REQUIRED"},
		}))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusBadRequest)
	}

	var payload struct {
		Code        string                 `json:"code"`
		Message     string                 `json:"message"`
		FieldErrors []apperrors.FieldError `json:"field_errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != "INVALID_REQUEST" {
		t.Fatalf("unexpected code: got %q", payload.Code)
	}
	if len(payload.FieldErrors) != 1 || payload.FieldErrors[0].Field != "name" || payload.FieldErrors[0].Code != "REQUIRED" {
		t.Fatalf("unexpected field_errors: %+v", payload.FieldErrors)
	}
}
