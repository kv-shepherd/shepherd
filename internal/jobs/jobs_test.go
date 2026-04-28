package jobs

import (
	"os"
	"testing"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func TestMain(m *testing.M) {
	if err := logger.Init("error", "json"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
