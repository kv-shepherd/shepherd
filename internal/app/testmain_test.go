package app

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.MustStartDockerPG(m)
}
