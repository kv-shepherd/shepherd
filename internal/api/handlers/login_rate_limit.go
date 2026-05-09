package handlers

import (
	"context"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	defaultLoginRateLimitMaxFailures   = 5
	defaultLoginRateLimitWindow        = 15 * time.Minute
	defaultLoginRateLimitBlockDuration = 15 * time.Minute
	loginRateLimitedErrorCode          = "LOGIN_RATE_LIMITED"
)

type loginAttemptLimiter struct {
	cfg     config.LoginRateLimit
	now     func() time.Time
	store   loginAttemptStore
	mu      sync.Mutex
	buckets map[string]*loginAttemptBucket
}

type loginAttemptStore interface {
	Allow(ctx context.Context, keys []string, now time.Time, window time.Duration) (time.Duration, error)
	RecordFailure(ctx context.Context, keys []string, now time.Time, window, blockDuration time.Duration, maxFailures int) error
	RecordSuccess(ctx context.Context, keys []string) error
}

type loginAttemptBucket struct {
	failures     []time.Time
	blockedUntil time.Time
}

func newLoginAttemptLimiter(cfg config.LoginRateLimit) *loginAttemptLimiter {
	return newLoginAttemptLimiterWithStore(cfg, nil)
}

func newLoginAttemptLimiterWithStore(cfg config.LoginRateLimit, store loginAttemptStore) *loginAttemptLimiter {
	normalized := normalizeLoginRateLimitConfig(cfg)
	if !normalized.Enabled {
		return nil
	}
	return &loginAttemptLimiter{
		cfg:     normalized,
		now:     time.Now,
		store:   store,
		buckets: make(map[string]*loginAttemptBucket),
	}
}

func normalizeLoginRateLimitConfig(cfg config.LoginRateLimit) config.LoginRateLimit {
	if cfg == (config.LoginRateLimit{}) {
		return config.LoginRateLimit{
			Enabled:       true,
			MaxFailures:   defaultLoginRateLimitMaxFailures,
			Window:        defaultLoginRateLimitWindow,
			BlockDuration: defaultLoginRateLimitBlockDuration,
		}
	}
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = defaultLoginRateLimitMaxFailures
	}
	if cfg.Window <= 0 {
		cfg.Window = defaultLoginRateLimitWindow
	}
	if cfg.BlockDuration <= 0 {
		cfg.BlockDuration = defaultLoginRateLimitBlockDuration
	}
	return cfg
}

func (l *loginAttemptLimiter) allowContext(ctx context.Context, username, clientID string) (bool, time.Duration, error) {
	if l == nil {
		return true, 0, nil
	}

	keys := loginAttemptLimiterKeys(username, clientID)
	now := l.now()
	if l.store != nil {
		retryAfter, err := l.store.Allow(ctx, keys, now, l.cfg.Window)
		if err != nil {
			return false, 0, err
		}
		if retryAfter > 0 {
			return false, retryAfter, nil
		}
		return true, 0, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	var retryAfter time.Duration
	for _, key := range keys {
		bucket := l.buckets[key]
		if bucket == nil {
			continue
		}
		l.pruneBucketLocked(bucket, now)
		if bucket.blockedUntil.After(now) {
			remaining := bucket.blockedUntil.Sub(now)
			if remaining > retryAfter {
				retryAfter = remaining
			}
		}
	}

	if retryAfter > 0 {
		return false, retryAfter, nil
	}
	return true, 0, nil
}

func (l *loginAttemptLimiter) recordFailure(username, clientID string) {
	_ = l.recordFailureContext(context.Background(), username, clientID)
}

func (l *loginAttemptLimiter) recordFailureContext(ctx context.Context, username, clientID string) error {
	if l == nil {
		return nil
	}

	keys := loginAttemptLimiterKeys(username, clientID)
	now := l.now()
	if l.store != nil {
		return l.store.RecordFailure(ctx, keys, now, l.cfg.Window, l.cfg.BlockDuration, l.cfg.MaxFailures)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range keys {
		bucket := l.bucketLocked(key)
		l.pruneBucketLocked(bucket, now)
		bucket.failures = append(bucket.failures, now)
		if len(bucket.failures) >= l.cfg.MaxFailures {
			bucket.blockedUntil = now.Add(l.cfg.BlockDuration)
		}
	}
	return nil
}

func (l *loginAttemptLimiter) recordSuccess(username, clientID string) {
	_ = l.recordSuccessContext(context.Background(), username, clientID)
}

func (l *loginAttemptLimiter) recordSuccessContext(ctx context.Context, username, clientID string) error {
	if l == nil {
		return nil
	}

	keys := loginAttemptLimiterKeys(username, clientID)
	if l.store != nil {
		return l.store.RecordSuccess(ctx, keys)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range keys {
		delete(l.buckets, key)
	}
	return nil
}

func (l *loginAttemptLimiter) bucketLocked(key string) *loginAttemptBucket {
	bucket := l.buckets[key]
	if bucket != nil {
		return bucket
	}
	bucket = &loginAttemptBucket{}
	l.buckets[key] = bucket
	return bucket
}

func (l *loginAttemptLimiter) pruneBucketLocked(bucket *loginAttemptBucket, now time.Time) {
	if bucket == nil {
		return
	}
	if len(bucket.failures) > 0 {
		kept := bucket.failures[:0]
		for _, ts := range bucket.failures {
			if now.Sub(ts) <= l.cfg.Window {
				kept = append(kept, ts)
			}
		}
		bucket.failures = kept
	}
	if bucket.blockedUntil.Before(now) || bucket.blockedUntil.Equal(now) {
		bucket.blockedUntil = time.Time{}
	}
}

func loginAttemptLimiterKeys(username, clientID string) []string {
	userKey := normalizeLoginAttemptIdentity(username)
	clientKey := normalizeLoginAttemptIdentity(clientID)

	keys := make([]string, 0, 3)
	if clientKey != "" {
		keys = append(keys, "client:"+clientKey)
	}
	if userKey != "" {
		keys = append(keys, "user:"+userKey)
	}
	if clientKey != "" && userKey != "" {
		keys = append(keys, "combo:"+clientKey+"|"+userKey)
	}
	if len(keys) == 0 {
		return []string{"anonymous"}
	}
	return keys
}

func normalizeLoginAttemptIdentity(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	return value
}

func credentialLoginIdentity(providerID string, credentials map[string]interface{}) string {
	identity := normalizeLoginAttemptIdentity(providerID)
	for _, key := range []string{"username", "user", "email", "login", "account"} {
		value, ok := credentials[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = normalizeLoginAttemptIdentity(text)
		if text == "" {
			continue
		}
		if identity == "" {
			return text
		}
		return identity + ":" + text
	}
	return identity
}

func loginAttemptClientIdentity(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return normalizeLoginAttemptIdentity(c.ClientIP())
}

func (s *Server) enforceLoginRateLimit(c *gin.Context, username string) bool {
	allowed, retryAfter, err := s.loginRateLimiter.allowContext(c.Request.Context(), username, loginAttemptClientIdentity(c))
	if err != nil {
		c.JSON(500, generated.Error{
			Code:    "INTERNAL_ERROR",
			Message: "login rate limit is unavailable",
		})
		return false
	}
	if allowed {
		return true
	}

	retrySeconds := int(math.Ceil(retryAfter.Seconds()))
	if retrySeconds < 1 {
		retrySeconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(retrySeconds))
	c.JSON(429, generated.Error{
		Code:    loginRateLimitedErrorCode,
		Message: "too many login attempts; try again later",
	})
	return false
}

func (s *Server) recordLoginFailure(c *gin.Context, username string) {
	if s == nil || s.loginRateLimiter == nil {
		return
	}
	if err := s.loginRateLimiter.recordFailureContext(c.Request.Context(), username, loginAttemptClientIdentity(c)); err != nil {
		logger.Warn("failed to record login rate limit failure", zap.Error(err))
		return
	}
}

func (s *Server) recordLoginSuccess(c *gin.Context, username string) {
	if s == nil || s.loginRateLimiter == nil {
		return
	}
	if err := s.loginRateLimiter.recordSuccessContext(c.Request.Context(), username, loginAttemptClientIdentity(c)); err != nil {
		logger.Warn("failed to clear login rate limit state", zap.Error(err))
		return
	}
}
