package main

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandProductionFilesAvoidTransportConstruction(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(".", "*.go"))
	if err != nil {
		t.Fatalf("glob command files: %v", err)
	}

	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if importPath == "github.com/hicancan/njupt-net-cli/internal/httpx" {
				t.Fatalf("command production file %s imports disallowed transport package %s", path, importPath)
			}
		}
	}
}
