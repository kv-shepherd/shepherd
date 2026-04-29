package modules

import (
	"context"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
	kubeconfigcodec "kv-shepherd.io/shepherd/internal/provider/kubeconfigcodec"
	"kv-shepherd.io/shepherd/internal/testutil"
)

const testModulesSafeClusterKubeconfig = `
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
    token: cluster-token
contexts:
- name: prod
  context:
    cluster: prod
    user: prod-user
current-context: prod
`

func TestMigrateLegacyClusterKubeconfigs_ReencryptsPlaintextRows(t *testing.T) {
	t.Parallel()

	if err := logger.Init("error", "json"); err != nil {
		t.Fatalf("logger.Init() error = %v", err)
	}

	client := testutil.OpenEntPostgres(t, "modules_cluster_kubeconfig_migration")
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-legacy-startup").
		SetName("legacy-startup").
		SetDisplayName("Legacy Startup").
		SetAPIServerURL("https://stale.example.com").
		SetEncryptedKubeconfig([]byte(testModulesSafeClusterKubeconfig)).
		SetCreatedBy("test").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	codec := kubeconfigcodec.NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	migrateLegacyClusterKubeconfigs(context.Background(), client, codec)

	stored, err := client.Cluster.Get(ctx, "cl-legacy-startup")
	if err != nil {
		t.Fatalf("reload cluster: %v", err)
	}
	if stored.EncryptionKeyID == "" {
		t.Fatal("encryption_key_id is empty after migration")
	}
	if stored.APIServerURL != "https://cluster.example.com" {
		t.Fatalf("api_server_url = %q, want https://cluster.example.com", stored.APIServerURL)
	}
	if strings.Contains(string(stored.EncryptedKubeconfig), "cluster-token") {
		t.Fatalf("stored payload leaked plaintext token: %s", string(stored.EncryptedKubeconfig))
	}
	if !strings.HasPrefix(string(stored.EncryptedKubeconfig), "kc:v1:") {
		t.Fatalf("stored payload = %q, want encrypted prefix", string(stored.EncryptedKubeconfig))
	}
}

func TestNewClusterKubeconfigLoader_MigratesLegacyPlaintextRowsOnRead(t *testing.T) {
	t.Parallel()

	if err := logger.Init("error", "json"); err != nil {
		t.Fatalf("logger.Init() error = %v", err)
	}

	client := testutil.OpenEntPostgres(t, "modules_cluster_kubeconfig_loader_migration")
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-legacy-read").
		SetName("legacy-read").
		SetDisplayName("Legacy Read").
		SetAPIServerURL("https://stale.example.com").
		SetEncryptedKubeconfig([]byte(testModulesSafeClusterKubeconfig)).
		SetCreatedBy("test").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	codec := kubeconfigcodec.NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	loader := newClusterKubeconfigLoader(client, codec, false)

	loaded, err := loader("cl-legacy-read")
	if err != nil {
		t.Fatalf("loader() error = %v", err)
	}
	if !strings.Contains(string(loaded), "current-context: shepherd") {
		t.Fatalf("loaded kubeconfig = %s, want canonicalized current-context", string(loaded))
	}

	stored, err := client.Cluster.Get(ctx, "cl-legacy-read")
	if err != nil {
		t.Fatalf("reload cluster: %v", err)
	}
	if stored.EncryptionKeyID == "" {
		t.Fatal("encryption_key_id is empty after lazy migration")
	}
	if stored.APIServerURL != "https://cluster.example.com" {
		t.Fatalf("api_server_url = %q, want https://cluster.example.com", stored.APIServerURL)
	}
	if strings.Contains(string(stored.EncryptedKubeconfig), "cluster-token") {
		t.Fatalf("stored payload leaked plaintext token: %s", string(stored.EncryptedKubeconfig))
	}
}

func TestMigrateLegacyClusterKubeconfigs_ReencryptsRowsWithEmptyKeyID(t *testing.T) {
	t.Parallel()

	if err := logger.Init("error", "json"); err != nil {
		t.Fatalf("logger.Init() error = %v", err)
	}

	client := testutil.OpenEntPostgres(t, "modules_cluster_kubeconfig_empty_keyid")
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-empty-keyid").
		SetName("empty-keyid").
		SetDisplayName("Empty KeyID").
		SetAPIServerURL("https://stale.example.com").
		SetEncryptedKubeconfig([]byte(testModulesSafeClusterKubeconfig)).
		SetEncryptionKeyID("").
		SetCreatedBy("test").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	codec := kubeconfigcodec.NewClusterKubeconfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	migrateLegacyClusterKubeconfigs(context.Background(), client, codec)

	stored, err := client.Cluster.Get(ctx, "cl-empty-keyid")
	if err != nil {
		t.Fatalf("reload cluster: %v", err)
	}
	if stored.EncryptionKeyID == "" {
		t.Fatal("encryption_key_id is empty after migration")
	}
	if !strings.HasPrefix(string(stored.EncryptedKubeconfig), "kc:v1:") {
		t.Fatalf("stored payload = %q, want encrypted prefix", string(stored.EncryptedKubeconfig))
	}
}
