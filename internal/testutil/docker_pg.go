// Package testutil provides shared test helpers for KubeVirt Shepherd.
// docker_pg.go: Auto-provision a temporary PostgreSQL 18 container for tests.
//
// Usage in any test package that requires PostgreSQL:
//
//	// testmain_test.go
//	func TestMain(m *testing.M) {
//	    testutil.MustStartDockerPG(m)
//	}
//
// MustStartDockerPG checks for an existing TEST_DATABASE_URL / DATABASE_URL.
// If found, it is used directly (CI / pre-existing containers).
// If not found, it launches a postgres:18 container, waits for it to be
// healthy, sets TEST_DATABASE_URL in the process environment, runs tests,
// then removes the container on exit.
package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	dockerPGImage      = "postgres:18"
	dockerPGUser       = "shepherd"
	dockerPGPassword   = "shepherd"
	dockerPGDB         = "shepherd_test"
	dockerPGHealthWait = 90 * time.Second
)

// MustStartDockerPG ensures a PostgreSQL 18 instance is available for the
// test binary.  Call it from TestMain:
//
//	func TestMain(m *testing.M) { testutil.MustStartDockerPG(m) }
//
// Behaviour:
//   - If TEST_DATABASE_URL or DATABASE_URL is already set, use it as-is and
//     run tests without touching Docker.
//   - Otherwise, pull up a temporary postgres:18 container, wait for it to
//     become healthy, inject TEST_DATABASE_URL into the process environment,
//     run the test suite, then remove the container regardless of outcome.
func MustStartDockerPG(m *testing.M) {
	os.Exit(runWithDockerPG(m))
}

// runWithDockerPG executes the test suite and returns the process exit code.
// This keeps cleanup in deferred functions, and the caller can os.Exit(code).
func runWithDockerPG(m *testing.M) int {
	cmdCtx := context.Background()

	// If a DSN is already provided (CI / developer with local PG), use it.
	if os.Getenv("TEST_DATABASE_URL") != "" || os.Getenv("DATABASE_URL") != "" {
		return m.Run()
	}

	// Verify Docker is available before attempting container start.
	if err := exec.CommandContext(cmdCtx, "docker", "info").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Docker is not available and no TEST_DATABASE_URL is set.\n"+
			"  Either install Docker, or set TEST_DATABASE_URL to an existing PostgreSQL 18 DSN.\n"+
			"  Error: %v\n", err)
		return 1
	}

	name := fmt.Sprintf("shepherd-test-pg-%d-%s", time.Now().Unix(), uuid.NewString()[:8])

	// Always clean up the container before returning to the caller.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", name).Run()
	}()

	port, err := startPGContainer(cmdCtx, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: failed to start PostgreSQL container: %v\n", err)
		return 1
	}

	dsn := fmt.Sprintf("postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable",
		dockerPGUser, dockerPGPassword, port, dockerPGDB)

	if err := os.Setenv("TEST_DATABASE_URL", dsn); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: cannot set TEST_DATABASE_URL: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "INFO: PostgreSQL 18 test container %q ready on port %s\n", name, port)

	return m.Run()
}

// startPGContainer launches the container, resolves its mapped port, and waits
// until the health-check reports "healthy".  Returns the mapped host port.
func startPGContainer(ctx context.Context, name string) (string, error) {
	// Start container with a random host port and built-in health-check.
	runArgs := []string{
		"run", "-d",
		"--name", name,
		"-e", "POSTGRES_USER=" + dockerPGUser,
		"-e", "POSTGRES_PASSWORD=" + dockerPGPassword,
		"-e", "POSTGRES_DB=" + dockerPGDB,
		"-p", "127.0.0.1::5432", // random host port
		"--health-cmd", fmt.Sprintf("pg_isready -U %s -d %s", dockerPGUser, dockerPGDB),
		"--health-interval", "1s",
		"--health-timeout", "3s",
		"--health-retries", "60",
		dockerPGImage,
	}

	out, err := exec.CommandContext(ctx, "docker", runArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run failed: %w\noutput: %s", err, strings.TrimSpace(string(out)))
	}

	// Resolve the randomly assigned host port.
	port, err := resolveDockerPort(ctx, name)
	if err != nil {
		return "", err
	}

	// Wait for the container to become healthy.
	if err := waitHealthy(ctx, name); err != nil {
		return "", err
	}

	return port, nil
}

// resolveDockerPort polls `docker port` until a mapping is returned.
func resolveDockerPort(ctx context.Context, name string) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.CommandContext(ctx, "docker", "port", name, "5432/tcp").Output()
		if err == nil {
			// Output format: "0.0.0.0:54321\n" or "127.0.0.1:54321\n"
			line := strings.TrimSpace(string(out))
			// Take the last line (IPv4 preferred over IPv6 dual-stack).
			lines := strings.Split(line, "\n")
			last := strings.TrimSpace(lines[len(lines)-1])
			if idx := strings.LastIndex(last, ":"); idx >= 0 {
				return last[idx+1:], nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("timed out waiting for docker port mapping for %q", name)
}

// waitHealthy polls the container health status until healthy or timeout.
func waitHealthy(ctx context.Context, name string) error {
	deadline := time.Now().Add(dockerPGHealthWait)
	for time.Now().Before(deadline) {
		out, err := exec.CommandContext(
			ctx,
			"docker", "inspect",
			"-f", "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}",
			name,
		).Output()
		if err == nil {
			status := strings.TrimSpace(string(out))
			switch status {
			case "healthy":
				return nil
			case "unhealthy":
				logs, _ := exec.CommandContext(ctx, "docker", "logs", "--tail", "20", name).CombinedOutput()
				return fmt.Errorf("PostgreSQL container %q became unhealthy\nlogs:\n%s", name, logs)
			}
		}
		time.Sleep(time.Second)
	}
	logs, _ := exec.CommandContext(ctx, "docker", "logs", "--tail", "20", name).CombinedOutput()
	return fmt.Errorf("timed out (%s) waiting for PostgreSQL container %q to become healthy\nlogs:\n%s",
		dockerPGHealthWait, name, logs)
}

// SkipIfNoPG skips the test if no PostgreSQL DSN is configured and Docker is
// unavailable.  Useful for individual tests that optionally need PG, as an
// alternative to TestMain-level auto-provisioning.
func SkipIfNoPG(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") != "" || os.Getenv("DATABASE_URL") != "" {
		return
	}
	if exec.CommandContext(context.Background(), "docker", "info").Run() == nil {
		return // Docker available; container will be started by TestMain
	}
	t.Skip("skipping: no TEST_DATABASE_URL and Docker unavailable")
}
