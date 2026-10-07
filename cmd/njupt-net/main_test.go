package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHelpNeedsNoNetworkOrCredentials(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"p", "--help"}, {"zfw", "--help"}, {"p", "password", "--help"}, {"zfw", "export", "--help"}} {
		var out, err bytes.Buffer
		if code := run(context.Background(), args, &out, &err); code != 0 || !strings.Contains(out.String(), "zfw commands") || err.Len() != 0 {
			t.Fatalf("%v help exit %d: stdout=%s stderr=%s", args, code, out.String(), err.String())
		}
	}
}

func TestVersionNeedsNoNetworkOrCredentials(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--config", "does-not-exist.json", "version"}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("version exit=%d stderr=%s", code, stderr.String())
	}
	var result struct {
		Command string      `json:"command"`
		Data    versionInfo `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Command != "version" || result.Data.Version == "" || result.Data.GoVersion != runtime.Version() || result.Data.OS != runtime.GOOS || result.Data.Arch != runtime.GOARCH {
		t.Fatalf("version=%s error=%v", stdout.String(), err)
	}
}

func TestBridgeURLFlagIsExplicitAndValidatedBeforeNetwork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge.txt")
	if err := os.WriteFile(path, []byte("http://example.com/Self/login/eportalLogin?params=private-params&timestamp=t&sign=s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, group := range []string{"p", "zfw"} {
		var stdout, stderr bytes.Buffer
		args := []string{"--self-url-file", path, group, "status"}
		if group == "zfw" {
			args[len(args)-1] = "account"
		}
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 || strings.Contains(stderr.String(), "private-params") {
			t.Fatalf("bridge flag ignored or exposed URL contents: exit=%d stderr=%s", code, stderr.String())
		}
		if strings.Contains(stderr.String(), "--interface") || strings.Contains(stderr.String(), "--account") {
			t.Fatalf("bridge validation happened after configuration or link selection: %s", stderr.String())
		}
	}
}
func TestMissingSourceIsUsageError(t *testing.T) {
	var out, err bytes.Buffer
	if code := run(context.Background(), []string{"p", "status"}, &out, &err); code != 2 || !strings.Contains(err.String(), "--interface") {
		t.Fatalf("exit %d: %s", code, err.String())
	}
}
func TestOutputDoesNotOverwriteExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.xls")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(path, []byte("replacement")); err == nil {
		t.Fatal("overwrote existing output")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "original" {
		t.Fatal("original changed")
	}
}

func TestInvalidBusinessArgumentsFailBeforeNetwork(t *testing.T) {
	checks := []struct {
		group, command string
		args           []string
	}{
		{"p", "login", []string{"--terminal", "unknown"}},
		{"p", "login", []string{"--operator", "unknown"}},
		{"zfw", "consume", []string{"--limit", "-1"}},
		{"zfw", "bills", []string{"--kind", "monthly", "--start", "2026-01-01"}},
		{"zfw", "devices", []string{"--size", "20"}},
		{"zfw", "unbind", []string{"--mac", "invalid-mac"}},
		{"zfw", "operator", []string{"--unbind", "invalid"}},
		{"zfw", "operator", []string{"--unbind", ""}},
		{"zfw", "operator", []string{"--bind", "--unbind", "cmcc"}},
	}
	for _, check := range checks {
		t.Run(check.group+" "+check.command+strings.Join(check.args, ""), func(t *testing.T) {
			var err error
			if check.group == "p" {
				_, err = pCommand(context.Background(), options{}, check.command, check.args, io.Discard)
			} else {
				_, err = zfwCommand(context.Background(), options{}, check.command, check.args)
			}
			var argument *argumentError
			if !errors.As(err, &argument) {
				t.Fatalf("expected argument error, got %v", err)
			}
		})
	}
}

func TestOperatorUnbindUsesAccountWithoutBroadbandConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"accounts":{"fixture":{"account":"campus-account","password":"campus-password"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, operator := range []string{"njxy", "cmcc"} {
		opt := options{config: path, account: "fixture"}
		_, err := zfwCommand(context.Background(), opt, "operator", []string{"--unbind", operator})
		var argument *argumentError
		if !errors.As(err, &argument) || !strings.Contains(err.Error(), "--interface") {
			t.Fatalf("unbind required broadband configuration before link selection: %v", err)
		}
		opt.account = ""
		_, err = zfwCommand(context.Background(), opt, "operator", []string{"--unbind", operator})
		if !errors.As(err, &argument) || !strings.Contains(err.Error(), "--account is required") {
			t.Fatalf("unbind did not require account identity: %v", err)
		}
	}
}

func TestFlagFailureIsOneJSONDocument(t *testing.T) {
	for _, args := range [][]string{{"--not-a-flag"}, {"p", "login", "--not-a-flag"}, {"zfw", "offline", "--session"}} {
		var out, err bytes.Buffer
		if code := run(context.Background(), args, &out, &err); code != 2 || out.Len() != 0 {
			t.Fatalf("%v: exit=%d stdout=%s stderr=%s", args, code, out.String(), err.String())
		}
		decoder := json.NewDecoder(&err)
		var value envelope
		if decodeErr := decoder.Decode(&value); decodeErr != nil || value.Error == nil || value.Error.Message == "" {
			t.Fatalf("%v: invalid error envelope: %v, %+v", args, decodeErr, value)
		}
		if decodeErr := decoder.Decode(new(any)); !errors.Is(decodeErr, io.EOF) {
			t.Fatalf("%v: duplicate output after JSON: %v", args, decodeErr)
		}
	}
}

func TestRemovedCommandsAreNotAliases(t *testing.T) {
	for _, args := range [][]string{{"p", "probe"}, {"p", "captcha"}, {"zfw", "login"}, {"zfw", "language"}} {
		var out, err bytes.Buffer
		if code := run(context.Background(), args, &out, &err); code != 2 || !strings.Contains(err.String(), "unknown") {
			t.Fatalf("%v: exit=%d stderr=%s", args, code, err.String())
		}
	}
}

func TestExportExistingOutputFailsBeforeCredentialsOrNetwork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bill.xls")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, err bytes.Buffer
	args := []string{"--config", "does-not-exist.json", "zfw", "export", "--kind", "monthly", "--year", "2026", "--output", path}
	if code := run(context.Background(), args, &out, &err); code != 2 || !strings.Contains(err.String(), "output already exists") {
		t.Fatalf("exit=%d stderr=%s", code, err.String())
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "keep" {
		t.Fatalf("output changed: %q, %v", data, readErr)
	}
}

func TestMultilinePasswordFailsBeforeCredentialsNetworkOrOutput(t *testing.T) {
	for _, password := range []string{"first\nsecond\n", "first\rsecond\r\n"} {
		directory := t.TempDir()
		input := filepath.Join(directory, "password.txt")
		output := filepath.Join(directory, "captcha.png")
		if err := os.WriteFile(input, []byte(password), 0600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		args := []string{"p", "password", "--new-password-file", input, "--captcha-output", output}
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "must contain one line") {
			t.Fatalf("exit=%d stderr=%s", code, stderr.String())
		}
		if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("captcha output created before password validation: %v", err)
		}
	}
}

func TestErrorKeepsTypedResult(t *testing.T) {
	var out, stderr bytes.Buffer
	data := struct {
		Outcome string `json:"outcome"`
	}{"unknown"}
	if code := finish(&out, &stderr, "p login", data, errors.New("submission outcome unknown")); code != 1 || out.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s", code, out.String())
	}
	var result struct {
		Command string `json:"command"`
		Data    struct {
			Outcome string `json:"outcome"`
		} `json:"data"`
		Error *errorDetail `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &result); err != nil || result.Data.Outcome != "unknown" || result.Error == nil {
		t.Fatalf("typed result lost: %s (%v)", stderr.String(), err)
	}
}

func TestCaptchaInputCanBeCancelled(t *testing.T) {
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readCaptcha(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled input: %v", err)
	}
}
