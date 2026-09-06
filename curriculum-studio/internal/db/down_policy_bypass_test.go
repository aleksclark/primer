package db_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
)

// TestNoExportedMigrateDownBypass ensures package db no longer exports a
// policy-free down path that callers could use to skip DownWithPolicy.
func TestNoExportedMigrateDownBypass(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(file)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgDir, func(fi os.FileInfo) bool {
		name := fi.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	require.NoError(t, err)

	pkg, ok := pkgs["db"]
	require.True(t, ok, "expected package db sources under %s", pkgDir)

	var exportedDown []string
	for _, f := range pkg.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			name := fn.Name.Name
			if !ast.IsExported(name) {
				continue
			}
			// Forbid package-level MigrateDown and any exported method named Down
			// on Migrator (policy-free goose down).
			if name == "MigrateDown" {
				exportedDown = append(exportedDown, "func MigrateDown")
			}
			if name == "Down" && fn.Recv != nil {
				exportedDown = append(exportedDown, "method Down")
			}
		}
	}
	require.Empty(t, exportedDown,
		"exported policy-free down path(s) must be removed; use DownWithPolicy only: %v",
		exportedDown)
}

// TestDownWithPolicyIsSoleDestructivePath: live config cannot down without break-glass;
// non-live can. There is no alternate exported helper that skips the check.
func TestDownWithPolicyIsSoleDestructivePath(t *testing.T) {
	url := startPostgres(t)
	ctx := context.Background()
	require.NoError(t, studiodb.Migrate(ctx, url))

	live := studiodb.Config{DatabaseURL: url, MigrationsLive: true, BreakGlassDown: false}
	err := studiodb.Studio.DownWithPolicy(ctx, url, live)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "live")

	v, err := studiodb.Studio.CurrentVersion(ctx, url)
	require.NoError(t, err)
	require.Equal(t, int64(14), v, "live down must not apply")

	// Status remains available (non-destructive).
	st, err := studiodb.Studio.Status(ctx, url)
	require.NoError(t, err)
	require.NotEmpty(t, st)

	nonLive := studiodb.Config{DatabaseURL: url, MigrationsLive: false}
	require.NoError(t, studiodb.Studio.DownWithPolicy(ctx, url, nonLive))
	v, err = studiodb.Studio.CurrentVersion(ctx, url)
	require.NoError(t, err)
	require.Equal(t, int64(13), v)
}
