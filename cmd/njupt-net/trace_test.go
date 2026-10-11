package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceWriteFailureIsSeparateFromBusinessResult(t *testing.T) {
	var out, failures bytes.Buffer
	code := finish(&out, &failures, "p status", map[string]any{"online": true}, nil, errors.New("trace writer unavailable"))
	var result envelope
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 0 || failures.Len() != 0 || result.Error != nil || result.TraceError != "trace writer unavailable" {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), failures.String())
	}
}

func TestTraceFileIsExplicitAndNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "http.jsonl")
	if err := os.WriteFile(path, []byte("existing trace\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, failures bytes.Buffer
	code := run(context.Background(), []string{"--trace", path, "version"}, &out, &failures)
	if code != 1 || out.Len() != 0 || !strings.Contains(failures.String(), "create HTTP trace") {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), failures.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "existing trace\n" {
		t.Fatalf("trace overwritten: %q %v", data, err)
	}
}

func TestTracePreservesCLIResultAndNeedsNoConfigurationForVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "http.jsonl")
	var out, failures bytes.Buffer
	code := run(context.Background(), []string{"--config", "missing.json", "--trace", path, "version"}, &out, &failures)
	if code != 0 || failures.Len() != 0 || !strings.Contains(out.String(), `"command": "version"`) {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), failures.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != 0 {
		t.Fatalf("non-network command emitted HTTP traffic: %q %v", data, err)
	}
}

func TestTraceCommandSequenceIncludesNonHTTPCommands(t *testing.T) {
	var sink bytes.Buffer
	owner := newCommandContext(options{})
	owner.trace = &sink
	for i := 1; i <= 2; i++ {
		_, _, err := owner.execute(context.Background(), "", []string{"version"}, &sink)
		if err != nil || owner.traceCommand != uint64(i) {
			t.Fatalf("command=%d err=%v", owner.traceCommand, err)
		}
	}
	if err := owner.close(context.Background()); err != nil || owner.traceCommand != 3 {
		t.Fatalf("close command=%d err=%v", owner.traceCommand, err)
	}
}
