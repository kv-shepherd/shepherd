package kubeconfigcodec

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	clusterKubeconfigEncPrefix   = "kc:v1:"
	sanitizedKubeconfigContext   = "shepherd"
	sanitizedKubeconfigCluster   = "cluster"
	sanitizedKubeconfigUser      = "user"
	currentContextFallbackSuffix = "current-context"
)

var (
	ErrInvalidClusterKubeconfig     = errors.New("invalid cluster kubeconfig")
	ErrClusterKubeconfigKeyMissing  = errors.New("cluster kubeconfig encryption key is not configured")
	ErrClusterKubeconfigCiphertext  = errors.New("cluster kubeconfig ciphertext is invalid")
	ErrClusterKubeconfigDecrypt     = errors.New("cluster kubeconfig decryption failed")
	ErrClusterKubeconfigKeyMismatch = errors.New("cluster kubeconfig was encrypted with an unavailable key")
)

// ClusterKubeconfigCodec sanitizes and protects cluster kubeconfig bytes.
type ClusterKubeconfigCodec struct {
	encryptionKey []byte
}

type ClusterKubeconfigMigration struct {
	EncryptedKubeconfig []byte
	APIServerURL        string
	EncryptionKeyID     string
}

func NewClusterKubeconfigCodec(encryptionKey []byte) *ClusterKubeconfigCodec {
	return &ClusterKubeconfigCodec{encryptionKey: encryptionKey}
}

func (c *ClusterKubeconfigCodec) KeyID() string {
	if len(c.encryptionKey) == 0 {
		return ""
	}
	sum := sha256.Sum256(c.encryptionKey)
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// PrepareForStorage validates, canonicalizes, and encrypts kubeconfig bytes for persistence.
func (c *ClusterKubeconfigCodec) PrepareForStorage(raw []byte) (stored []byte, apiServerURL, encryptionKeyID string, err error) {
	sanitized, apiServerURL, err := SanitizeClusterKubeconfig(raw)
	if err != nil {
		return nil, "", "", err
	}
	encrypted, err := c.encrypt(sanitized)
	if err != nil {
		return nil, "", "", err
	}
	return encrypted, apiServerURL, c.KeyID(), nil
}

func (c *ClusterKubeconfigCodec) PrepareForMigration(stored []byte, encryptionKeyID string) (*ClusterKubeconfigMigration, error) {
	if strings.TrimSpace(string(stored)) == "" {
		return nil, nil
	}
	if c == nil {
		return nil, fmt.Errorf("cluster kubeconfig codec is not configured")
	}

	currentKeyID := c.KeyID()
	trimmedKeyID := strings.TrimSpace(encryptionKeyID)
	if c.isEncryptedPayload(stored) && trimmedKeyID != "" && trimmedKeyID == currentKeyID {
		return nil, nil
	}

	plaintext, err := c.decryptMaybe(stored, encryptionKeyID)
	if err != nil {
		return nil, err
	}
	encrypted, apiServerURL, keyID, err := c.PrepareForStorage(plaintext)
	if err != nil {
		return nil, err
	}
	return &ClusterKubeconfigMigration{
		EncryptedKubeconfig: encrypted,
		APIServerURL:        apiServerURL,
		EncryptionKeyID:     keyID,
	}, nil
}

// LoadForRuntime decrypts stored kubeconfig bytes and re-sanitizes them before use.
// Legacy plaintext rows remain readable, but still pass through the same sanitizer.
func (c *ClusterKubeconfigCodec) LoadForRuntime(stored []byte, encryptionKeyID string) ([]byte, error) {
	plaintext, err := c.decryptMaybe(stored, encryptionKeyID)
	if err != nil {
		return nil, err
	}
	sanitized, _, err := SanitizeClusterKubeconfig(plaintext)
	if err != nil {
		return nil, err
	}
	return sanitized, nil
}

func (c *ClusterKubeconfigCodec) isEncryptedPayload(stored []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(stored)), clusterKubeconfigEncPrefix)
}

func (c *ClusterKubeconfigCodec) encrypt(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, nil
	}
	if len(c.encryptionKey) == 0 {
		return nil, ErrClusterKubeconfigKeyMissing
	}
	block, err := aes.NewCipher(c.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("create cluster kubeconfig cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, fmt.Errorf("create cluster kubeconfig aead: %w", err)
	}
	ciphertext := aead.Seal(nil, nil, plain, nil)
	encoded := clusterKubeconfigEncPrefix + base64.RawURLEncoding.EncodeToString(ciphertext)
	return []byte(encoded), nil
}

func (c *ClusterKubeconfigCodec) decryptMaybe(stored []byte, encryptionKeyID string) ([]byte, error) {
	if len(stored) == 0 {
		return nil, nil
	}

	encoded := strings.TrimSpace(string(stored))
	if !strings.HasPrefix(encoded, clusterKubeconfigEncPrefix) {
		return append([]byte(nil), stored...), nil
	}

	if len(c.encryptionKey) == 0 {
		return nil, ErrClusterKubeconfigKeyMissing
	}
	if expected := strings.TrimSpace(encryptionKeyID); expected != "" && expected != c.KeyID() {
		return nil, fmt.Errorf("%w: stored=%s current=%s", ErrClusterKubeconfigKeyMismatch, expected, c.KeyID())
	}

	raw := strings.TrimPrefix(encoded, clusterKubeconfigEncPrefix)
	ciphertext, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.Join(ErrClusterKubeconfigCiphertext, err)
	}
	block, err := aes.NewCipher(c.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("create cluster kubeconfig cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, fmt.Errorf("create cluster kubeconfig aead: %w", err)
	}
	plaintext, err := aead.Open(nil, nil, ciphertext, nil)
	if err != nil {
		return nil, errors.Join(ErrClusterKubeconfigDecrypt, err)
	}
	return plaintext, nil
}

// SanitizeClusterKubeconfig reduces kubeconfig input to the minimal safe subset
// Shepherd accepts for server-side cluster access.
func SanitizeClusterKubeconfig(raw []byte) (sanitizedKubeconfig []byte, apiServerURL string, err error) {
	cfg, err := clientcmd.Load(raw)
	if err != nil {
		return nil, "", errors.Join(ErrInvalidClusterKubeconfig, fmt.Errorf("parse kubeconfig YAML: %w", err))
	}
	sanitized, apiServerURL, err := sanitizeClusterKubeconfigConfig(cfg)
	if err != nil {
		return nil, "", err
	}
	serialized, err := clientcmd.Write(*sanitized)
	if err != nil {
		return nil, "", errors.Join(ErrInvalidClusterKubeconfig, fmt.Errorf("serialize sanitized kubeconfig: %w", err))
	}
	return serialized, apiServerURL, nil
}

func sanitizeClusterKubeconfigConfig(cfg *clientcmdapi.Config) (*clientcmdapi.Config, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("%w: kubeconfig must not be empty", ErrInvalidClusterKubeconfig)
	}
	if len(cfg.Extensions) > 0 {
		return nil, "", fmt.Errorf("%w: top-level extensions are not supported", ErrInvalidClusterKubeconfig)
	}

	currentContextName, currentContext, err := resolveClusterKubeconfigCurrentContext(cfg)
	if err != nil {
		return nil, "", err
	}
	if currentContext == nil {
		return nil, "", fmt.Errorf("%w: current-context %q was not found", ErrInvalidClusterKubeconfig, currentContextName)
	}

	clusterName := strings.TrimSpace(currentContext.Cluster)
	if clusterName == "" {
		return nil, "", fmt.Errorf("%w: current-context cluster is required", ErrInvalidClusterKubeconfig)
	}
	clusterCfg, ok := cfg.Clusters[clusterName]
	if !ok || clusterCfg == nil {
		return nil, "", fmt.Errorf("%w: cluster %q was not found", ErrInvalidClusterKubeconfig, clusterName)
	}

	authInfoName := strings.TrimSpace(currentContext.AuthInfo)
	if authInfoName == "" {
		return nil, "", fmt.Errorf("%w: current-context user is required", ErrInvalidClusterKubeconfig)
	}
	authInfoCfg, ok := cfg.AuthInfos[authInfoName]
	if !ok || authInfoCfg == nil {
		return nil, "", fmt.Errorf("%w: user %q was not found", ErrInvalidClusterKubeconfig, authInfoName)
	}

	sanitizedCluster, apiServerURL, err := sanitizeClusterKubeconfigCluster(clusterCfg)
	if err != nil {
		return nil, "", err
	}
	sanitizedAuthInfo, err := sanitizeClusterKubeconfigAuthInfo(authInfoCfg)
	if err != nil {
		return nil, "", err
	}

	result := clientcmdapi.NewConfig()
	result.CurrentContext = sanitizedKubeconfigContext
	result.Contexts[sanitizedKubeconfigContext] = &clientcmdapi.Context{
		Cluster:  sanitizedKubeconfigCluster,
		AuthInfo: sanitizedKubeconfigUser,
	}
	if namespace := strings.TrimSpace(currentContext.Namespace); namespace != "" {
		result.Contexts[sanitizedKubeconfigContext].Namespace = namespace
	}
	result.Clusters[sanitizedKubeconfigCluster] = sanitizedCluster
	result.AuthInfos[sanitizedKubeconfigUser] = sanitizedAuthInfo
	return result, apiServerURL, nil
}

func resolveClusterKubeconfigCurrentContext(cfg *clientcmdapi.Config) (string, *clientcmdapi.Context, error) {
	currentContextName := strings.TrimSpace(cfg.CurrentContext)
	if currentContextName != "" {
		currentContext := cfg.Contexts[currentContextName]
		if currentContext == nil {
			return "", nil, fmt.Errorf("%w: current-context %q was not found", ErrInvalidClusterKubeconfig, currentContextName)
		}
		return currentContextName, currentContext, nil
	}
	if len(cfg.Contexts) != 1 {
		return "", nil, fmt.Errorf("%w: current-context is required when kubeconfig contains %d contexts", ErrInvalidClusterKubeconfig, len(cfg.Contexts))
	}
	for name, ctx := range cfg.Contexts {
		return strings.TrimSpace(name), ctx, nil
	}
	return "", nil, fmt.Errorf("%w: kubeconfig does not define a usable %s", ErrInvalidClusterKubeconfig, currentContextFallbackSuffix)
}

func sanitizeClusterKubeconfigCluster(clusterCfg *clientcmdapi.Cluster) (*clientcmdapi.Cluster, string, error) {
	if clusterCfg == nil {
		return nil, "", fmt.Errorf("%w: kubeconfig cluster entry is required", ErrInvalidClusterKubeconfig)
	}

	if strings.TrimSpace(clusterCfg.CertificateAuthority) != "" {
		return nil, "", fmt.Errorf("%w: certificate-authority file paths are not supported", ErrInvalidClusterKubeconfig)
	}
	if strings.TrimSpace(clusterCfg.ProxyURL) != "" {
		return nil, "", fmt.Errorf("%w: proxy-url is not supported", ErrInvalidClusterKubeconfig)
	}
	if clusterCfg.InsecureSkipTLSVerify {
		return nil, "", fmt.Errorf("%w: insecure-skip-tls-verify must remain false", ErrInvalidClusterKubeconfig)
	}
	if len(clusterCfg.Extensions) > 0 {
		return nil, "", fmt.Errorf("%w: cluster extensions are not supported", ErrInvalidClusterKubeconfig)
	}

	serverURL, err := sanitizeClusterKubeconfigServerURL(clusterCfg.Server)
	if err != nil {
		return nil, "", err
	}

	sanitized := &clientcmdapi.Cluster{
		Server:                   serverURL,
		TLSServerName:            strings.TrimSpace(clusterCfg.TLSServerName),
		CertificateAuthorityData: append([]byte(nil), clusterCfg.CertificateAuthorityData...),
		DisableCompression:       clusterCfg.DisableCompression,
	}
	return sanitized, serverURL, nil
}

func sanitizeClusterKubeconfigAuthInfo(authInfoCfg *clientcmdapi.AuthInfo) (*clientcmdapi.AuthInfo, error) {
	if authInfoCfg == nil {
		return nil, fmt.Errorf("%w: kubeconfig user entry is required", ErrInvalidClusterKubeconfig)
	}
	if authInfoCfg.AuthProvider != nil {
		return nil, fmt.Errorf("%w: auth-provider plugins are not supported", ErrInvalidClusterKubeconfig)
	}
	if authInfoCfg.Exec != nil {
		return nil, fmt.Errorf("%w: exec credential plugins are not supported", ErrInvalidClusterKubeconfig)
	}
	if strings.TrimSpace(authInfoCfg.TokenFile) != "" {
		return nil, fmt.Errorf("%w: tokenFile references are not supported", ErrInvalidClusterKubeconfig)
	}
	if strings.TrimSpace(authInfoCfg.ClientCertificate) != "" || strings.TrimSpace(authInfoCfg.ClientKey) != "" {
		return nil, fmt.Errorf("%w: client certificate/key file paths are not supported", ErrInvalidClusterKubeconfig)
	}
	if strings.TrimSpace(authInfoCfg.Impersonate) != "" ||
		strings.TrimSpace(authInfoCfg.ImpersonateUID) != "" ||
		len(authInfoCfg.ImpersonateGroups) > 0 ||
		len(authInfoCfg.ImpersonateUserExtra) > 0 {
		return nil, fmt.Errorf("%w: impersonation fields are not supported", ErrInvalidClusterKubeconfig)
	}
	if len(authInfoCfg.Extensions) > 0 {
		return nil, fmt.Errorf("%w: user extensions are not supported", ErrInvalidClusterKubeconfig)
	}

	token := strings.TrimSpace(authInfoCfg.Token)
	username := strings.TrimSpace(authInfoCfg.Username)
	password := strings.TrimSpace(authInfoCfg.Password)
	clientCertificateData := append([]byte(nil), authInfoCfg.ClientCertificateData...)
	clientKeyData := append([]byte(nil), authInfoCfg.ClientKeyData...)

	hasToken := token != ""
	hasClientCertData := len(clientCertificateData) > 0
	hasClientKeyData := len(clientKeyData) > 0
	hasBasicAuth := username != "" || password != ""

	if hasClientCertData != hasClientKeyData {
		return nil, fmt.Errorf("%w: embedded client-certificate-data and client-key-data must be provided together", ErrInvalidClusterKubeconfig)
	}
	if hasBasicAuth && (username == "" || password == "") {
		return nil, fmt.Errorf("%w: username and password must be provided together", ErrInvalidClusterKubeconfig)
	}
	if !hasToken && !hasClientCertData && !hasBasicAuth {
		return nil, fmt.Errorf("%w: kubeconfig must embed a token, client certificate pair, or username/password", ErrInvalidClusterKubeconfig)
	}

	return &clientcmdapi.AuthInfo{
		ClientCertificateData: clientCertificateData,
		ClientKeyData:         clientKeyData,
		Token:                 token,
		Username:              username,
		Password:              password,
	}, nil
}

func sanitizeClusterKubeconfigServerURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: kubeconfig cluster server URL is required", ErrInvalidClusterKubeconfig)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: kubeconfig cluster server URL must be a valid absolute URL", ErrInvalidClusterKubeconfig)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: kubeconfig cluster server URL must use https", ErrInvalidClusterKubeconfig)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%w: kubeconfig cluster server URL must not include userinfo", ErrInvalidClusterKubeconfig)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: kubeconfig cluster server URL must not include query or fragment", ErrInvalidClusterKubeconfig)
	}
	return parsed.String(), nil
}
