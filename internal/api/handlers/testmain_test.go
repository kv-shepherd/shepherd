package handlers

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestMain(m *testing.M) {
	_ = logger.Init("error", "json")
	testutil.MustStartDockerPG(m)
}
