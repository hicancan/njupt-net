package workflow

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowProductionFilesAvoidConcreteTransportImports(t *testing.T) {
	t.Helper()

	patterns := []string{
		filepath.Join(".", "*.go"),
	}
	disallowed := []string{
		"github.com/hicancan/njupt-net-cli/internal/config",
		"github.com/hicancan/njupt-net-cli/internal/httpx",
		"github.com/hicancan/njupt-net-cli/internal/portal",
		"github.com/hicancan/njupt-net-cli/internal/selfservice",
	}

	matches := []string{}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %q: %v", pattern, err)
		}
		matches = append(matches, files...)
	}

	fset := token.NewFileSet()
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			for _, banned := range disallowed {
				if importPath == banned {
					t.Fatalf("workflow production file %s imports disallowed concrete package %s", path, importPath)
				}
			}
		}
	}
}
