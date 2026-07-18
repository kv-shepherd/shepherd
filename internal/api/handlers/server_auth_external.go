package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/authprovider"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/edge/authworkspace/runtimeview"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
	"kv-shepherd.io/shepherd/internal/service"
)

const externalAuthStateTTL = 5 * time.Minute
const externalAuthStateIssuerSuffix = "/external-auth/v2"
const externalAuthBridgeMessageType = "shepherd.external_auth.complete"
const externalAuthSchemeHTTP = "http"
const externalAuthSchemeHTTPS = "https"
const loginFailureCode = "INVALID_CREDENTIALS"
const externalAuthFailureCode = "EXTERNAL_AUTH_FAILED"
const externalAuthFailureMessage = "external authentication failed"

var (
	errExternalAuthUserDisabled        = errors.New("external auth user disabled")
	errExternalAuthProviderUnavailable = errors.New("external auth provider unavailable")
	errExternalAuthIdentityChanged     = errors.New("external auth identity changed before token issuance")
)

type externalAuthFailureLogFunc func(string, ...zap.Field)

func (s *Server) logExternalAuthProviderFailure(
	operation, failureClass, providerID string,
	err error,
) {
	errorType := "<nil>"
	if err != nil {
		errorType = fmt.Sprintf("%T", err)
	}
	fields := []zap.Field{
		zap.String("operation", strings.TrimSpace(operation)),
		zap.String("failure_class", strings.TrimSpace(failureClass)),
		zap.String("provider_id", strings.TrimSpace(providerID)),
		zap.String("error_type", errorType),
	}
	const message = "external auth provider operation failed"
	if s != nil && s.externalAuthFailureLog != nil {
		s.externalAuthFailureLog(message, fields...)
		return
	}
	logger.Warn(message, fields...)
}

type externalAuthStateClaims struct {
	ProviderID         string `json:"provider_id"`
	ProviderGeneration string `json:"provider_generation"`
	ReturnTo           string `json:"return_to"`
	LoginMode          string `json:"login_mode,omitempty"`
	jwt.RegisteredClaims
}

func (c externalAuthStateClaims) Validate() error {
	switch {
	case strings.TrimSpace(c.ProviderID) == "":
		return fmt.Errorf("provider_id is required")
	case strings.TrimSpace(c.ProviderGeneration) == "":
		return fmt.Errorf("provider_generation is required")
	case strings.TrimSpace(c.ReturnTo) == "":
		return fmt.Errorf("return_to is required")
	}
	return nil
}

type externalAuthCallbackPayload struct {
	Type                string `json:"type"`
	Success             bool   `json:"success"`
	ForcePasswordChange bool   `json:"force_password_change,omitempty"`
	Code                string `json:"code,omitempty"`
	ReturnTo            string `json:"return_to,omitempty"`
}

func (s *Server) ListLoginAuthProviders(c *gin.Context) {
	ctx := c.Request.Context()

	rows, err := s.client.AuthProvider.Query().
		Where(authprovider.EnabledEQ(true)).
		Order(ent.Asc(authprovider.FieldSortOrder), ent.Asc(authprovider.FieldName)).
		All(ctx)
	if err != nil {
		logger.Error("failed to list login auth providers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	items := make([]generated.LoginAuthProvider, 0, len(rows))
	for _, row := range rows {
		item, supported := runtimeview.BuildLoginProvider(row)
		if !supported {
			continue
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, generated.LoginAuthProviderList{Items: items})
}

func (s *Server) SubmitLoginAuthProvider(c *gin.Context, providerID generated.ProviderID) {
	ctx := c.Request.Context()

	var req generated.AuthProviderCredentialLoginRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}
	loginIdentity := credentialLoginIdentity(providerID, req.Credentials)
	if !s.enforceLoginRateLimit(c, loginIdentity) {
		return
	}

	providerRow, adapter, ok := s.resolveLoginAuthProviderAdapter(ctx, c, providerID)
	if !ok {
		return
	}
	credentialCapability, ok := adapter.(runtimecontract.AuthCredentialCapability)
	if !ok || credentialCapability == nil {
		c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
		return
	}
	providerGeneration, generationErr := service.CaptureAuthProviderGeneration(providerRow)
	if generationErr != nil {
		logger.Error("failed to capture auth provider generation for credential login", zap.Error(generationErr), zap.String("provider_id", providerRow.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(providerRow.AuthType, providerRow.Config)
	if cfgErr != nil {
		logger.Error("failed to decrypt auth provider config for credential login", zap.Error(cfgErr), zap.String("provider_id", providerRow.ID))
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "AUTH_PROVIDER_CONFIG_INVALID",
			Message: "auth provider runtime configuration is invalid; recreate or update this provider",
		})
		return
	}

	authResult, err := credentialCapability.AuthenticateCredentials(ctx, runtimeConfig, runtimecontract.AuthCredentialRequest{
		LoginMode:   strings.TrimSpace(req.LoginMode),
		Credentials: cloneCredentialAttributes(req.Credentials),
		UserAgent:   strings.TrimSpace(c.Request.UserAgent()),
	})
	if err != nil {
		var credentialErr *runtimecontract.AuthCredentialError
		if errors.As(err, &credentialErr) && strings.TrimSpace(credentialErr.Code) != "" {
			s.logExternalAuthProviderFailure("credential_authenticate", "public_provider_error", providerRow.ID, err)
			status := http.StatusBadRequest
			if credentialErr.Code == loginFailureCode {
				status = http.StatusUnauthorized
				s.recordLoginFailure(c, loginIdentity)
			}
			c.JSON(status, generated.Error{
				Code:    credentialErr.Code,
				Message: strings.TrimSpace(credentialErr.Message),
			})
			return
		}
		s.logExternalAuthProviderFailure("credential_authenticate", "provider_operation_failed", providerRow.ID, err)
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    externalAuthFailureCode,
			Message: externalAuthFailureMessage,
		})
		return
	}

	loginResp, err := s.completeExternalAuthResultLogin(ctx, c, providerRow.ID, providerGeneration, authResult)
	if err != nil {
		switch {
		case errors.Is(err, errExternalAuthUserDisabled):
			c.JSON(http.StatusForbidden, generated.Error{Code: "USER_DISABLED"})
		case errors.Is(err, service.ErrAuthProviderGenerationChanged),
			errors.Is(err, errExternalAuthIdentityChanged):
			c.JSON(http.StatusConflict, generated.Error{
				Code:    "AUTH_PROVIDER_CHANGED",
				Message: "authentication provider changed; authenticate again",
			})
		case errors.Is(err, errExternalAuthProviderUnavailable):
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
		case strings.Contains(err.Error(), "already belongs to another user"):
			c.JSON(http.StatusConflict, generated.Error{Code: "EXTERNAL_IDENTITY_CONFLICT"})
		default:
			logger.Error("failed to complete credential auth login", zap.Error(err), zap.String("provider_id", providerRow.ID))
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		}
		return
	}

	s.recordLoginSuccess(c, loginIdentity)
	s.setAuthSessionCookie(c, loginResp.Token, loginResp.ExpiresAt)
	clientResp := loginResponseForClient(c, loginResp)
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, clientResp)
}

func (s *Server) StartLoginAuthProvider(c *gin.Context, providerID generated.ProviderID) {
	ctx := c.Request.Context()
	allowedOrigins := s.effectiveExternalAuthAllowedOrigins()

	var req generated.AuthProviderLoginStartRequest
	if !bindAndValidateJSON(c, &req) {
		return
	}

	returnTo, err := s.validateExternalAuthReturnToForStart(c, req.ReturnTo, allowedOrigins)
	if err != nil {
		logger.Warn(
			"external auth start rejected invalid return_to",
			zap.Error(err),
			zap.String("provider_id", providerID),
			zap.String("return_to", req.ReturnTo),
			zap.String("request_origin", externalAuthRequestOrigin(c)),
			zap.Strings("allowed_origins", allowedOrigins),
		)
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST", Message: err.Error()})
		return
	}

	providerRow, adapter, ok := s.resolveLoginAuthProviderAdapter(ctx, c, providerID)
	if !ok {
		return
	}
	runtimeCapability, ok := adapter.(runtimecontract.AuthRuntimeCapability)
	if !ok || runtimeCapability == nil {
		c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
		return
	}
	providerGeneration, generationErr := service.CaptureAuthProviderGeneration(providerRow)
	if generationErr != nil {
		logger.Error("failed to capture auth provider generation for login start", zap.Error(generationErr), zap.String("provider_id", providerRow.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	state, err := s.issueExternalAuthState(providerRow.ID, returnTo.String(), req.LoginMode, providerGeneration)
	if err != nil {
		logger.Error("failed to issue external auth state", zap.Error(err), zap.String("provider_id", providerRow.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	callbackURL := s.externalAuthCallbackURL(providerRow.ID)
	if strings.TrimSpace(callbackURL) == "" {
		logger.Error("external auth runtime login requires configured public base url", zap.String("provider_id", providerRow.ID))
		c.JSON(http.StatusServiceUnavailable, generated.Error{
			Code:    "EXTERNAL_AUTH_PUBLIC_BASE_URL_REQUIRED",
			Message: "external auth runtime login requires a configured public base URL",
		})
		return
	}
	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(providerRow.AuthType, providerRow.Config)
	if cfgErr != nil {
		logger.Error("failed to decrypt auth provider config for runtime login", zap.Error(cfgErr), zap.String("provider_id", providerRow.ID))
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "AUTH_PROVIDER_CONFIG_INVALID",
			Message: "auth provider runtime configuration is invalid; recreate or update this provider",
		})
		return
	}

	startResp, err := runtimeCapability.StartLogin(ctx, runtimeConfig, runtimecontract.AuthStartRequest{
		LoginMode:   strings.TrimSpace(req.LoginMode),
		ReturnTo:    returnTo.String(),
		CallbackURL: callbackURL,
		State:       state,
		UserAgent:   strings.TrimSpace(c.Request.UserAgent()),
	})
	if err != nil {
		var startErr *runtimecontract.AuthStartError
		if errors.As(err, &startErr) && strings.TrimSpace(startErr.Code) != "" {
			s.logExternalAuthProviderFailure("login_start", "public_provider_error", providerRow.ID, err)
			c.JSON(http.StatusBadRequest, generated.Error{
				Code:    startErr.Code,
				Message: strings.TrimSpace(startErr.Message),
			})
			return
		}
		s.logExternalAuthProviderFailure("login_start", "provider_operation_failed", providerRow.ID, err)
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    externalAuthFailureCode,
			Message: externalAuthFailureMessage,
		})
		return
	}
	if strings.TrimSpace(startResp.RedirectURL) == "" {
		logger.Error("runtime auth provider returned empty redirect_url", zap.String("provider_id", providerRow.ID))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, generated.AuthProviderLoginStartResponse{
		RedirectUrl: startResp.RedirectURL,
	})
}

func (s *Server) CompleteLoginAuthProviderGet(c *gin.Context, providerID generated.ProviderID, params generated.CompleteLoginAuthProviderGetParams) {
	callbackReq := runtimecontract.AuthCallbackRequest{
		Method:      c.Request.Method,
		Query:       map[string][]string(c.Request.URL.Query()),
		Header:      sanitizedAuthCallbackHeaders(c.Request.Header),
		RemoteAddr:  strings.TrimSpace(c.Request.RemoteAddr),
		CallbackURL: s.externalAuthCallbackURL(providerID),
	}
	if strings.TrimSpace(params.Code) != "" || strings.TrimSpace(params.State) != "" {
		callbackReq.Query["code"] = []string{params.Code}
		callbackReq.Query["state"] = []string{params.State}
	}
	s.completeExternalAuthLogin(c, providerID, callbackReq)
}

func (s *Server) CompleteLoginAuthProviderPost(c *gin.Context, providerID generated.ProviderID) {
	form := make(map[string][]string)
	if parseErr := c.Request.ParseForm(); parseErr == nil {
		form = map[string][]string(c.Request.PostForm)
	}
	callbackReq := runtimecontract.AuthCallbackRequest{
		Method:      c.Request.Method,
		Query:       map[string][]string(c.Request.URL.Query()),
		Form:        form,
		Header:      sanitizedAuthCallbackHeaders(c.Request.Header),
		RemoteAddr:  strings.TrimSpace(c.Request.RemoteAddr),
		CallbackURL: s.externalAuthCallbackURL(providerID),
	}
	s.completeExternalAuthLogin(c, providerID, callbackReq)
}

func sanitizedAuthCallbackHeaders(header http.Header) map[string][]string {
	if len(header) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"accept":          {},
		"accept-language": {},
		"content-type":    {},
		"user-agent":      {},
		"x-request-id":    {},
	}
	out := make(map[string][]string)
	for name, values := range header {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if _, ok := allowed[normalized]; !ok {
			continue
		}
		copied := make([]string, 0, len(values))
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				copied = append(copied, value)
			}
		}
		if len(copied) > 0 {
			out[http.CanonicalHeaderKey(normalized)] = copied
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Server) completeExternalAuthLogin(c *gin.Context, providerID string, callbackReq runtimecontract.AuthCallbackRequest) {
	ctx := c.Request.Context()
	state := firstExternalAuthCallbackValue(callbackReq, "state")
	if state == "" {
		s.renderExternalAuthBridge(c, http.StatusBadRequest, "", "", externalAuthCallbackPayload{
			Type:    externalAuthBridgeMessageType,
			Success: false,
			Code:    "INVALID_REQUEST",
		})
		return
	}

	stateClaims, err := s.validateExternalAuthState(state, providerID)
	if err != nil {
		logger.Warn("external auth callback rejected by state validation", zap.Error(err), zap.String("provider_id", providerID))
		s.renderExternalAuthBridge(c, http.StatusBadRequest, "", "", externalAuthCallbackPayload{
			Type:    externalAuthBridgeMessageType,
			Success: false,
			Code:    "INVALID_STATE",
		})
		return
	}

	providerRow, providerErr := s.loadEnabledLoginAuthProvider(ctx, providerID)
	if providerErr != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if ent.IsNotFound(providerErr) {
			status = http.StatusNotFound
			code = "AUTH_PROVIDER_NOT_FOUND"
		} else {
			logger.Error("failed to query auth provider for callback", zap.Error(providerErr), zap.String("provider_id", providerID))
		}
		s.renderExternalAuthBridge(c, status, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
			Type:     externalAuthBridgeMessageType,
			Success:  false,
			Code:     code,
			ReturnTo: stateClaims.ReturnTo,
		})
		return
	}
	providerGeneration, generationErr := service.CaptureAuthProviderGeneration(providerRow)
	if generationErr != nil {
		logger.Error("failed to capture auth provider generation for callback", zap.Error(generationErr), zap.String("provider_id", providerID))
		s.renderExternalAuthBridge(c, http.StatusInternalServerError, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
			Type:     externalAuthBridgeMessageType,
			Success:  false,
			Code:     "INTERNAL_ERROR",
			ReturnTo: stateClaims.ReturnTo,
		})
		return
	}
	if bindingErr := providerGeneration.ValidateStateBinding(s.jwtCfg.SigningKey, stateClaims.ProviderGeneration); bindingErr != nil {
		logger.Warn("external auth callback rejected stale provider generation", zap.Error(bindingErr), zap.String("provider_id", providerID))
		s.renderExternalAuthBridge(c, http.StatusConflict, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
			Type:     externalAuthBridgeMessageType,
			Success:  false,
			Code:     "AUTH_PROVIDER_CHANGED",
			ReturnTo: stateClaims.ReturnTo,
		})
		return
	}

	adapter := adminglobal.Resolve(providerRow.AuthType)
	runtimeCapability, ok := adapter.(runtimecontract.AuthRuntimeCapability)
	if !ok || runtimeCapability == nil {
		s.renderExternalAuthBridge(c, http.StatusNotFound, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
			Type:     externalAuthBridgeMessageType,
			Success:  false,
			Code:     "AUTH_PROVIDER_NOT_FOUND",
			ReturnTo: stateClaims.ReturnTo,
		})
		return
	}

	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(providerRow.AuthType, providerRow.Config)
	if cfgErr != nil {
		logger.Error("failed to decrypt auth provider config for callback", zap.Error(cfgErr), zap.String("provider_id", providerID))
		s.renderExternalAuthBridge(c, http.StatusInternalServerError, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
			Type:     externalAuthBridgeMessageType,
			Success:  false,
			Code:     "INTERNAL_ERROR",
			ReturnTo: stateClaims.ReturnTo,
		})
		return
	}

	authResult, err := runtimeCapability.CompleteLogin(ctx, runtimeConfig, callbackReq)
	if err != nil {
		s.logExternalAuthProviderFailure("login_callback", "provider_operation_failed", providerID, err)
		s.renderExternalAuthBridge(c, http.StatusBadRequest, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
			Type:     externalAuthBridgeMessageType,
			Success:  false,
			Code:     externalAuthFailureCode,
			ReturnTo: stateClaims.ReturnTo,
		})
		return
	}

	loginResp, upsertResult, txErr := s.finalizeExternalAuthLogin(ctx, providerID, providerGeneration, authResult)
	if txErr != nil {
		switch {
		case errors.Is(txErr, errExternalAuthUserDisabled):
			s.renderExternalAuthBridge(c, http.StatusForbidden, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
				Type:     externalAuthBridgeMessageType,
				Success:  false,
				Code:     "USER_DISABLED",
				ReturnTo: stateClaims.ReturnTo,
			})
			return
		case errors.Is(txErr, service.ErrAuthProviderGenerationChanged),
			errors.Is(txErr, errExternalAuthIdentityChanged):
			s.renderExternalAuthBridge(c, http.StatusConflict, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
				Type:     externalAuthBridgeMessageType,
				Success:  false,
				Code:     "AUTH_PROVIDER_CHANGED",
				ReturnTo: stateClaims.ReturnTo,
			})
			return
		case errors.Is(txErr, errExternalAuthProviderUnavailable):
			s.renderExternalAuthBridge(c, http.StatusNotFound, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
				Type:     externalAuthBridgeMessageType,
				Success:  false,
				Code:     "AUTH_PROVIDER_NOT_FOUND",
				ReturnTo: stateClaims.ReturnTo,
			})
			return
		case strings.Contains(txErr.Error(), "already belongs to another user"):
			logger.Warn("external auth user provisioning failed", zap.Error(txErr), zap.String("provider_id", providerID))
			s.renderExternalAuthBridge(c, http.StatusConflict, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
				Type:     externalAuthBridgeMessageType,
				Success:  false,
				Code:     "EXTERNAL_IDENTITY_CONFLICT",
				ReturnTo: stateClaims.ReturnTo,
			})
			return
		default:
			logger.Error("failed to complete external login transaction", zap.Error(txErr), zap.String("provider_id", providerID))
			s.renderExternalAuthBridge(c, http.StatusInternalServerError, stateClaims.ReturnTo, stateClaims.ReturnTo, externalAuthCallbackPayload{
				Type:     externalAuthBridgeMessageType,
				Success:  false,
				Code:     "INTERNAL_ERROR",
				ReturnTo: stateClaims.ReturnTo,
			})
			return
		}
	}

	if s.audit != nil {
		clientIP, requestID := loginAuditContext(c)
		_ = s.audit.LogAction(ctx, "user.external_login", "user", upsertResult.User.ID, upsertResult.User.ID, map[string]interface{}{
			"auth_provider_id": providerID,
			"created":          upsertResult.Created,
			"updated":          upsertResult.Updated,
			"provider":         "external",
			"client_ip":        clientIP,
			"request_id":       requestID,
		})
	}

	s.setAuthSessionCookie(c, loginResp.Token, loginResp.ExpiresAt)
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Location", externalAuthSuccessRedirectTarget(stateClaims.ReturnTo, loginResp.ForcePasswordChange))
	c.Status(http.StatusSeeOther)
	c.Writer.WriteHeaderNow()
}

func (s *Server) resolveLoginAuthProviderAdapter(
	ctx context.Context,
	c *gin.Context,
	providerID generated.ProviderID,
) (*ent.AuthProvider, admincontract.AuthProviderAdminAdapter, bool) {
	providerRow, err := s.loadEnabledLoginAuthProvider(ctx, providerID)
	if err != nil {
		if ent.IsNotFound(err) {
			if c != nil {
				c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
			}
			return nil, nil, false
		}
		logger.Error("failed to query auth provider for runtime auth", zap.Error(err), zap.String("provider_id", providerID))
		if c != nil {
			c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		}
		return nil, nil, false
	}

	adapter := adminglobal.Resolve(providerRow.AuthType)
	if adapter == nil {
		if c != nil {
			c.JSON(http.StatusNotFound, generated.Error{Code: "AUTH_PROVIDER_NOT_FOUND"})
		}
		return nil, nil, false
	}
	return providerRow, adapter, true
}

func (s *Server) loadEnabledLoginAuthProvider(ctx context.Context, providerID string) (*ent.AuthProvider, error) {
	return s.client.AuthProvider.Query().
		Where(
			authprovider.IDEQ(providerID),
			authprovider.EnabledEQ(true),
		).
		Only(ctx)
}

func (s *Server) issueExternalAuthState(
	providerID, returnTo, loginMode string,
	providerGeneration service.AuthProviderGeneration,
) (string, error) {
	tokenID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate external auth state id: %w", err)
	}
	generationBinding, err := providerGeneration.StateBinding(s.jwtCfg.SigningKey)
	if err != nil {
		return "", fmt.Errorf("bind external auth state to provider generation: %w", err)
	}
	now := time.Now().UTC()
	claims := externalAuthStateClaims{
		ProviderID:         providerID,
		ProviderGeneration: generationBinding,
		ReturnTo:           returnTo,
		LoginMode:          strings.TrimSpace(loginMode),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.jwtCfg.Issuer + externalAuthStateIssuerSuffix,
			Subject:   providerID,
			ExpiresAt: jwt.NewNumericDate(now.Add(externalAuthStateTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        tokenID.String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtCfg.SigningKey)
	if err != nil {
		return "", fmt.Errorf("sign external auth state: %w", err)
	}
	return signed, nil
}

func (s *Server) validateExternalAuthState(tokenString, providerID string) (*externalAuthStateClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &externalAuthStateClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtCfg.SigningKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(s.jwtCfg.Issuer+externalAuthStateIssuerSuffix), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*externalAuthStateClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	if strings.TrimSpace(claims.ProviderID) != strings.TrimSpace(providerID) {
		return nil, fmt.Errorf("provider_id mismatch")
	}
	if _, err := parseExternalAuthAbsoluteReturnTo(claims.ReturnTo); err != nil {
		return nil, err
	}
	return claims, nil
}

func parseExternalAuthReturnTo(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("return_to is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("return_to must be a valid URL")
	}
	if parsed.Scheme != "" || parsed.Host != "" {
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("return_to must be a valid URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("return_to must use http or https")
		}
		return parsed, nil
	}

	if strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "//") {
		return parsed, nil
	}

	return nil, fmt.Errorf("return_to must be an absolute URL or an absolute path")
}

func parseExternalAuthAbsoluteReturnTo(raw string) (*url.URL, error) {
	parsed, err := parseExternalAuthReturnTo(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("return_to must be an absolute URL")
	}
	return parsed, nil
}

func (s *Server) IsAllowedOrigin(ctx context.Context, origin string) bool {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	allowedOrigins := s.effectiveExternalAuthAllowedOrigins()
	return externalAuthOriginAllowed(parsed, allowedOrigins)
}

func (s *Server) IsAllowedRequestOrigin(ctx context.Context, path, origin string) bool {
	if s.IsAllowedOrigin(ctx, origin) {
		return true
	}
	providerID, ok := externalAuthCallbackProviderIDFromPath(path)
	if !ok {
		return false
	}
	return s.isAllowedExternalAuthCallbackOrigin(ctx, providerID, origin)
}

func externalAuthCallbackProviderIDFromPath(path string) (string, bool) {
	const prefix = "/api/v1/auth/providers/"
	const suffix = "/callback"
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	rawProviderID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if rawProviderID == "" || strings.Contains(rawProviderID, "/") {
		return "", false
	}
	providerID, err := url.PathUnescape(rawProviderID)
	if err != nil || strings.TrimSpace(providerID) == "" || strings.Contains(providerID, "/") {
		return "", false
	}
	return providerID, true
}

func (s *Server) isAllowedExternalAuthCallbackOrigin(ctx context.Context, providerID, origin string) bool {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if s == nil || s.client == nil || s.authProviderConfig == nil {
		return false
	}

	providerRow, adapter, ok := s.resolveLoginAuthProviderAdapter(ctx, nil, providerID)
	if !ok {
		return false
	}
	callbackOrigins, ok := adapter.(runtimecontract.AuthCallbackOriginDescriber)
	if !ok || callbackOrigins == nil {
		return false
	}
	runtimeConfig, cfgErr := s.authProviderConfig.DecryptForUse(providerRow.AuthType, providerRow.Config)
	if cfgErr != nil {
		logger.Warn("failed to decrypt auth provider config for callback origin check", zap.Error(cfgErr), zap.String("provider_id", providerID))
		return false
	}
	for _, allowedOrigin := range callbackOrigins.AllowedCallbackOrigins(runtimeConfig) {
		if sameExternalAuthOrigin(origin, allowedOrigin) {
			return true
		}
	}
	return false
}

func (s *Server) validateExternalAuthReturnToForStart(c *gin.Context, raw string, allowedOrigins []string) (*url.URL, error) {
	parsed, err := parseExternalAuthReturnTo(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		base, resolveErr := s.resolveExternalAuthReturnToBase(c, allowedOrigins)
		if resolveErr != nil {
			return nil, resolveErr
		}
		parsed = base.ResolveReference(parsed)
	}
	if err := validateExternalAuthReturnToOrigin(parsed, allowedOrigins); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (s *Server) resolveExternalAuthReturnToBase(c *gin.Context, allowedOrigins []string) (*url.URL, error) {
	if origin := externalAuthRequestOrigin(c); origin != "" {
		parsed, err := url.Parse(origin)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			if len(allowedOrigins) == 0 || externalAuthOriginAllowed(parsed, allowedOrigins) {
				return &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}, nil
			}
		}
	}

	if len(allowedOrigins) == 1 {
		parsed, err := url.Parse(strings.TrimSpace(allowedOrigins[0]))
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}, nil
		}
	}

	return nil, fmt.Errorf("relative return_to requires an allowed request origin")
}

func validateExternalAuthReturnToOrigin(parsed *url.URL, allowedOrigins []string) error {
	if len(allowedOrigins) == 0 {
		return nil
	}
	if externalAuthOriginAllowed(parsed, allowedOrigins) {
		return nil
	}
	return fmt.Errorf("return_to origin is not allowed")
}

func (s *Server) effectiveExternalAuthAllowedOrigins() []string {
	allowedOrigins := sanitizeExternalAuthOrigins(s.allowedOrigins)
	if len(allowedOrigins) == 0 {
		allowedOrigins = defaultExternalAuthAllowedOrigins()
	}

	if publicBaseURL, _ := s.effectiveExternalAuthPublicBaseURL(); publicBaseURL != "" {
		if origin := externalAuthOriginString(publicBaseURL); origin != "" {
			allowedOrigins = appendExternalAuthOrigin(allowedOrigins, origin)
		}
	}

	return allowedOrigins
}

func sanitizeExternalAuthOrigins(origins []string) []string {
	cleaned := make([]string, 0, len(origins))
	for _, origin := range origins {
		if normalized := externalAuthOriginString(origin); normalized != "" {
			cleaned = appendExternalAuthOrigin(cleaned, normalized)
		}
	}
	return cleaned
}

func defaultExternalAuthAllowedOrigins() []string {
	return []string{
		"http://localhost:3000",
		"http://127.0.0.1:3000",
	}
}

func externalAuthOriginString(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func appendExternalAuthOrigin(origins []string, origin string) []string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return origins
	}
	for _, existing := range origins {
		if sameExternalAuthOrigin(existing, origin) {
			return origins
		}
	}
	return append(origins, origin)
}

func externalAuthOriginAllowed(parsed *url.URL, allowedOrigins []string) bool {
	origin := parsed.Scheme + "://" + parsed.Host
	for _, allowedOrigin := range allowedOrigins {
		if sameExternalAuthOrigin(origin, allowedOrigin) {
			return true
		}
	}
	return false
}

func externalAuthRequestOrigin(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	for _, raw := range []string{
		strings.TrimSpace(c.GetHeader("Origin")),
		strings.TrimSpace(c.GetHeader("Referer")),
	} {
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		return parsed.Scheme + "://" + parsed.Host
	}
	return ""
}

func sameExternalAuthOrigin(left, right string) bool {
	leftParsed, leftErr := url.Parse(strings.TrimSpace(left))
	rightParsed, rightErr := url.Parse(strings.TrimSpace(right))
	if leftErr != nil || rightErr != nil {
		return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
	}

	if !strings.EqualFold(leftParsed.Scheme, rightParsed.Scheme) {
		return false
	}
	if normalizeExternalAuthPort(leftParsed) != normalizeExternalAuthPort(rightParsed) {
		return false
	}

	leftHost := strings.ToLower(leftParsed.Hostname())
	rightHost := strings.ToLower(rightParsed.Hostname())
	if leftHost == rightHost {
		return true
	}
	return isExternalAuthLocalDevHost(leftHost) && isExternalAuthLocalDevHost(rightHost)
}

func normalizeExternalAuthPort(parsed *url.URL) string {
	port := parsed.Port()
	if port != "" {
		return port
	}
	switch strings.ToLower(parsed.Scheme) {
	case externalAuthSchemeHTTP:
		return "80"
	case externalAuthSchemeHTTPS:
		return "443"
	default:
		return ""
	}
}

func isExternalAuthLocalDevHost(host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	switch host {
	case "localhost", "0.0.0.0", "::":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) externalAuthCallbackURL(providerID string) string {
	if baseURL, _ := s.effectiveExternalAuthPublicBaseURL(); baseURL != "" {
		parsed, parseErr := url.Parse(baseURL)
		if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
			return ""
		}
		target := parsed.ResolveReference(&url.URL{
			Path: "/api/v1/auth/providers/" + url.PathEscape(providerID) + "/callback",
		})
		return target.String()
	}
	return ""
}

func (s *Server) renderExternalAuthBridge(
	c *gin.Context,
	status int,
	returnTo string,
	targetURL string,
	payload externalAuthCallbackPayload,
) {
	targetOrigin := ""
	if parsed, err := url.Parse(strings.TrimSpace(returnTo)); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		targetOrigin = parsed.Scheme + "://" + parsed.Host
	}
	payloadJSON, _ := json.Marshal(payload)
	nonce, nonceErr := generateExternalAuthBridgeNonce()
	if nonceErr != nil {
		logger.Error("failed to generate external auth bridge CSP nonce", zap.Error(nonceErr))
		c.Header("Content-Security-Policy", externalAuthBridgeNoScriptCSP())
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(buildExternalAuthBridgeFallbackHTML()))
		return
	}
	c.Header("Content-Security-Policy", externalAuthBridgeCSP(nonce))
	body := buildExternalAuthBridgeHTML(string(payloadJSON), targetOrigin, targetURL, nonce)
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Data(status, "text/html; charset=utf-8", []byte(body))
}

func externalAuthSuccessRedirectTarget(returnTo string, forcePasswordChange bool) string {
	returnTo = strings.TrimSpace(returnTo)
	if !forcePasswordChange {
		return returnTo
	}
	parsed, err := url.Parse(returnTo)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return returnTo
	}
	parsed.Path = "/auth/change-password"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func generateExternalAuthBridgeNonce() (string, error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func externalAuthBridgeCSP(nonce string) string {
	return "default-src 'none'; script-src 'nonce-" + nonce + "'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'; connect-src 'none'; img-src 'none'; style-src 'none'"
}

func externalAuthBridgeNoScriptCSP() string {
	return "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'; connect-src 'none'; img-src 'none'; style-src 'none'"
}

func buildExternalAuthBridgeFallbackHTML() string {
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Shepherd External Login</title>
</head>
<body>
  <p>External login could not be completed. Return to Shepherd.</p>
</body>
</html>`
}

func buildExternalAuthBridgeHTML(payloadJSON, targetOrigin, targetURL, nonce string) string {
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Shepherd External Login</title>
</head>
<body>
  <p>External login completed. You can close this window.</p>
  <script nonce="` + nonce + `">
    (function () {
      const payload = ` + payloadJSON + `;
      const targetOrigin = ` + jsonStringLiteral(targetOrigin) + `;
      const targetURL = ` + jsonStringLiteral(targetURL) + `;
      const successTarget = payload && payload.force_password_change
        ? '/auth/change-password'
        : (targetURL || payload.return_to || '/dashboard');

      if (window.opener && targetOrigin) {
        try {
          window.opener.postMessage(payload, targetOrigin);
        } catch (_) {
          // ignore opener delivery failures and fall through to delayed close
        }
        window.setTimeout(function () {
          window.close();
        }, 150);
        return;
      }
      if (payload && payload.success) {
        window.location.replace(successTarget);
        return;
      }
      if (targetURL) {
        window.location.replace(targetURL);
        return;
      }
      document.body.innerHTML = '<p>External login completed. Return to Shepherd.</p>';
    })();
  </script>
</body>
</html>`
}

func jsonStringLiteral(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func firstExternalAuthCallbackValue(req runtimecontract.AuthCallbackRequest, key string) string {
	for _, source := range []map[string][]string{req.Query, req.Form} {
		if len(source) == 0 {
			continue
		}
		if values := source[key]; len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func (s *Server) finalizeExternalAuthLogin(
	ctx context.Context,
	providerID string,
	providerGeneration service.AuthProviderGeneration,
	authResult *runtimecontract.AuthResult,
) (generated.LoginResponse, *service.ExternalAuthUpsertResult, error) {
	if authResult == nil {
		return generated.LoginResponse{}, nil, fmt.Errorf("external auth result is required")
	}
	if err := s.ensureAuthSessionSchema(ctx); err != nil {
		return generated.LoginResponse{}, nil, fmt.Errorf("prepare auth session state for external login: %w", err)
	}

	var upsertResult *service.ExternalAuthUpsertResult
	disabledUser := false
	err := WithTx(ctx, s.client, func(tx *ent.Tx) error {
		if lockErr := service.LockAuthProviderMutation(ctx, tx, providerID); lockErr != nil {
			return lockErr
		}
		providerRow, loadErr := tx.Client().AuthProvider.Get(ctx, providerID)
		if loadErr != nil {
			if ent.IsNotFound(loadErr) {
				return errExternalAuthProviderUnavailable
			}
			return fmt.Errorf("reload auth provider for external login: %w", loadErr)
		}
		if generationErr := providerGeneration.Validate(providerRow); generationErr != nil {
			return generationErr
		}
		if !providerRow.Enabled {
			return errExternalAuthProviderUnavailable
		}
		txExternalAuth := s.externalAuth.WithTransaction(tx)

		var upsertErr error
		upsertResult, upsertErr = txExternalAuth.UpsertExternalUser(ctx, providerID, *authResult)
		if upsertErr != nil {
			return upsertErr
		}
		if upsertResult == nil || upsertResult.User == nil {
			return fmt.Errorf("external auth provisioning returned no user")
		}
		if !upsertResult.User.Enabled {
			disabledUser = true
			return s.revokeUserSessionsTx(ctx, tx, upsertResult.User.ID, "external_auth_user_disabled")
		}
		if upsertResult.IdentityStateChanged || upsertResult.RBACChanged {
			reason := "external_auth_rbac_changed"
			if upsertResult.IdentityStateChanged {
				reason = "external_auth_identity_state_changed"
			}
			if err := s.revokeUserSessionsTx(ctx, tx, upsertResult.User.ID, reason); err != nil {
				return err
			}
		}
		if issueErr := txExternalAuth.RecordLogin(ctx, upsertResult.User.ID); issueErr != nil {
			return fmt.Errorf("record last_login_at: %w", issueErr)
		}
		return nil
	})
	if err != nil {
		return generated.LoginResponse{}, upsertResult, err
	}
	if disabledUser {
		return generated.LoginResponse{}, upsertResult, errExternalAuthUserDisabled
	}
	expectedUserID := strings.TrimSpace(upsertResult.User.ID)
	expectedProviderID := strings.TrimSpace(providerID)
	// UpsertExternalUser returns the canonical, normalized external identity
	// persisted in the provisioning transaction. Bind every authorization
	// snapshot and issuance attempt to that exact identity generation.
	expectedExternalID := upsertResult.User.ExternalID
	validateTokenSubject := func(currentUser *ent.User) error {
		return validateExternalAuthTokenSubject(
			currentUser,
			expectedUserID,
			expectedProviderID,
			expectedExternalID,
		)
	}
	if s.externalAuthBeforeTokenIssue != nil {
		if hookErr := s.externalAuthBeforeTokenIssue(ctx); hookErr != nil {
			return generated.LoginResponse{}, upsertResult, hookErr
		}
	}
	for attempt := 0; attempt < loginAuthorizationSnapshotMaxRetries; attempt++ {
		snapshot, snapshotErr := s.loadLoginAuthorizationSnapshot(ctx, expectedUserID, validateTokenSubject)
		if snapshotErr != nil {
			return generated.LoginResponse{}, upsertResult, snapshotErr
		}

		var loginResp generated.LoginResponse
		if issueErr := WithTx(ctx, s.client, func(tx *ent.Tx) error {
			if lockErr := service.LockAuthProviderMutation(ctx, tx, providerID); lockErr != nil {
				return lockErr
			}
			providerRow, loadErr := tx.Client().AuthProvider.Get(ctx, providerID)
			if loadErr != nil {
				if ent.IsNotFound(loadErr) {
					return errExternalAuthProviderUnavailable
				}
				return fmt.Errorf("reload auth provider before issuing external login token: %w", loadErr)
			}
			if generationErr := providerGeneration.Validate(providerRow); generationErr != nil {
				return generationErr
			}
			if !providerRow.Enabled {
				return errExternalAuthProviderUnavailable
			}
			if lockErr := lockUserRow(ctx, tx, expectedUserID); lockErr != nil {
				return lockErr
			}
			currentUser, loadUserErr := tx.Client().User.Get(ctx, expectedUserID)
			if ent.IsNotFound(loadUserErr) {
				return errExternalAuthIdentityChanged
			}
			if loadUserErr != nil {
				return fmt.Errorf("reload external auth user before issuing token: %w", loadUserErr)
			}
			if identityErr := validateTokenSubject(currentUser); identityErr != nil {
				return identityErr
			}
			var tokenErr error
			loginResp, tokenErr = s.loginResponseFromAuthorizationSnapshot(snapshot)
			return tokenErr
		}); issueErr != nil {
			return generated.LoginResponse{}, upsertResult, issueErr
		}
		if activateErr := s.activateAuthSession(ctx, expectedUserID, snapshot.SessionVersion); activateErr != nil {
			if errors.Is(activateErr, service.ErrAuthSessionVersionChanged) {
				continue
			}
			return generated.LoginResponse{}, upsertResult, activateErr
		}
		return loginResp, upsertResult, nil
	}
	return generated.LoginResponse{}, upsertResult, fmt.Errorf("authorization changed repeatedly while activating external login session")
}

func validateExternalAuthTokenSubject(
	currentUser *ent.User,
	expectedUserID, expectedProviderID, expectedExternalID string,
) error {
	if currentUser == nil ||
		!currentUser.Enabled ||
		strings.TrimSpace(currentUser.ID) != strings.TrimSpace(expectedUserID) ||
		strings.TrimSpace(currentUser.AuthProviderID) != strings.TrimSpace(expectedProviderID) ||
		currentUser.ExternalID != expectedExternalID {
		return errExternalAuthIdentityChanged
	}
	return nil
}

func (s *Server) completeExternalAuthResultLogin(
	ctx context.Context,
	c *gin.Context,
	providerID string,
	providerGeneration service.AuthProviderGeneration,
	authResult *runtimecontract.AuthResult,
) (generated.LoginResponse, error) {
	loginResp, upsertResult, err := s.finalizeExternalAuthLogin(ctx, providerID, providerGeneration, authResult)
	if err != nil {
		return generated.LoginResponse{}, err
	}
	if s.audit != nil && upsertResult != nil && upsertResult.User != nil {
		clientIP, requestID := loginAuditContext(c)
		_ = s.audit.LogAction(ctx, "user.external_login", "user", upsertResult.User.ID, upsertResult.User.ID, map[string]interface{}{
			"auth_provider_id": providerID,
			"created":          upsertResult.Created,
			"updated":          upsertResult.Updated,
			"provider":         "external",
			"client_ip":        clientIP,
			"request_id":       requestID,
		})
	}
	return loginResp, nil
}

func cloneCredentialAttributes(value map[string]interface{}) map[string]interface{} {
	if len(value) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
