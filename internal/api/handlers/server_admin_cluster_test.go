package handlers

import (
	"net/http"
	"testing"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestListClusters_RequiresFilterPaginationUsesFilteredTotals(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	mustCreateCluster := func(id, name string, features []string) {
		t.Helper()
		_, err := client.Cluster.Create().
			SetID(id).
			SetName(name).
			SetDisplayName(name).
			SetAPIServerURL("https://cluster.invalid").
			SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
			SetStatus(entcluster.StatusHEALTHY).
			SetCreatedBy("test").
			SetEnabledFeatures(features).
			Save(ctx)
		if err != nil {
			t.Fatalf("create cluster %s: %v", id, err)
		}
	}

	mustCreateCluster("cl-1", "cluster-a", []string{"LiveMigration", "Snapshot"})
	mustCreateCluster("cl-2", "cluster-b", []string{"LiveMigration"})
	mustCreateCluster("cl-3", "cluster-c", []string{"GPU"})

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=1&per_page=1&requires=LiveMigration,Snapshot",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:     1,
		PerPage:  1,
		Requires: "LiveMigration,Snapshot",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].Name != "cluster-a" {
		t.Fatalf("items[0].name = %q, want cluster-a", resp.Items[0].Name)
	}
	if got := resp.Pagination.Total; got != 1 {
		t.Fatalf("pagination.total = %d, want 1 (filtered total)", got)
	}
	if got := resp.Pagination.TotalPages; got != 1 {
		t.Fatalf("pagination.total_pages = %d, want 1", got)
	}
}

func TestListClusters_RequiresFilterSecondPageKeepsFilteredTotals(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.Cluster.Create().
		SetID("cl-1").
		SetName("cluster-a").
		SetDisplayName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetCreatedBy("test").
		SetEnabledFeatures([]string{"LiveMigration", "Snapshot"}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/clusters?page=2&per_page=1&requires=LiveMigration,Snapshot",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListClusters(c, generated.ListClustersParams{
		Page:     2,
		PerPage:  1,
		Requires: "LiveMigration,Snapshot",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list clusters page2 status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ClusterList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 0 {
		t.Fatalf("items len page2 = %d, want 0", got)
	}
	if got := resp.Pagination.Total; got != 1 {
		t.Fatalf("pagination.total page2 = %d, want 1 (filtered total)", got)
	}
	if got := resp.Pagination.TotalPages; got != 1 {
		t.Fatalf("pagination.total_pages page2 = %d, want 1", got)
	}
}
