package tui

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestImportGraph_NoForbiddenImports is the structural no-network / no-provider
// guard required by t1-feasibility §6. It parses every non-test source file in
// the tui package and asserts none imports a package that could perform
// network I/O, call provider APIs, read credentials, or shell out. This is what
// keeps the opt-in dashboard strictly read-only over local SQLite.
func TestImportGraph_NoForbiddenImports(t *testing.T) {
	forbidden := []string{
		"github.com/costroid/costroid/providers",
		"github.com/costroid/costroid/client",
		"net/http",
		"net",
		"os/exec",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		scanned++
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				if path == bad || strings.HasPrefix(path, bad+"/") {
					t.Errorf("%s imports forbidden package %q (metadata-only / no-network boundary)", name, path)
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no tui source files were scanned")
	}
}
