package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type envelope struct {
	Command string       `json:"command"`
	Data    any          `json:"data"`
	Error   *errorDetail `json:"error,omitempty"`
}

type errorDetail struct {
	Message string `json:"message"`
}

type fileResult struct {
	Output string `json:"output"`
	Bytes  int    `json:"bytes"`
	Format string `json:"format"`
}

func finish(stdout, stderr io.Writer, command string, data any, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if err != nil {
		code := errorCode(err)
		_ = json.NewEncoder(stderr).Encode(envelope{Command: command, Data: data, Error: &errorDetail{Message: err.Error()}})
		return code
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(envelope{Command: command, Data: data}); err != nil {
		_ = json.NewEncoder(stderr).Encode(envelope{Command: command, Error: &errorDetail{Message: err.Error()}})
		return 1
	}
	return 0
}

func errorCode(err error) int {
	if err == nil {
		return 0
	}
	var argument *argumentError
	if errors.As(err, &argument) {
		return 2
	}
	return 1
}

func readCaptcha(ctx context.Context, input io.Reader) (string, error) {
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := bufio.NewReader(input).ReadString('\n')
		if errors.Is(err, io.EOF) && text != "" {
			err = nil
		}
		done <- result{text, err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-done:
		return value.text, value.err
	}
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr, os.Remove(path))
	}
	return nil
}

func outputAvailable(path string) error {
	if path == "" {
		return invalid("an output path is required")
	}
	if _, err := os.Lstat(path); err == nil {
		return invalid("output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !parent.IsDir() {
		return invalid("output parent is not a directory")
	}
	return nil
}
