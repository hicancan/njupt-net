package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hicancan/njupt-net/v4/network"
	"github.com/hicancan/njupt-net/v4/zfw"
)

func sessionLines(t *testing.T, output string) []sessionResponse {
	t.Helper()
	var results []sessionResponse
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var result sessionResponse
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			t.Fatalf("response is not single-line JSON: %q: %v", line, err)
		}
		results = append(results, result)
	}
	return results
}

func TestSessionFramesErrorsWithoutEndingTheConversation(t *testing.T) {
	owner := newCommandContext(options{timeout: time.Second})
	var output bytes.Buffer
	input := strings.NewReader(strings.Join([]string{
		`{"account":"B","args":["version"]}`,
		`{"account":"B","args":["p","login","--operator","wrong"]}`,
		`{"args":["zfw","operator"]}`,
		`{"args":["p","password","--new-password-file","missing"]}`,
		`{"args":["p","login","-help"]}`,
		`{"args":["version"]}`,
		`{"args":["close"]}`,
		`{"args":["unknown-after-close"]}`,
	}, "\n"))
	if code := runSession(context.Background(), owner, input, &output); code != 0 {
		t.Fatalf("session exit %d: %s", code, output.String())
	}
	results := sessionLines(t, output.String())
	if len(results) != 7 {
		t.Fatalf("responses=%d: %s", len(results), output.String())
	}
	for index, want := range []int{0, 2, 2, 2, 2, 0, 0} {
		if results[index].ExitCode != want || (results[index].Error != nil) != (want != 0) {
			t.Fatalf("response %d: %+v", index, results[index])
		}
	}
	if results[2].Error.Message != "--account is required" {
		t.Fatalf("request inherited previous account: %+v", results[2])
	}
	if !strings.Contains(results[3].Error.Message, "interactive") || strings.Contains(results[3].Error.Message, "read new password") {
		t.Fatalf("interactive command accessed a file: %+v", results[3])
	}
	if results[6].Command != "close" || !owner.closed || owner.link != nil {
		t.Fatalf("close did not terminate locally: %+v", results[6])
	}
}

func TestSessionRejectsMalformedRequestsAndGlobalOverrides(t *testing.T) {
	for _, line := range []string{
		`null`, `[]`, `{}`, `{"args":[]}`, `{"args":null}`, `{"args":"version"}`,
		`{"args":["version"],"source":"127.0.0.1"}`,
		`{"args":["version"]} {"args":["version"]}`,
		`{"account":null,"args":[]}`,
		`{"args":["--source","127.0.0.1","version"]}`,
		`{"account":"B","args":["close"]}`,
	} {
		t.Run(line, func(t *testing.T) {
			var output bytes.Buffer
			owner := newCommandContext(options{})
			input := strings.NewReader(line + "\n" + `{"args":["close"]}`)
			if code := runSession(context.Background(), owner, input, &output); code != 0 {
				t.Fatalf("session exit %d: %s", code, output.String())
			}
			results := sessionLines(t, output.String())
			if len(results) != 2 || results[0].ExitCode != 2 || results[0].Error == nil || results[1].Command != "close" {
				t.Fatalf("invalid request changed session framing: %s", output.String())
			}
		})
	}
}

func TestSessionEOFClosesAndCancellationDoesNotWaitForInput(t *testing.T) {
	owner := newCommandContext(options{})
	var output bytes.Buffer
	if code := runSession(context.Background(), owner, strings.NewReader(""), &output); code != 0 || !owner.closed || output.Len() != 0 {
		t.Fatalf("EOF: exit=%d closed=%t output=%s", code, owner.closed, output.String())
	}
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner = newCommandContext(options{})
	finished := make(chan int, 1)
	go func() { finished <- runSession(ctx, owner, input, &output) }()
	cancel()
	select {
	case code := <-finished:
		if code != 1 || !owner.closed {
			t.Fatalf("cancel: exit=%d closed=%t", code, owner.closed)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled session remained blocked on stdin")
	}
	results := sessionLines(t, output.String())
	if len(results) != 1 || results[0].Command != "close" || results[0].ExitCode != 1 {
		t.Fatalf("cancel response: %s", output.String())
	}
}

func TestSessionClosesEveryManagementSessionAndReportsFailures(t *testing.T) {
	owner := newCommandContext(options{timeout: time.Second})
	// Unauthenticated objects fail Logout without any network request. They
	// model independent cleanup failures to check aggregation and continuation.
	owner.sessions["B"], owner.sessions["W"] = &zfw.Session{}, &zfw.Session{}
	owner.order = []string{"B", "W"}
	var output bytes.Buffer
	if code := runSession(context.Background(), owner, strings.NewReader(`{"args":["close"]}`), &output); code != 1 {
		t.Fatalf("cleanup failure exit=%d output=%s", code, output.String())
	}
	results := sessionLines(t, output.String())
	if len(results) != 1 || results[0].ExitCode != 1 || results[0].Error == nil ||
		!strings.Contains(results[0].Error.Message, `"B"`) || !strings.Contains(results[0].Error.Message, `"W"`) || len(owner.sessions) != 0 {
		t.Fatalf("cleanup stopped early: %s", output.String())
	}
	if err := owner.close(context.Background()); err != nil {
		t.Fatalf("second cleanup repeated failed logouts: %v", err)
	}
}

func TestSessionRejectsOversizedFramesAndCloses(t *testing.T) {
	owner := newCommandContext(options{})
	var output bytes.Buffer
	if code := runSession(context.Background(), owner, strings.NewReader(strings.Repeat("x", 1024*1024+1)), &output); code != 1 || !owner.closed {
		t.Fatalf("oversized frame: exit=%d closed=%t", code, owner.closed)
	}
	results := sessionLines(t, output.String())
	if len(results) != 1 || results[0].Command != "close" || results[0].ExitCode != 1 {
		t.Fatalf("oversized response: %s", output.String())
	}
}

func TestCommandContextReusesPortalAndIsolatesAccounts(t *testing.T) {
	owner := newCommandContext(options{timeout: time.Second})
	link, err := network.NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	owner.link = link // Object construction only; this test issues no HTTP.
	defer owner.close(context.Background())
	pc, err := owner.portal("pc", 804)
	if err != nil {
		t.Fatal(err)
	}
	pcAgain, err := owner.portal("pc", 804)
	if err != nil || pcAgain != pc {
		t.Fatalf("portal was rebuilt: %v", err)
	}
	mobile, err := owner.portal("mobile", 804)
	if err != nil || mobile == pc {
		t.Fatalf("terminal contexts were mixed: %v", err)
	}
	httpPortal, err := owner.portal("pc", 803)
	if err != nil || httpPortal == pc {
		t.Fatalf("port contexts were mixed: %v", err)
	}
	sameHTTP, err := owner.portal("pc", 803)
	if err != nil || sameHTTP != httpPortal {
		t.Fatalf("port context was rebuilt: %v", err)
	}
	b, w := &zfw.Session{}, &zfw.Session{}
	owner.sessions["B"], owner.sessions["W"] = b, w
	for _, check := range []struct {
		alias string
		want  *zfw.Session
	}{{"B", b}, {"W", w}, {"B", b}} {
		got, err := owner.self(context.Background(), check.alias, credential{}, "")
		if err != nil || got != check.want {
			t.Fatalf("session %s was mixed or reauthenticated: %v", check.alias, err)
		}
	}
	owner.forgetExpired("B", fmt.Errorf("request failed: %w", zfw.ErrSessionExpired))
	if owner.sessions["B"] != nil || owner.sessions["W"] != w {
		t.Fatal("expired account invalidated another account")
	}
	if _, err := owner.self(context.Background(), "B", credential{}, ""); err == nil || owner.sessions["B"] != nil {
		t.Fatal("failed authentication was cached")
	}
	owner.forgetExpired("W", errors.New("ordinary operation failure"))
	if owner.sessions["W"] != w {
		t.Fatal("operation failure discarded a live management session")
	}
}

func TestCommandContextConfigurationIsOneSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	initial := `{"accounts":{"B":{"account":"one","password":"first"},"W":{"account":"two","password":"second"}}}`
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	owner := newCommandContext(options{config: path})
	_, b, err := owner.configured("B", true)
	if err != nil || b.Account != "one" {
		t.Fatalf("first account: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"accounts":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, w, err := owner.configured("W", true)
	if err != nil || w.Account != "two" {
		t.Fatalf("session credentials changed between commands: %v", err)
	}
	if _, _, err := owner.configured("", true); err == nil {
		t.Fatal("account default leaked across requests")
	}
}

func TestSessionStartupRejectsBridgeBeforeOpeningNetwork(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--self-url-file", "missing", "session"}, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "--self-url-file") {
		t.Fatalf("bridge accepted: exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}
