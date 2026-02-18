package testutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDsnWithSearchPath_URL(t *testing.T) {
	dsn := "postgres://user:pass@localhost:5432/shepherd?sslmode=disable"
	got, err := dsnWithSearchPath(dsn, "tenant_a")
	require.NoError(t, err)
	require.Contains(t, got, "search_path=tenant_a")
	require.Contains(t, got, "sslmode=disable")
}

func TestDsnWithSearchPath_KeywordAndReplace(t *testing.T) {
	keywordDSN := "host=localhost port=5432 dbname=shepherd user=user password=pass sslmode=disable"
	got, err := dsnWithSearchPath(keywordDSN, "tenant_b")
	require.NoError(t, err)
	require.Contains(t, got, "search_path=tenant_b")

	withExisting := "host=localhost dbname=shepherd search_path=public sslmode=disable"
	got, err = dsnWithSearchPath(withExisting, "tenant_c")
	require.NoError(t, err)
	require.Contains(t, got, "search_path=tenant_c")
	require.NotContains(t, got, "search_path=public")
}

func TestNewSchemaName_NormalizationAndLength(t *testing.T) {
	got := newSchemaName("Feature-X/Prod@Namespace")
	require.True(t, strings.HasPrefix(got, "t_"))
	require.LessOrEqual(t, len(got), 63)
	require.NotContains(t, got, "-")
	require.NotContains(t, got, "/")
	require.Equal(t, strings.ToLower(got), got)
}
