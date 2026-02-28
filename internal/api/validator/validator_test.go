package validator

import (
	"errors"
	"net/http"
	"sort"
	"testing"

	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

type createSystemReq struct {
	Name        string `json:"name" validate:"required,min=3,max=15"`
	Description string `json:"description" validate:"max=64"`
	CpuCores    int    `json:"cpu_cores" validate:"gte=1,lte=96"`
}

func TestValidateStruct_Success(t *testing.T) {
	req := createSystemReq{
		Name:        "demo-system",
		Description: "ok",
		CpuCores:    4,
	}
	if err := ValidateStruct(req); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateStruct_FieldErrors(t *testing.T) {
	req := createSystemReq{
		Name:     "",
		CpuCores: 0,
	}

	err := ValidateStruct(req)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("status=%d, want=%d", appErr.HTTPStatus, http.StatusBadRequest)
	}
	if appErr.Code != "INVALID_REQUEST" {
		t.Fatalf("code=%q, want=INVALID_REQUEST", appErr.Code)
	}
	if len(appErr.FieldErrors) < 2 {
		t.Fatalf("expected >=2 field errors, got %d", len(appErr.FieldErrors))
	}

	codesByField := map[string][]string{}
	for _, fe := range appErr.FieldErrors {
		codesByField[fe.Field] = append(codesByField[fe.Field], fe.Code)
	}
	for field := range codesByField {
		sort.Strings(codesByField[field])
	}

	if got := codesByField["name"]; len(got) == 0 {
		t.Fatalf("missing validation errors for field 'name': %+v", appErr.FieldErrors)
	}
	if got := codesByField["cpu_cores"]; len(got) == 0 {
		t.Fatalf("missing validation errors for field 'cpu_cores': %+v", appErr.FieldErrors)
	}
}

func TestValidateStruct_InvalidInput(t *testing.T) {
	err := ValidateStruct(nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.HTTPStatus != http.StatusInternalServerError {
		t.Fatalf("status=%d, want=%d", appErr.HTTPStatus, http.StatusInternalServerError)
	}
	if appErr.Code != "INTERNAL_ERROR" {
		t.Fatalf("code=%q, want=INTERNAL_ERROR", appErr.Code)
	}
}
