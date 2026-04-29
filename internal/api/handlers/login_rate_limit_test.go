package handlers

import (
	"testing"
	"time"

	"kv-shepherd.io/shepherd/internal/config"
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

	if allowed, _ := limiter.allow("alice", "203.0.113.10"); !allowed {
		t.Fatal("unexpected initial block")
	}

	limiter.recordFailure("alice", "203.0.113.10")
	if allowed, _ := limiter.allow("alice", "203.0.113.10"); !allowed {
		t.Fatal("unexpected block after one failure")
	}

	limiter.recordFailure("alice", "203.0.113.10")
	if allowed, retryAfter := limiter.allow("alice", "203.0.113.10"); allowed || retryAfter <= 0 {
		t.Fatalf("expected block after repeated failures, allowed=%v retry_after=%v", allowed, retryAfter)
	}

	now = now.Add(2 * time.Minute)
	if allowed, _ := limiter.allow("alice", "203.0.113.10"); !allowed {
		t.Fatal("expected block to expire")
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

	if allowed, _ := limiter.allow("alice", "203.0.113.10"); !allowed {
		t.Fatal("expected success to clear prior failure state")
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
