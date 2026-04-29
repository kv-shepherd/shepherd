package handlers

import (
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/config"
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
	mu      sync.Mutex
	buckets map[string]*loginAttemptBucket
}

type loginAttemptBucket struct {
	failures     []time.Time
	blockedUntil time.Time
}

func newLoginAttemptLimiter(cfg config.LoginRateLimit) *loginAttemptLimiter {
	normalized := normalizeLoginRateLimitConfig(cfg)
	if !normalized.Enabled {
		return nil
	}
	return &loginAttemptLimiter{
		cfg:     normalized,
		now:     time.Now,
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

func (l *loginAttemptLimiter) allow(username, clientID string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}

	keys := loginAttemptLimiterKeys(username, clientID)
	now := l.now()

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
		return false, retryAfter
	}
	return true, 0
}

func (l *loginAttemptLimiter) recordFailure(username, clientID string) {
	if l == nil {
		return
	}

	keys := loginAttemptLimiterKeys(username, clientID)
	now := l.now()

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
}

func (l *loginAttemptLimiter) recordSuccess(username, clientID string) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range loginAttemptLimiterKeys(username, clientID) {
		delete(l.buckets, key)
	}
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
	if c == nil || c.Request == nil {
		return ""
	}

	for _, raw := range []string{
		firstForwardedIP(c.GetHeader("X-Forwarded-For")),
		normalizeLoginAttemptIP(c.GetHeader("X-Real-IP")),
	} {
		if raw != "" {
			return raw
		}
	}

	host := strings.TrimSpace(c.Request.RemoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return normalizeLoginAttemptIP(host)
}

func firstForwardedIP(raw string) string {
	for _, part := range strings.Split(raw, ",") {
		if ip := normalizeLoginAttemptIP(part); ip != "" {
			return ip
		}
	}
	return ""
}

func normalizeLoginAttemptIP(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), "[]")
	if raw == "" {
		return ""
	}
	if ip := net.ParseIP(raw); ip != nil {
		return ip.String()
	}
	return strings.ToLower(raw)
}

func (s *Server) enforceLoginRateLimit(c *gin.Context, username string) bool {
	allowed, retryAfter := s.loginRateLimiter.allow(username, loginAttemptClientIdentity(c))
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
	s.loginRateLimiter.recordFailure(username, loginAttemptClientIdentity(c))
}

func (s *Server) recordLoginSuccess(c *gin.Context, username string) {
	if s == nil || s.loginRateLimiter == nil {
		return
	}
	s.loginRateLimiter.recordSuccess(username, loginAttemptClientIdentity(c))
}
