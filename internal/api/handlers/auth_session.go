package handlers

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultAuthSessionCookieName = "shepherd_session"

func (s *Server) authSessionCookieName() string {
	if s == nil {
		return defaultAuthSessionCookieName
	}
	if name := strings.TrimSpace(s.sessionCfg.Cookie); name != "" {
		return name
	}
	return defaultAuthSessionCookieName
}

func (s *Server) buildAuthSessionCookie(c *gin.Context, value string, expiresAt time.Time, maxAge int) *http.Cookie {
	httpOnly := true
	secure := isSecureRequest(c)
	if s != nil {
		if s.sessionCfg.HTTPOnly || s.sessionCfg.Cookie != "" || s.sessionCfg.Secure {
			httpOnly = s.sessionCfg.HTTPOnly
		}
		secure = secureCookieByPolicy(c, s.sessionCfg.Secure, s.publicBaseURL)
	}
	return &http.Cookie{
		Name:     s.authSessionCookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  expiresAt,
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func secureCookieByPolicy(c *gin.Context, configuredSecure bool, publicBaseURL string) bool {
	return secureCookieByPolicyWithReleaseMode(
		c,
		configuredSecure,
		publicBaseURL,
		strings.EqualFold(strings.TrimSpace(os.Getenv("GIN_MODE")), "release"),
	)
}

func secureCookieByPolicyWithReleaseMode(c *gin.Context, configuredSecure bool, publicBaseURL string, releaseMode bool) bool {
	if !configuredSecure {
		return false
	}
	if isSecureRequest(c) {
		return true
	}
	if publicBaseURLUsesHTTPS(publicBaseURL) {
		return true
	}
	return releaseMode
}

func publicBaseURLUsesHTTPS(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(parsed.Scheme, "https")
}

func (s *Server) setAuthSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	if c == nil {
		return
	}
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(c.Writer, s.buildAuthSessionCookie(c, token, expiresAt, maxAge))
}

func (s *Server) clearAuthSessionCookie(c *gin.Context) {
	if c == nil {
		return
	}
	expiredAt := time.Unix(0, 0).UTC()
	http.SetCookie(c.Writer, s.buildAuthSessionCookie(c, "", expiredAt, -1))
}
