package jobs

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsClusterRuntimeUnavailable_ClassifiesRuntimeDependencyFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "cluster not healthy",
			err:  fmt.Errorf("cluster cluster-a is not healthy (status: UNREACHABLE)"),
			want: true,
		},
		{
			name: "kubeconfig empty",
			err:  fmt.Errorf("cluster cluster-a kubeconfig is empty"),
			want: true,
		},
		{
			name: "semantic validation error",
			err:  fmt.Errorf("cpu limit exceeds quota"),
			want: false,
		},
		{
			name: "context canceled is worker lifecycle not cluster runtime",
			err:  context.Canceled,
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isClusterRuntimeUnavailable(tt.err); got != tt.want {
				t.Fatalf("isClusterRuntimeUnavailable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestJobContextErr(t *testing.T) {
	t.Parallel()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := jobContextErr(canceledCtx, nil); !errors.Is(got, context.Canceled) {
		t.Fatalf("jobContextErr(canceled ctx) = %v, want context.Canceled", got)
	}

	err := fmt.Errorf("provider returned: %w", context.Canceled)
	if got := jobContextErr(context.Background(), err); !errors.Is(got, context.Canceled) {
		t.Fatalf("jobContextErr(wrapped canceled err) = %v, want context.Canceled", got)
	}

	if got := jobContextErr(context.Background(), context.DeadlineExceeded); got != nil {
		t.Fatalf("jobContextErr(request deadline err with active worker ctx) = %v, want nil", got)
	}
}
