package validator

import (
	"errors"
	"net/http"
	"sort"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

type createSystemReq struct {
	Name        string `json:"name" validate:"required,min=3,max=15"`
	Description string `json:"description" validate:"max=64"`
	CPUCores    int    `json:"cpu_cores" validate:"gte=1,lte=96"`
}

func TestValidateStruct_Success(t *testing.T) {
	req := createSystemReq{
		Name:        "demo-system",
		Description: "ok",
		CPUCores:    4,
	}
	if err := ValidateStruct(req); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateStruct_FieldErrors(t *testing.T) {
	req := createSystemReq{
		Name:     "",
		CPUCores: 0,
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

func TestValidateStruct_GovernanceNamePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload any
		wantErr bool
	}{
		{
			name: "system accepts conservative rfc1035 name",
			payload: generated.SystemCreateRequest{
				Name: "shop-1",
			},
		},
		{
			name: "service rejects consecutive hyphens",
			payload: generated.ServiceCreateRequest{
				Name: "shop--api",
			},
			wantErr: true,
		},
		{
			name: "namespace rejects trailing hyphen",
			payload: generated.NamespaceCreateRequest{
				Name:        "team-prod-",
				Environment: generated.NamespaceCreateRequestEnvironmentProd,
			},
			wantErr: true,
		},
		{
			name: "namespace accepts current public length contract",
			payload: generated.NamespaceCreateRequest{
				Name:        "team-prod-advisory",
				Environment: generated.NamespaceCreateRequestEnvironmentProd,
			},
		},
		{
			name: "system rejects digit prefix",
			payload: generated.SystemCreateRequest{
				Name: "1shop",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateStruct(tt.payload)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
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
