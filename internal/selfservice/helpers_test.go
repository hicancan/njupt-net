package selfservice

import (
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func TestLooksLikeErrorMessage(t *testing.T) {
	if !looksLikeErrorMessage("账号或密码错误") {
		t.Fatal("expected chinese error text to be detected")
	}
	if looksLikeErrorMessage("all good") {
		t.Fatal("did not expect normal text to be detected as error")
	}
}

func TestToStringAndBoolFromJSON(t *testing.T) {
	if got := kernel.ToString(12); got != "12" {
		t.Fatalf("unexpected toString int: %q", got)
	}
	if got := kernel.ToString(float64(1.5)); got != "1.5" {
		t.Fatalf("unexpected toString float: %q", got)
	}
	if got := kernel.ToString(true); got != "true" {
		t.Fatalf("unexpected toString bool: %q", got)
	}
	if !boolFromJSON(true) || !boolFromJSON("1") || !boolFromJSON("true") {
		t.Fatal("expected truthy values")
	}
	if boolFromJSON("0") || boolFromJSON(false) {
		t.Fatal("expected falsy values")
	}
}

func TestEnsureSessionAndRawCapture(t *testing.T) {
	var client *Client
	if err := client.ensureSession("self.test"); err == nil {
		t.Fatal("expected nil session error")
	}
	if kernel.CaptureRaw(nil) != nil {
		t.Fatal("expected nil raw capture for nil response")
	}
	resp := &kernel.SessionResponse{StatusCode: 200, FinalURL: "/done", Body: []byte("body")}
	if capture := kernel.CaptureRaw(resp); capture == nil || capture.Status != 200 || capture.Body != "body" {
		t.Fatalf("unexpected raw capture: %#v", capture)
	}
}
