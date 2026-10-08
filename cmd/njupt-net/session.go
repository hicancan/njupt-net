package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
)

type sessionRequest struct {
	Account string   `json:"account,omitempty"`
	Args    []string `json:"args"`
}

type sessionResponse struct {
	Command  string       `json:"command"`
	Data     any          `json:"data"`
	Error    *errorDetail `json:"error,omitempty"`
	ExitCode int          `json:"exit_code"`
}

func sessionResult(command string, data any, err error) sessionResponse {
	result := sessionResponse{Command: command, Data: data, ExitCode: errorCode(err)}
	if err != nil {
		result.Error = &errorDetail{Message: err.Error()}
	}
	return result
}

func decodeSessionRequest(line []byte) (sessionRequest, error) {
	var request *sessionRequest
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request == nil {
		return sessionRequest{}, invalid("session request must be one JSON object with account and args")
	}
	if err := decoder.Decode(new(any)); err != io.EOF || len(request.Args) == 0 {
		return sessionRequest{}, invalid("session request requires one nonempty args array")
	}
	return *request, nil
}

// runSession is a sequential command transport. It adds no business workflow.
func runSession(ctx context.Context, owner *commandContext, input io.Reader, output io.Writer) int {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer owner.close(ctx)
	owner.stream = true
	type frame struct {
		line []byte
		err  error
	}
	frames := make(chan frame)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			line := bytes.Clone(scanner.Bytes())
			select {
			case frames <- frame{line: line}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case frames <- frame{err: err}:
			case <-ctx.Done():
			}
		}
	}()
	encoder := json.NewEncoder(output)
	closeSession := func(cause error, respond bool) int {
		err := errors.Join(cause, owner.close(ctx))
		if respond || err != nil {
			data := struct {
				Closed bool `json:"closed"`
			}{err == nil}
			if encoder.Encode(sessionResult("close", data, err)) != nil {
				return 1
			}
		}
		return errorCode(err)
	}
	for {
		if err := ctx.Err(); err != nil {
			return closeSession(err, true)
		}
		select {
		case <-ctx.Done():
			return closeSession(ctx.Err(), true)
		case frame, ok := <-frames:
			if !ok {
				return closeSession(nil, false)
			}
			if frame.err != nil {
				return closeSession(errors.New("cannot read session request"), true)
			}
			if err := ctx.Err(); err != nil {
				return closeSession(err, true)
			}
			request, err := decodeSessionRequest(frame.line)
			command := "session"
			var data any
			if err == nil {
				if len(request.Args) == 1 && request.Args[0] == "close" {
					if request.Account == "" {
						return closeSession(nil, true)
					}
					err = invalid("close does not select an account")
				} else {
					command, data, err = owner.execute(ctx, request.Account, request.Args, io.Discard)
					if errors.Is(err, flag.ErrHelp) {
						err = invalid("help is available outside a session")
					}
				}
			}
			if encoder.Encode(sessionResult(command, data, err)) != nil {
				return 1
			}
		}
	}
}
