package usecase

import (
	"testing"

	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

func TestValidateDeleteConfirmationByEnvironment(t *testing.T) {
	testCases := []struct {
		name        string
		environment namespaceregistry.Environment
		confirm     bool
		confirmName string
		wantErrCode string
	}{
		{
			name:        "test env accepts confirm true",
			environment: namespaceregistry.EnvironmentTest,
			confirm:     true,
		},
		{
			name:        "test env rejects confirm_name only",
			environment: namespaceregistry.EnvironmentTest,
			confirmName: "vm-01",
			wantErrCode: "DELETE_CONFIRMATION_REQUIRED",
		},
		{
			name:        "prod env requires confirm_name",
			environment: namespaceregistry.EnvironmentProd,
			confirm:     true,
			wantErrCode: "DELETE_CONFIRMATION_REQUIRED",
		},
		{
			name:        "prod env rejects mismatched confirm_name",
			environment: namespaceregistry.EnvironmentProd,
			confirmName: "other-vm",
			wantErrCode: "CONFIRMATION_NAME_MISMATCH",
		},
		{
			name:        "prod env accepts exact confirm_name",
			environment: namespaceregistry.EnvironmentProd,
			confirmName: "vm-01",
		},
		{
			name:        "unsupported environment rejected",
			environment: namespaceregistry.Environment("staging"),
			confirm:     true,
			wantErrCode: "UNSUPPORTED_NAMESPACE_ENVIRONMENT",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDeleteConfirmationByEnvironment("vm-01", tc.environment, tc.confirm, tc.confirmName)
			if tc.wantErrCode == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error code %s, got nil", tc.wantErrCode)
			}
			appErr, ok := apperrors.IsAppError(err)
			if !ok {
				t.Fatalf("expected AppError, got %T", err)
			}
			if appErr.Code != tc.wantErrCode {
				t.Fatalf("error code mismatch: got %s want %s", appErr.Code, tc.wantErrCode)
			}
		})
	}
}
