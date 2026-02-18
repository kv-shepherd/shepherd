package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
		VMID:      "vm-1",
		VMName:    "vm-one",
		ClusterID: "cluster-a",
		Namespace: "dev",
		Actor:     "user-3",
	}
	data, err := deletePayload.ToJSON()
	require.NoError(t, err)
	var gotDelete VMDeletePayload
	require.NoError(t, json.Unmarshal(data, &gotDelete))
	require.Equal(t, deletePayload, gotDelete)

	powerPayload := VMPowerPayload{
		VMID:      "vm-2",
		VMName:    "vm-two",
		ClusterID: "cluster-b",
		Namespace: "prod",
		Operation: "restart",
		Actor:     "user-4",
	}
	data, err = powerPayload.ToJSON()
	require.NoError(t, err)
	var gotPower VMPowerPayload
	require.NoError(t, json.Unmarshal(data, &gotPower))
	require.Equal(t, powerPayload, gotPower)
}
