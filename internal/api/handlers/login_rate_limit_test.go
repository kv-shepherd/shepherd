package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestLoginAttemptLimiterBlocksAndExpires(t *testing.T) {
	t.Parallel()

	limiter := newLoginAttemptLimiter(config.LoginRateLimit{
		Enabled:       true,
		MaxFailures:   2,
		Window:        time.Minute,
		BlockDuration: 2 * time.Minute,
	})
	now := time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	if allowed, _, err := limiter.allowContext(t.Context(), "alice", "203.0.113.10"); err != nil || !allowed {
		t.Fatalf("initial allow allowed=%v err=%v, want allowed", allowed, err)
	}

	limiter.recordFailure("alice", "203.0.113.10")
	if allowed, _, err := limiter.allowContext(t.Context(), "alice", "203.0.113.10"); err != nil || !allowed {
		t.Fatalf("allow after one failure allowed=%v err=%v, want allowed", allowed, err)
	}

	limiter.recordFailure("alice", "203.0.113.10")
	if allowed, retryAfter, err := limiter.allowContext(t.Context(), "alice", "203.0.113.10"); err != nil || allowed || retryAfter <= 0 {
		t.Fatalf("expected block after repeated failures, allowed=%v retry_after=%v err=%v", allowed, retryAfter, err)
	}

	now = now.Add(2 * time.Minute)
	if allowed, _, err := limiter.allowContext(t.Context(), "alice", "203.0.113.10"); err != nil || !allowed {
		t.Fatalf("allow after expiry allowed=%v err=%v, want allowed", allowed, err)
	}
}

func TestLoginAttemptLimiterSuccessClearsBuckets(t *testing.T) {
	t.Parallel()

	limiter := newLoginAttemptLimiter(config.LoginRateLimit{
		Enabled:       true,
		MaxFailures:   2,
		Window:        time.Minute,
		BlockDuration: time.Minute,
	})

	limiter.recordFailure("alice", "203.0.113.10")
	limiter.recordSuccess("alice", "203.0.113.10")

	if allowed, _, err := limiter.allowContext(t.Context(), "alice", "203.0.113.10"); err != nil || !allowed {
		t.Fatalf("allow after success allowed=%v err=%v, want allowed", allowed, err)
	}
}

func TestPostgresLoginAttemptStoreSharesBucketsAcrossLimiters(t *testing.T) {
	t.Parallel()

	pool := testutil.OpenPGXPool(t, "login_rate_limit")
	cfg := config.LoginRateLimit{
		Enabled:       true,
		MaxFailures:   2,
		Window:        time.Minute,
		BlockDuration: 2 * time.Minute,
	}
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	limiterA := newLoginAttemptLimiterWithStore(cfg, newPostgresLoginAttemptStore(pool))
	limiterA.now = func() time.Time { return now }
	limiterB := newLoginAttemptLimiterWithStore(cfg, newPostgresLoginAttemptStore(pool))
	limiterB.now = func() time.Time { return now }

	if err := limiterA.recordFailureContext(t.Context(), "alice", "203.0.113.10"); err != nil {
		t.Fatalf("record first failure: %v", err)
	}
	if allowed, _, err := limiterB.allowContext(t.Context(), "alice", "203.0.113.10"); err != nil || !allowed {
		t.Fatalf("allow after one shared failure allowed=%v err=%v, want allowed", allowed, err)
	}

	if err := limiterB.recordFailureContext(t.Context(), "alice", "203.0.113.10"); err != nil {
		t.Fatalf("record second failure: %v", err)
	}
	if allowed, retryAfter, err := limiterA.allowContext(t.Context(), "alice", "203.0.113.10"); err != nil || allowed || retryAfter <= 0 {
		t.Fatalf("expected shared block, allowed=%v retry_after=%v err=%v", allowed, retryAfter, err)
	}

	if err := limiterA.recordSuccessContext(t.Context(), "alice", "203.0.113.10"); err != nil {
		t.Fatalf("record success: %v", err)
	}
	if allowed, _, err := limiterB.allowContext(t.Context(), "alice", "203.0.113.10"); err != nil || !allowed {
		t.Fatalf("allow after shared success clear allowed=%v err=%v, want allowed", allowed, err)
	}
}

func TestPostgresLoginAttemptStoreRetriesSchemaAfterCanceledInitialization(t *testing.T) {
	t.Parallel()

	pool := testutil.OpenPGXPool(t, "login_rate_limit_schema_retry")
	store := &postgresLoginAttemptStore{pool: pool}

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	err := store.ensureSchema(canceledCtx)
	if err == nil {
		t.Fatal("ensureSchema(canceled) error = nil, want cancellation error")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("ensureSchema(canceled) error = %v, want context cancellation", err)
	}
	if store.initialized {
		t.Fatal("store initialized after canceled schema init, want retryable failure")
	}

	if err := store.ensureSchema(t.Context()); err != nil {
		t.Fatalf("ensureSchema(retry) error = %v", err)
	}
	if !store.initialized {
		t.Fatal("store initialized = false after successful retry")
	}

	now := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	if err := store.RecordFailure(t.Context(), []string{"user:alice"}, now, time.Minute, time.Minute, 2); err != nil {
		t.Fatalf("RecordFailure() after schema retry error = %v", err)
	}
}

func TestCredentialLoginIdentityUsesProviderAndKnownPrincipalField(t *testing.T) {
	t.Parallel()

	got := credentialLoginIdentity("ldap-main", map[string]interface{}{
		"username": "Alice ",
		"password": "secret",
	})

	if want := "ldap-main:alice"; got != want {
		t.Fatalf("credentialLoginIdentity() = %q, want %q", got, want)
	}
}

func TestLoginAttemptClientIdentityUsesGinClientIP(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies(nil): %v", err)
	}
	router.POST("/auth/login", func(c *gin.Context) {
		c.String(http.StatusOK, loginAttemptClientIdentity(c))
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/login", http.NoBody)
	req.RemoteAddr = "198.51.100.10:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if got := w.Body.String(); got != "198.51.100.10" {
		t.Fatalf("loginAttemptClientIdentity() = %q, want %q", got, "198.51.100.10")
	}
}
