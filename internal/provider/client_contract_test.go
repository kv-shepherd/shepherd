package provider

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVirtualMachineClientContractUsesPatchForTargetedMutation(t *testing.T) {
	t.Parallel()

	vmClientType := reflect.TypeOf((*VirtualMachineClient)(nil)).Elem()

	_, hasPatch := vmClientType.MethodByName("Patch")
	require.True(t, hasPatch)

	_, hasCreate := vmClientType.MethodByName("Create")
	require.False(t, hasCreate)

	_, hasUpdate := vmClientType.MethodByName("Update")
	require.False(t, hasUpdate)
}
