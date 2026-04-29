package config

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	minPasswordLength = 8
	maxPasswordBytes  = 72 // bcrypt truncates beyond 72 bytes.
)

var commonPasswordBlocklist = map[string]struct{}{
	"123456":      {},
	"12345678":    {},
	"123456789":   {},
	"admin":       {},
	"admin123":    {},
	"changeme":    {},
	"letmein":     {},
	"password":    {},
	"password1":   {},
	"password123": {},
	"qwerty":      {},
	"secret":      {},
	"welcome":     {},
}

// ValidatePassword enforces the configured password policy.
func (p PasswordPolicy) ValidatePassword(password string, identityHints ...string) error {
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("password must not be empty or whitespace")
	}
	if utf8.RuneCountInString(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	if len([]byte(password)) > maxPasswordBytes {
		return fmt.Errorf("password must not exceed %d bytes", maxPasswordBytes)
	}

	normalized := strings.ToLower(strings.TrimSpace(password))
	if _, blocked := commonPasswordBlocklist[normalized]; blocked {
		return fmt.Errorf("password is too common")
	}

	for _, hint := range identityHints {
		hint = normalizePasswordIdentityHint(hint)
		if hint == "" {
			continue
		}
		if normalized == hint {
			return fmt.Errorf("password must not match account identifiers")
		}
	}

	if strings.EqualFold(strings.TrimSpace(p.Mode), "legacy") {
		var hasUpper, hasLower, hasDigit, hasSpecial bool
		for _, r := range password {
			switch {
			case unicode.IsUpper(r):
				hasUpper = true
			case unicode.IsLower(r):
				hasLower = true
			case unicode.IsDigit(r):
				hasDigit = true
			case unicode.IsPunct(r) || unicode.IsSymbol(r):
				hasSpecial = true
			}
		}

		switch {
		case p.RequireUppercase && !hasUpper:
			return fmt.Errorf("password must include an uppercase letter")
		case p.RequireLowercase && !hasLower:
			return fmt.Errorf("password must include a lowercase letter")
		case p.RequireDigit && !hasDigit:
			return fmt.Errorf("password must include a digit")
		case p.RequireSpecial && !hasSpecial:
			return fmt.Errorf("password must include a special character")
		}
	}

	return nil
}

func normalizePasswordIdentityHint(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	if local, _, ok := strings.Cut(raw, "@"); ok {
		if strings.TrimSpace(local) != "" {
			return strings.TrimSpace(local)
		}
	}
	return raw
}
