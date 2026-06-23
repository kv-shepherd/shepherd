package jobs

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestMain(m *testing.M) {
	if err := logger.Init("error", "json"); err != nil {
		panic(err)
	}
	testutil.MustStartDockerPG(m)
}
