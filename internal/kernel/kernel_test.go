package kernel

import (
	"errors"
	"testing"
)

func TestOpErrorUnwrap(t *testing.T) {
	err := &OpError{
		Op:      "self.login",
		Message: "login failed",
		Err:     ErrAuth,
	}
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("expected unwrap to expose ErrAuth, got %v", err)
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error string")
	}
}

func TestOpErrorErrorBranches(t *testing.T) {
	cases := []*OpError{
		{Op: "one", Message: "message only"},
		{Op: "two", Err: ErrPortal},
		{Op: "three"},
	}
	for _, tc := range cases {
		if tc.Error() == "" {
			t.Fatalf("expected non-empty error string for %#v", tc)
		}
	}
}

func TestCloneStateMap(t *testing.T) {
	original := map[string]string{"a": "1"}
	cloned := CloneStateMap(original)
	if cloned["a"] != "1" {
		t.Fatalf("unexpected clone: %#v", cloned)
	}
	cloned["a"] = "2"
	if original["a"] != "1" {
		t.Fatalf("expected original to remain unchanged: %#v", original)
	}
}

func TestCloneStateMapNil(t *testing.T) {
	if got := CloneStateMap(nil); got != nil {
		t.Fatalf("expected nil clone, got %#v", got)
	}
}
