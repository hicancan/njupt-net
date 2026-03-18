package output

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRendererPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer(&buf, "json")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	if err := renderer.Print(map[string]string{"hello": "world"}, nil); err != nil {
		t.Fatalf("print json: %v", err)
	}
	if !strings.Contains(buf.String(), `"hello": "world"`) {
		t.Fatalf("unexpected json output: %s", buf.String())
	}
}

func TestRendererPrintHuman(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer(&buf, "human")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	if err := renderer.Print(nil, func(w io.Writer) error {
		_, err := io.WriteString(w, "human output")
		return err
	}); err != nil {
		t.Fatalf("print human: %v", err)
	}
	if buf.String() != "human output" {
		t.Fatalf("unexpected human output: %q", buf.String())
	}
}

func TestNewRendererRejectsInvalidMode(t *testing.T) {
	_, err := NewRenderer(&bytes.Buffer{}, "yaml")
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
}
