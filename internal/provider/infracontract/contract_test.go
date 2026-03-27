package infracontract

import (
	"context"
	"net"
	"testing"

	"kv-shepherd.io/shepherd/internal/domain"
)

type stubConsoleProvider struct{}

func (stubConsoleProvider) GetVNCConnection(context.Context, string, string, string) (*domain.ConsoleConnection, error) {
	return nil, nil
}

func (stubConsoleProvider) GetSerialConsole(context.Context, string, string, string) (*domain.ConsoleConnection, error) {
	return nil, nil
}

type stubVMMutationProvider struct{}

func (stubVMMutationProvider) DryRunVMMutation(context.Context, string, string, string, *domain.VMMutation) error {
	return nil
}

func (stubVMMutationProvider) ExecuteVMMutation(context.Context, string, string, string, *domain.VMMutation) (*domain.VM, error) {
	return nil, nil
}

type stubVNCStreamProvider struct{}

func (stubVNCStreamProvider) OpenVNCStream(context.Context, string, string, string) (net.Conn, error) {
	return nil, nil
}

type stubSerialConsoleStreamProvider struct{}

func (stubSerialConsoleStreamProvider) OpenSerialConsoleStream(context.Context, string, string, string) (net.Conn, error) {
	return nil, nil
}

var (
	_ ConsoleProvider             = (*stubConsoleProvider)(nil)
	_ VMMutationProvider          = (*stubVMMutationProvider)(nil)
	_ VNCStreamProvider           = (*stubVNCStreamProvider)(nil)
	_ SerialConsoleStreamProvider = (*stubSerialConsoleStreamProvider)(nil)
)

func TestListOptionsZeroValue(t *testing.T) {
	var options ListOptions
	if options.Limit != 0 {
		t.Fatalf("ListOptions.Limit = %d, want 0", options.Limit)
	}
	if options.SkipVMIEnrichment {
		t.Fatal("ListOptions.SkipVMIEnrichment = true, want false")
	}
}
