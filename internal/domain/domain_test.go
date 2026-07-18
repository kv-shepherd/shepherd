package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBatchChildMaxAttemptsIncludesInitialDispatchAndRetries(t *testing.T) {
	t.Parallel()

	const initialDispatch = 1
	const allowedExplicitRetries = 2
	if BatchChildMaxAttempts != initialDispatch+allowedExplicitRetries {
		t.Fatalf(
			"BatchChildMaxAttempts = %d, want %d logical attempts",
			BatchChildMaxAttempts,
			initialDispatch+allowedExplicitRetries,
		)
	}
}

func TestVMCreationPayload_ToJSON(t *testing.T) {
	payload := VMCreationPayload{
		RequesterID:    "user-1",
		ServiceID:      "svc-1",
		TemplateID:     "tpl-1",
		InstanceSizeID: "size-1",
		Namespace:      "dev",
		Reason:         "load-test",
	}

	data, err := payload.ToJSON()
	require.NoError(t, err)

	var decoded VMCreationPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, payload, decoded)
}

func TestBatchVMRequestPayload_ToJSON(t *testing.T) {
	ts := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	payload := BatchVMRequestPayload{
		Operation:   "create",
		RequestID:   "req-123",
		Reason:      "scale-out",
		SubmittedBy: "user-2",
		SubmittedAt: ts,
		Items: []BatchVMItemPayload{
			{
				ServiceID:      "svc-1",
				TemplateID:     "tpl-2",
				InstanceSizeID: "size-2",
				Namespace:      "prod",
				Reason:         "capacity",
			},
		},
	}

	data, err := payload.ToJSON()
	require.NoError(t, err)

	var decoded BatchVMRequestPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, payload.Operation, decoded.Operation)
	require.Equal(t, payload.RequestID, decoded.RequestID)
	require.Equal(t, payload.SubmittedBy, decoded.SubmittedBy)
	require.Equal(t, payload.SubmittedAt.UTC(), decoded.SubmittedAt.UTC())
	require.Len(t, decoded.Items, 1)
	require.Equal(t, payload.Items[0], decoded.Items[0])
}

func TestPowerAndDeletePayload_ToJSON(t *testing.T) {
	deletePayload := VMDeletePayload{
		VMID:               "vm-1",
		VMName:             "vm-one",
		ClusterID:          "cluster-a",
		ClusterName:        "cluster-a",
		ClusterEnvironment: "test",
		Namespace:          "dev",
		SystemID:           "system-1",
		SystemName:         "Payments",
		ServiceID:          "service-1",
		ServiceName:        "billing-worker",
		OwnerID:            "user-3",
		OwnerDisplayName:   "Alex Chen",
		OwnerUsername:      "alexchen",
		TemplateID:         "template-1",
		TemplateName:       "OpenEuler 22.03",
		InstanceSizeID:     "size-1",
		InstanceSizeName:   "M4 Large",
		RequestVMStatus:    "STOPPED",
		CurrentCPUCores:    4,
		CurrentMemoryGi:    8,
		CurrentDiskGB:      60,
		Actor:              "user-3",
	}
	data, err := deletePayload.ToJSON()
	require.NoError(t, err)
	var gotDelete VMDeletePayload
	require.NoError(t, json.Unmarshal(data, &gotDelete))
	require.Equal(t, deletePayload, gotDelete)

	powerPayload := VMPowerPayload{
		VMID:         "vm-2",
		VMName:       "vm-two",
		ClusterID:    "cluster-b",
		Namespace:    "prod",
		Operation:    "restart",
		Actor:        "user-4",
		DispatchMode: VMPowerDispatchDirect,
	}
	data, err = powerPayload.ToJSON()
	require.NoError(t, err)
	var gotPower VMPowerPayload
	require.NoError(t, json.Unmarshal(data, &gotPower))
	require.Equal(t, powerPayload, gotPower)
}

func TestBatchVMRequestPayload_ToJSON_WithReadableContext(t *testing.T) {
	ts := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	targetCPU := 8.0
	targetMemory := 16.0
	targetDisk := 120
	payload := BatchVMRequestPayload{
		Operation:   "modify",
		RequestID:   "req-456",
		Reason:      "capacity planning",
		SubmittedBy: "user-2",
		SubmittedAt: ts,
		Items: []BatchVMItemPayload{
			{
				VMName:             "billing-api-01",
				SystemID:           "system-1",
				SystemName:         "Payments",
				ServiceID:          "service-1",
				ServiceName:        "billing-api",
				Namespace:          "gtest1",
				ClusterID:          "cluster-1",
				ClusterName:        "kubevirt-test02",
				ClusterEnvironment: "test",
				OwnerID:            "user-9",
				OwnerDisplayName:   "Alex Chen",
				OwnerUsername:      "alexchen",
				TemplateID:         "template-1",
				TemplateName:       "OpenEuler 22.03",
				InstanceSizeID:     "size-1",
				InstanceSizeName:   "M4 Large",
				RequestVMStatus:    "RUNNING",
				CurrentCPUCores:    4,
				CurrentMemoryGi:    8,
				CurrentDiskGB:      60,
				Operation:          "modify",
				TargetCPUCores:     &targetCPU,
				TargetMemoryGi:     &targetMemory,
				TargetDiskGB:       &targetDisk,
			},
		},
	}

	data, err := payload.ToJSON()
	require.NoError(t, err)

	var decoded BatchVMRequestPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, payload, decoded)
}

func TestVMJSON_OmitsResourceVersion(t *testing.T) {
	t.Parallel()

	vm := VM{
		ID:              "vm-1",
		Name:            "vm-1",
		Namespace:       "dev",
		Cluster:         "cluster-a",
		Status:          VMStatusRunning,
		ResourceVersion: "123456",
	}

	data, err := json.Marshal(vm)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))
	_, exists := decoded["resource_version"]
	require.False(t, exists, "resource_version must be excluded from JSON payload")
}
