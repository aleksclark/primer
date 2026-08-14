package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	identitydb "github.com/aleksclark/primer/identity/internal/db"
)

func TestForbiddenDBNamesIncludesForeignProducts(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"primer":                 true,
		"primer_test":            true,
		"primer_tv":              true,
		"primer_tv_test":         true,
		"tv":                     true,
		"tv_test":                true,
		"curriculum_studio":      true,
		"curriculum_studio_test": true,
		"studio":                 true,
	}
	got := map[string]bool{}
	for _, n := range identitydb.ForbiddenDBNames {
		got[strings.ToLower(n)] = true
	}
	for n := range want {
		require.Truef(t, got[n], "ForbiddenDBNames missing %q", n)
	}
	// Identity's own names must remain free (not forbidden).
	require.False(t, got["primer_identity"])
	require.False(t, got["primer_identity_test"])
}

func TestParseDatabaseNameForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		dsn  string
		want string
	}{
		{"uri", "postgres://u:p@127.0.0.1:5432/primer_identity?sslmode=disable", "primer_identity"},
		{"uri_upper", "postgres://u:p@127.0.0.1:5432/Primer_Identity?sslmode=disable", "primer_identity"},
		{"libpq", "host=127.0.0.1 user=u password=p dbname=primer_identity_test sslmode=disable", "primer_identity_test"},
		{"double_slash_path", "postgres://u:p@127.0.0.1:5432//curriculum_studio_test?sslmode=disable", "curriculum_studio_test"},
		{"postgresql_scheme", "postgresql://u@localhost/primer_tv_test", "primer_tv_test"},
		{"spaces_libpq", "host=127.0.0.1 dbname= tv_test ", "tv_test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := identitydb.ParseDatabaseName(tc.dsn)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestValidateDatabaseURLRejectsForbiddenAndAllowsIdentity(t *testing.T) {
	t.Parallel()
	require.NoError(t, identitydb.ValidateDatabaseURL("postgres://u:p@127.0.0.1:5432/primer_identity?sslmode=disable"))
	require.NoError(t, identitydb.ValidateDatabaseURL("postgres://u:p@127.0.0.1:5432/primer_identity_test?sslmode=disable"))
	require.NoError(t, identitydb.ValidateDatabaseURL("host=127.0.0.1 user=u dbname=primer_identity sslmode=disable"))
	// Non-reserved ephemeral names are allowed at the library boundary (documented).
	require.NoError(t, identitydb.ValidateDatabaseURL("postgres://u:p@127.0.0.1:5432/identity_scratch_42?sslmode=disable"))

	for _, bad := range []string{
		"postgres://u:p@127.0.0.1:5432/primer?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/PRIMER_TV?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/curriculum_studio?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432//curriculum_studio_test?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/primer_tv_test?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/tv_test?sslmode=disable",
		"host=127.0.0.1 user=u dbname=primer_test sslmode=disable",
		"host=127.0.0.1 dbname= studio ",
		"postgres://u:p@127.0.0.1:5432/?sslmode=disable",
		"",
		"   ",
	} {
		err := identitydb.ValidateDatabaseURL(bad)
		require.Error(t, err, "dsn=%q", bad)
	}
}

func TestConnectAndMigrateRefuseForbiddenDBNames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Library entrypoints must not bypass config/CLI isolation — refuse before dial.
	for _, dsn := range []string{
		"postgres://identity:p@127.0.0.1:1/curriculum_studio_test?sslmode=disable&connect_timeout=1",
		"postgres://identity:p@127.0.0.1:1/primer?sslmode=disable&connect_timeout=1",
		"postgres://identity:p@127.0.0.1:1/primer_tv_test?sslmode=disable&connect_timeout=1",
		"host=127.0.0.1 port=1 user=identity password=p dbname=tv_test sslmode=disable connect_timeout=1",
		"postgres://identity:p@127.0.0.1:1//curriculum_studio?sslmode=disable&connect_timeout=1",
	} {
		_, err := identitydb.Connect(ctx, dsn)
		require.Error(t, err, "Connect must refuse %s", dsn)
		require.Contains(t, strings.ToLower(err.Error()), "forbidden")

		err = identitydb.Migrate(ctx, dsn)
		require.Error(t, err, "Migrate must refuse %s", dsn)
		require.Contains(t, strings.ToLower(err.Error()), "forbidden")

		err = identitydb.MigrateDown(ctx, dsn)
		require.Error(t, err, "MigrateDown must refuse %s", dsn)
		require.Contains(t, strings.ToLower(err.Error()), "forbidden")
	}
}

func TestIsForbiddenDBNameNormalized(t *testing.T) {
	t.Parallel()
	require.True(t, identitydb.IsForbiddenDBName("Curriculum_Studio_Test"))
	require.True(t, identitydb.IsForbiddenDBName(" /primer_tv_test/ "))
	require.False(t, identitydb.IsForbiddenDBName("primer_identity"))
	require.False(t, identitydb.IsForbiddenDBName(""))
}
