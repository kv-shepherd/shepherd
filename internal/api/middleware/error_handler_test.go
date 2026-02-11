package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
	_ = logger.Init("error", "json")
}

func TestErrorHandler_NoErrors(t *testing.T) {
	router := gin.New()
	router.Use(ErrorHandler())
	router.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestErrorHandler_AppError(t *testing.T) {
	router := gin.New()
	router.Use(ErrorHandler())
	router.GET("/fail", func(c *gin.Context) {
		_ = c.Error(apperrors.NotFound("VM_NOT_FOUND", "Virtual machine not found"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["code"] != "VM_NOT_FOUND" {
		t.Errorf("code = %q, want VM_NOT_FOUND", body["code"])
	}
}

func TestErrorHandler_GenericError(t *testing.T) {
	router := gin.New()
	router.Use(ErrorHandler())
	router.GET("/err", func(c *gin.Context) {
		_ = c.Error(fmt.Errorf("something unexpected"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["code"] != "INTERNAL_ERROR" {
		t.Errorf("code = %q, want INTERNAL_ERROR", body["code"])
	}
}
