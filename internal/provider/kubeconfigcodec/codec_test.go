package kubeconfigcodec

import (
	"errors"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

const testSafeClusterKubeconfig = `
apiVersion: v1
kind: Config
clusters:
- name: prod
  cluster:
    server: https://cluster.example.com
    certificate-authority-data: Y2E=
users:
- name: prod-user
  user:
    token: token-123
contexts:
- name: prod
  context:
    cluster: prod
    user: prod-user
current-context: prod
`

func TestClusterKubeconfigCodecPrepareForStorageEncryptsCanonicalConfig(t *testing.T) {
	t.Parallel()

	codec := NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	stored, apiServerURL, keyID, err := codec.PrepareForStorage([]byte(testSafeClusterKubeconfig))
	if err != nil {
		t.Fatalf("PrepareForStorage() error = %v", err)
	}
	if apiServerURL != "https://cluster.example.com" {
		t.Fatalf("apiServerURL = %q, want https://cluster.example.com", apiServerURL)
	}
	if keyID == "" {
		t.Fatal("keyID is empty")
	}
	if strings.Contains(string(stored), "token-123") {
		t.Fatalf("stored payload leaked plaintext token: %s", string(stored))
	}

	loaded, err := codec.LoadForRuntime(stored, keyID)
	if err != nil {
		t.Fatalf("LoadForRuntime() error = %v", err)
	}
	cfg, err := clientcmd.Load(loaded)
	if err != nil {
		t.Fatalf("clientcmd.Load() error = %v", err)
	}
	if got := cfg.CurrentContext; got != sanitizedKubeconfigContext {
		t.Fatalf("current-context = %q, want %q", got, sanitizedKubeconfigContext)
	}
	if got := cfg.Clusters[sanitizedKubeconfigCluster].Server; got != "https://cluster.example.com" {
		t.Fatalf("cluster server = %q, want https://cluster.example.com", got)
	}
	if got := cfg.AuthInfos[sanitizedKubeconfigUser].Token; got != "token-123" {
		t.Fatalf("token = %q, want token-123", got)
	}
}

func TestClusterKubeconfigCodecPrepareForStorageRejectsDangerousFields(t *testing.T) {
	t.Parallel()

	testCases := map[string]string{
		"exec": `
apiVersion: v1
kind: Config
clusters:
- name: prod
  cluster:
    server: https://cluster.example.com
users:
- name: prod-user
  user:
    exec:
      command: /bin/sh
      apiVersion: client.authentication.k8s.io/v1
      interactiveMode: Never
contexts:
- name: prod
  context:
    cluster: prod
    user: prod-user
current-context: prod
`,
		"client-key-path": `
apiVersion: v1
kind: Config
clusters:
- name: prod
  cluster:
    server: https://cluster.example.com
users:
- name: prod-user
  user:
    client-certificate: /tmp/client.crt
    client-key: /tmp/client.key
contexts:
- name: prod
  context:
    cluster: prod
    user: prod-user
current-context: prod
`,
		"proxy-url": `
apiVersion: v1
kind: Config
clusters:
- name: prod
  cluster:
    server: https://cluster.example.com
    proxy-url: http://127.0.0.1:8080
users:
- name: prod-user
  user:
    token: token-123
contexts:
- name: prod
  context:
    cluster: prod
    user: prod-user
current-context: prod
`,
	}

	codec := NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	for name, raw := range testCases {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := codec.PrepareForStorage([]byte(raw))
			if !errors.Is(err, ErrInvalidClusterKubeconfig) {
				t.Fatalf("PrepareForStorage() error = %v, want %v", err, ErrInvalidClusterKubeconfig)
			}
		})
	}
}

func TestClusterKubeconfigCodecLoadForRuntimeSupportsLegacyPlaintextRows(t *testing.T) {
	t.Parallel()

	codec := NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	loaded, err := codec.LoadForRuntime([]byte(testSafeClusterKubeconfig), "")
	if err != nil {
		t.Fatalf("LoadForRuntime() error = %v", err)
	}
	if !strings.Contains(string(loaded), "current-context: shepherd") {
		t.Fatalf("loaded config = %s, want canonical current-context", string(loaded))
	}
}

func TestClusterKubeconfigCodecLoadForRuntimeSupportsLegacyPlaintextRowsWithStaleKeyID(t *testing.T) {
	t.Parallel()

	codec := NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	loaded, err := codec.LoadForRuntime([]byte(testSafeClusterKubeconfig), "legacy-placeholder")
	if err != nil {
		t.Fatalf("LoadForRuntime() error = %v", err)
	}
	if !strings.Contains(string(loaded), "current-context: shepherd") {
		t.Fatalf("loaded config = %s, want canonical current-context", string(loaded))
	}
}

func TestClusterKubeconfigCodecPrepareForMigrationEncryptsLegacyPlaintextRows(t *testing.T) {
	t.Parallel()

	codec := NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	migration, err := codec.PrepareForMigration([]byte(testSafeClusterKubeconfig), "")
	if err != nil {
		t.Fatalf("PrepareForMigration() error = %v", err)
	}
	if migration == nil {
		t.Fatal("PrepareForMigration() returned nil migration")
	}
	if migration.APIServerURL != "https://cluster.example.com" {
		t.Fatalf("apiServerURL = %q, want https://cluster.example.com", migration.APIServerURL)
	}
	if migration.EncryptionKeyID == "" {
		t.Fatal("encryptionKeyID is empty")
	}
	if strings.Contains(string(migration.EncryptedKubeconfig), "token-123") {
		t.Fatalf("migration payload leaked plaintext token: %s", string(migration.EncryptedKubeconfig))
	}
}

func TestClusterKubeconfigCodecLoadForRuntimeRejectsKeyMismatch(t *testing.T) {
	t.Parallel()

	codecA := NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	codecB := NewClusterKubeconfigCodec([]byte("fedcba9876543210fedcba9876543210"))
	stored, _, keyID, err := codecA.PrepareForStorage([]byte(testSafeClusterKubeconfig))
	if err != nil {
		t.Fatalf("PrepareForStorage() error = %v", err)
	}
	if _, err := codecB.LoadForRuntime(stored, keyID); !errors.Is(err, ErrClusterKubeconfigKeyMismatch) {
		t.Fatalf("LoadForRuntime() error = %v, want %v", err, ErrClusterKubeconfigKeyMismatch)
	}
}
