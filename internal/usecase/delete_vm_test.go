package usecase

import (
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entvm "kv-shepherd.io/shepherd/ent/vm"
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
			name:        "prod env trims confirm_name whitespace",
			environment: namespaceregistry.EnvironmentProd,
			confirmName: "  vm-01  ",
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

func TestValidateDeleteConfirmationByEnvironment_ProdRejectsMissingSignal(t *testing.T) {
	t.Parallel()

	err := validateDeleteConfirmationByEnvironment("vm-01", namespaceregistry.EnvironmentProd, false, "")
	if err == nil {
		t.Fatal("expected DELETE_CONFIRMATION_REQUIRED, got nil")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "DELETE_CONFIRMATION_REQUIRED" {
		t.Fatalf("error code mismatch: got %s want DELETE_CONFIRMATION_REQUIRED", appErr.Code)
	}
}

func TestVMDeleteAllowedStatusAndErrorPayload(t *testing.T) {
	t.Parallel()

	allowed := []entvm.Status{
		entvm.StatusSTOPPED,
		entvm.StatusFAILED,
		entvm.StatusNOT_FOUND,
		entvm.StatusUNKNOWN,
	}
	for _, status := range allowed {
		if !VMDeleteAllowedStatus(status) {
			t.Fatalf("VMDeleteAllowedStatus(%q) = false, want true", status)
		}
	}

	if VMDeleteAllowedStatus(entvm.StatusRUNNING) {
		t.Fatal("VMDeleteAllowedStatus(RUNNING) = true, want false")
	}

	message := VMDeleteInvalidStateMessage(entvm.StatusRUNNING)
	if !strings.Contains(message, string(entvm.StatusRUNNING)) {
		t.Fatalf("VMDeleteInvalidStateMessage() = %q, want current status", message)
	}
	if !strings.Contains(message, VMDeleteAllowedStatesLabel()) {
		t.Fatalf("VMDeleteInvalidStateMessage() = %q, want allowed states label", message)
	}

	params := VMDeleteInvalidStateParams(entvm.StatusRUNNING)
	if got := params["current_state"]; got != string(entvm.StatusRUNNING) {
		t.Fatalf("params[current_state] = %#v, want %q", got, entvm.StatusRUNNING)
	}
	if got := params["allowed_states"]; got != VMDeleteAllowedStatesLabel() {
		t.Fatalf("params[allowed_states] = %#v, want %q", got, VMDeleteAllowedStatesLabel())
	}
}
