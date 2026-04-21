package ticketing

import (
	"fmt"
	"testing"
)

func TestIsClusterRuntimeUnavailable_ClassifiesHealthAndKubeconfigFailures(t *testing.T) {
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
			err:  fmt.Errorf("source PVC team-a/golden was not found on selected cluster"),
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
