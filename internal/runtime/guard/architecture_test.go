package guard

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuardProductionFilesRestrictConcreteProtocolImports(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(".", "*.go"))
	if err != nil {
		t.Fatalf("glob guard files: %v", err)
	}

	disallowed := map[string]struct{}{
		"github.com/hicancan/njupt-net-cli/internal/httpx":       {},
		"github.com/hicancan/njupt-net-cli/internal/portal":      {},
		"github.com/hicancan/njupt-net-cli/internal/selfservice": {},
	}

	fset := token.NewFileSet()
	for _, path := range files {
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_test.go") || base == "factory.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if _, banned := disallowed[importPath]; banned {
				t.Fatalf("guard production file %s imports disallowed concrete package %s", path, importPath)
			}
		}
	}
}
