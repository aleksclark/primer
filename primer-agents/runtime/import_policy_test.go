package agentruntime_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoInternalMAFImport ensures no production file in this package imports
// the internal/ subtree of agent-framework-go. MAF internal APIs are
// unstable, undocumented, and explicitly prohibited by the wave-1 plan.
func TestNoInternalMAFImport(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		// Skip _test.go files — only check production sources.
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if strings.Contains(path, "agent-framework-go/internal") {
					t.Errorf("%s: forbidden import of agent-framework-go/internal: %q", filepath.Base(filename), path)
				}
			}
		}
	}
}

// TestNoStockAgentToolCollect ensures no production file substitutes the stock
// agenttool.New / Collect() path for nested streaming. The custom
// StreamingChildTool must be used instead (see F3 in RUN_REPORT.md).
func TestNoStockAgentToolCollect(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0) // parse full source to find call expressions
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			// Reject any import of the stock agenttool package.
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if path == "github.com/microsoft/agent-framework-go/tool/agenttool" {
					t.Errorf("%s: forbidden import of stock agenttool (use StreamingChildTool instead)", filepath.Base(filename))
				}
			}
			// Reject any call to .Collect() in AST — belt-and-suspenders.
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name == "Collect" {
					pos := fset.Position(call.Pos())
					t.Errorf("%s:%d: forbidden .Collect() call — use StreamingChildTool for live child streaming", filepath.Base(filename), pos.Line)
				}
				return true
			})
		}
	}
}
