package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Mode controls the renderer output format.
type Mode string

const (
	ModeHuman Mode = "human"
	ModeJSON  Mode = "json"
)

// Renderer centralizes all machine vs human output decisions.
type Renderer struct {
	mode Mode
	out  io.Writer
}

// NewRenderer validates the requested mode and returns a renderer.
func NewRenderer(out io.Writer, mode string) (*Renderer, error) {
	normalized := Mode(strings.ToLower(strings.TrimSpace(mode)))
	if normalized == "" {
		normalized = ModeHuman
	}
	switch normalized {
	case ModeHuman, ModeJSON:
		return &Renderer{mode: normalized, out: out}, nil
	default:
		return nil, fmt.Errorf("unsupported output mode %q", mode)
	}
}

func (r *Renderer) Mode() Mode {
	return r.mode
}

// Print chooses between JSON serialization and the provided human renderer.
func (r *Renderer) Print(payload any, human func(io.Writer) error) error {
	if r.mode == ModeJSON {
		enc := json.NewEncoder(r.out)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	if human == nil {
		return nil
	}
	return human(r.out)
}

// Line prints a single human-readable line.
func (r *Renderer) Line(format string, args ...any) error {
	_, err := fmt.Fprintf(r.out, format+"\n", args...)
	return err
}
