package selfservice

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestParserProductionFilesAvoidTransportSideEffects(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(".", "parser_*.go"))
	if err != nil {
		t.Fatalf("glob parser files: %v", err)
	}

	disallowed := map[string]struct{}{
		"context": {},
		"github.com/hicancan/njupt-net-cli/internal/httpx": {},
	}

	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if _, banned := disallowed[importPath]; banned {
				t.Fatalf("parser production file %s imports disallowed package %s", path, importPath)
			}
		}
	}
}
