package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

func TestForbiddenDBNamesIncludesIdentity(t *testing.T) {
	t.Parallel()
	want := map[string]bool{
		"primer":               true,
		"primer_test":          true,
		"primer_tv":            true,
		"primer_tv_test":       true,
		"tv":                   true,
		"tv_test":              true,
		"primer_identity":      true,
		"primer_identity_test": true,
	}
	got := map[string]bool{}
	for _, n := range studiodb.ForbiddenDBNames {
		got[strings.ToLower(n)] = true
	}
	for n := range want {
		require.Truef(t, got[n], "ForbiddenDBNames missing %q", n)
	}
}

func TestParseDatabaseNameForms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		dsn  string
		want string
	}{
		{"uri", "postgres://u:p@127.0.0.1:5432/curriculum_studio?sslmode=disable", "curriculum_studio"},
		{"uri_upper", "postgres://u:p@127.0.0.1:5432/Curriculum_Studio?sslmode=disable", "curriculum_studio"},
		{"libpq", "host=127.0.0.1 user=u password=p dbname=curriculum_studio_test sslmode=disable", "curriculum_studio_test"},
		{"double_slash_path", "postgres://u:p@127.0.0.1:5432//primer_identity?sslmode=disable", "primer_identity"},
		{"postgresql_scheme", "postgresql://u@localhost/primer_identity_test", "primer_identity_test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := studiodb.ParseDatabaseName(tc.dsn)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestValidateDatabaseURLRejectsForbiddenAndAllowsStudio(t *testing.T) {
	t.Parallel()
	require.NoError(t, studiodb.ValidateDatabaseURL("postgres://u:p@127.0.0.1:5432/curriculum_studio?sslmode=disable"))
	require.NoError(t, studiodb.ValidateDatabaseURL("postgres://u:p@127.0.0.1:5432/curriculum_studio_test?sslmode=disable"))
	require.NoError(t, studiodb.ValidateDatabaseURL("host=127.0.0.1 user=u dbname=curriculum_studio sslmode=disable"))

	for _, bad := range []string{
		"postgres://u:p@127.0.0.1:5432/primer?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/PRIMER_TV?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/primer_identity?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432//primer_identity_test?sslmode=disable",
		"host=127.0.0.1 user=u dbname=primer_test sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/?sslmode=disable",
	} {
		err := studiodb.ValidateDatabaseURL(bad)
		require.Error(t, err, "dsn=%s", bad)
	}
}

func TestConnectAndMigrateRefuseForbiddenDBNames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Library entrypoints must not bypass config/CLI isolation.
	for _, dsn := range []string{
		"postgres://studio:studio@127.0.0.1:1/primer_identity?sslmode=disable&connect_timeout=1",
		"postgres://studio:studio@127.0.0.1:1/primer?sslmode=disable&connect_timeout=1",
		"host=127.0.0.1 port=1 user=studio password=studio dbname=primer_tv sslmode=disable connect_timeout=1",
	} {
		_, err := studiodb.Connect(ctx, dsn)
		require.Error(t, err, "Connect must refuse %s", dsn)
		require.Contains(t, strings.ToLower(err.Error()), "forbidden")

		err = studiodb.Migrate(ctx, dsn)
		require.Error(t, err, "Migrate must refuse %s", dsn)
		require.Contains(t, strings.ToLower(err.Error()), "forbidden")
	}
}

func TestLoadConfigRejectsIdentityDBNames(t *testing.T) {
	t.Setenv("STUDIO_MIGRATIONS_DIR", t.TempDir())
	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:***@127.0.0.1:5432/primer_identity?sslmode=disable")
	_, err := studiodb.LoadConfig()
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "forbidden")

	t.Setenv("STUDIO_DATABASE_URL", "postgres://studio:***@127.0.0.1:5432/primer_identity_test?sslmode=disable")
	_, err = studiodb.LoadConfig()
	require.Error(t, err)
}
