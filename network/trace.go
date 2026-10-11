package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// SetTrace enables one JSON line per native HTTP request, including body reads.
// A nil writer disables tracing. Existing and future clients share this sink.
// The caller owns the writer; tracing never closes it or changes a request.
func (n *Link) SetTrace(writer io.Writer) {
	if writer == nil {
		n.trace.target.Store(nil)
		return
	}
	n.trace.target.Store(&traceTarget{writer: writer})
}

// TraceError reports the first diagnostic write failure. HTTP operations keep
// their original results even when the diagnostic writer fails.
func (n *Link) TraceError() error {
	n.trace.errorMu.Lock()
	defer n.trace.errorMu.Unlock()
	return n.trace.writeError
}

type traceCommandKey struct{}
type traceCommand struct{ id, command string }

type tracePayloadKey struct{}
type tracePayloadPolicy func([]byte) (map[string]any, string)

// WithTracePayload selects the protocol owner's safe response projection.
// The network layer does not decode or retain website-specific response data.
// A policy is used only while the client's optional diagnostic sink is enabled.
func WithTracePayload(ctx context.Context, policy func([]byte) (map[string]any, string)) context.Context {
	return context.WithValue(ctx, tracePayloadKey{}, tracePayloadPolicy(policy))
}

// TraceEnabled reports whether this Link client's optional sink is enabled.
// Protocol owners can skip allocating response policies when tracing is off.
func TraceEnabled(client *http.Client) bool {
	transport, ok := client.Transport.(*linkTransport)
	return ok && transport.trace != nil && transport.trace.target.Load() != nil
}

// WithTraceCommand correlates the HTTP requests made by one CLI operation.
// Both values are labels; credentials must not be included in them.
func WithTraceCommand(ctx context.Context, id, command string) context.Context {
	return context.WithValue(ctx, traceCommandKey{}, traceCommand{id, command})
}

type traceState struct {
	source         string
	interfaceIndex int
	target         atomic.Pointer[traceTarget]
	sequence       atomic.Uint64
	errorMu        sync.Mutex
	writeError     error
}

type traceTarget struct {
	mu     sync.Mutex
	writer io.Writer
}

type tracePhase struct {
	StartedAt   string       `json:"started_at"`
	CompletedAt string       `json:"completed_at"`
	ElapsedMS   float64      `json:"elapsed_ms"`
	Address     string       `json:"address,omitempty"`
	Errors      []traceError `json:"errors,omitempty"`
}

type traceError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type traceConnection struct {
	LocalAddress  string `json:"local_address"`
	RemoteAddress string `json:"remote_address"`
	Reused        bool   `json:"reused"`
}

type traceRecord struct {
	RequestID      uint64            `json:"request_id"`
	CommandID      string            `json:"command_id,omitempty"`
	Command        string            `json:"command,omitempty"`
	StartedAt      string            `json:"started_at"`
	StartedAtUTC   string            `json:"started_at_utc"`
	CompletedAt    string            `json:"completed_at"`
	Method         string            `json:"method"`
	URL            string            `json:"url"`
	Source         string            `json:"source"`
	InterfaceIndex int               `json:"interface_index"`
	Connections    []traceConnection `json:"connections,omitempty"`
	DNS            []tracePhase      `json:"dns,omitempty"`
	Connect        []tracePhase      `json:"connect,omitempty"`
	TLS            []tracePhase      `json:"tls,omitempty"`
	FirstByte      *tracePhase       `json:"first_byte,omitempty"`
	BodyRead       *tracePhase       `json:"body_read,omitempty"`
	ElapsedMS      float64           `json:"elapsed_ms"`
	StatusCode     int               `json:"status_code,omitempty"`
	ContentType    string            `json:"content_type,omitempty"`
	BodyBytes      int64             `json:"body_bytes"`
	BodyComplete   bool              `json:"body_complete"`
	BodyShape      string            `json:"body_shape,omitempty"`
	Payload        map[string]any    `json:"payload,omitempty"`
	Errors         []traceError      `json:"errors,omitempty"`
}

type requestTrace struct {
	mu                          sync.Mutex
	state                       *traceState
	target                      *traceTarget
	started, wrote, readStarted time.Time
	dnsStarted, tlsStarted      time.Time
	connectStarted              map[string][]time.Time
	readEnded                   time.Time
	readError                   error
	record                      traceRecord
	secrets                     []string
	payloadPolicy               tracePayloadPolicy
	data                        []byte
	finished                    bool
	complete                    bool
}

const tracePayloadLimit = 256 * 1024

func traceRoundTrip(state *traceState, transport http.RoundTripper, req *http.Request) (*http.Response, error) {
	if state == nil {
		return transport.RoundTrip(req)
	}
	target := state.target.Load()
	if target == nil {
		return transport.RoundTrip(req)
	}
	started := time.Now()
	command, _ := req.Context().Value(traceCommandKey{}).(traceCommand)
	trace := &requestTrace{state: state, target: target, started: started, secrets: traceSecrets(req),
		record: traceRecord{RequestID: state.sequence.Add(1), CommandID: command.id, Command: command.command,
			StartedAt: traceTimestamp(started), StartedAtUTC: started.UTC().Format(time.RFC3339Nano),
			Method: req.Method, URL: traceURL(req.URL), Source: state.source, InterfaceIndex: state.interfaceIndex}}
	trace.payloadPolicy, _ = req.Context().Value(tracePayloadKey{}).(tracePayloadPolicy)
	traced := req.WithContext(httptrace.WithClientTrace(req.Context(), trace.clientTrace()))
	response, err := transport.RoundTrip(traced)
	if err != nil {
		trace.finish(err, false)
		return response, err
	}
	trace.mu.Lock()
	trace.record.StatusCode = response.StatusCode
	if mediaType, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type")); parseErr == nil {
		trace.record.ContentType = mediaType
	}
	trace.mu.Unlock()
	response.Body = &traceBody{body: response.Body, trace: trace}
	return response, nil
}

func (t *requestTrace) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.finished {
				t.dnsStarted = time.Now()
			}
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.finished && !t.dnsStarted.IsZero() {
				phase := makeTracePhase(t.dnsStarted, time.Now())
				phase.Errors = t.errorChain(info.Err)
				t.record.DNS = append(t.record.DNS, phase)
			}
		},
		ConnectStart: func(network, address string) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.finished {
				return
			}
			if t.connectStarted == nil {
				t.connectStarted = make(map[string][]time.Time)
			}
			key := network + "\x00" + address
			t.connectStarted[key] = append(t.connectStarted[key], time.Now())
		},
		ConnectDone: func(network, address string, err error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.finished {
				return
			}
			key := network + "\x00" + address
			starts := t.connectStarted[key]
			if len(starts) == 0 {
				return
			}
			t.connectStarted[key] = starts[1:]
			phase := makeTracePhase(starts[0], time.Now())
			phase.Address, phase.Errors = t.clean(address), t.errorChain(err)
			t.record.Connect = append(t.record.Connect, phase)
		},
		TLSHandshakeStart: func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.finished {
				t.tlsStarted = time.Now()
			}
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.finished && !t.tlsStarted.IsZero() {
				phase := makeTracePhase(t.tlsStarted, time.Now())
				phase.Errors = t.errorChain(err)
				t.record.TLS = append(t.record.TLS, phase)
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.finished {
				return
			}
			t.record.Connections = append(t.record.Connections, traceConnection{LocalAddress: info.Conn.LocalAddr().String(), RemoteAddress: info.Conn.RemoteAddr().String(), Reused: info.Reused})
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if !t.finished {
				t.wrote = time.Now()
			}
		},
		GotFirstResponseByte: func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.finished {
				return
			}
			start := t.wrote
			if start.IsZero() {
				start = t.started
			}
			phase := makeTracePhase(start, time.Now())
			t.record.FirstByte = &phase
		},
	}
}

type traceBody struct {
	body  io.ReadCloser
	trace *requestTrace
}

func (b *traceBody) Read(data []byte) (int, error) {
	t := b.trace
	t.mu.Lock()
	if t.readStarted.IsZero() {
		t.readStarted = time.Now()
	}
	t.mu.Unlock()
	n, err := b.body.Read(data)
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return n, err
	}
	t.record.BodyBytes += int64(n)
	if t.payloadPolicy != nil && len(t.data)+n <= tracePayloadLimit {
		t.data = append(t.data, data[:n]...)
	} else if t.payloadPolicy != nil {
		t.payloadPolicy, t.data = nil, nil
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			t.complete = true
		} else {
			t.readError = err
		}
		t.readEnded = time.Now()
	}
	t.mu.Unlock()
	return n, err
}

func (b *traceBody) Close() error {
	err := b.body.Close()
	b.trace.mu.Lock()
	complete, readError := b.trace.complete, b.trace.readError
	b.trace.mu.Unlock()
	b.trace.finish(errors.Join(readError, err), complete)
	return err
}

func (t *requestTrace) finish(err error, complete bool) {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.finished = true
	ended := time.Now()
	t.record.CompletedAt = traceTimestamp(ended)
	t.record.ElapsedMS = durationMS(ended.Sub(t.started))
	t.record.BodyComplete = complete
	if !t.readStarted.IsZero() {
		readEnded := t.readEnded
		if readEnded.IsZero() {
			readEnded = ended
		}
		phase := makeTracePhase(t.readStarted, readEnded)
		t.record.BodyRead = &phase
	}
	t.record.BodyShape = traceBodyShape(t.record.ContentType, t.record.BodyBytes)
	if t.payloadPolicy != nil && complete {
		payload, shape := t.payloadPolicy(t.data)
		t.record.Payload, t.record.BodyShape = t.cleanPayload(payload), t.clean(shape)
	}
	t.record.Errors = t.errorChain(err)
	record := t.record
	t.data = nil
	t.mu.Unlock()
	t.target.mu.Lock()
	writeErr := json.NewEncoder(t.target.writer).Encode(record)
	t.target.mu.Unlock()
	if writeErr != nil {
		t.state.errorMu.Lock()
		if t.state.writeError == nil {
			t.state.writeError = fmt.Errorf("write HTTP diagnostic trace: %w", writeErr)
		}
		t.state.errorMu.Unlock()
	}
}

func (t *requestTrace) cleanPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	var clean func(any, int) any
	clean = func(value any, depth int) any {
		if depth >= 16 {
			return nil
		}
		switch value := value.(type) {
		case string:
			return t.clean(value)
		case map[string]any:
			result := make(map[string]any, len(value))
			for key, item := range value {
				result[t.clean(key)] = clean(item, depth+1)
			}
			return result
		case []any:
			result := make([]any, len(value))
			for i, item := range value {
				result[i] = clean(item, depth+1)
			}
			return result
		case json.Number, bool, int, int64, uint64, float64, nil:
			return value
		default:
			return nil
		}
	}
	return clean(payload, 0).(map[string]any)
}

func (t *requestTrace) errorChain(err error) []traceError {
	if err == nil {
		return nil
	}
	var result []traceError
	var visit func(error, int)
	visit = func(current error, depth int) {
		if current == nil || depth >= 16 || len(result) >= 64 {
			return
		}
		result = append(result, traceError{Type: reflect.TypeOf(current).String(), Message: t.errorMessage(current)})
		if joined, ok := current.(interface{ Unwrap() []error }); ok {
			for _, nested := range joined.Unwrap() {
				visit(nested, depth+1)
			}
		} else {
			visit(errors.Unwrap(current), depth+1)
		}
	}
	visit(err, 0)
	return result
}

func (t *requestTrace) clean(value string) string {
	for _, secret := range t.secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "<redacted>")
		}
	}
	// A response can carry a new signed URL, including in embedded error text.
	value = traceURLPattern.ReplaceAllStringFunc(value, func(raw string) string {
		if u, err := url.Parse(raw); err == nil {
			return traceURL(u)
		}
		return "<url>"
	})
	value = traceSecretPattern.ReplaceAllString(value, "${1}<redacted>")
	return safeEndpointPath(value)
}

// Unknown error implementations can contain response bodies or fresh tokens.
// Preserve their types and unwrap chain, with text only for standard network
// metadata and terminal errors whose messages do not include arbitrary data.
func (t *requestTrace) errorMessage(err error) string {
	switch value := err.(type) {
	case *url.Error:
		endpoint := "<url>"
		if u, parseErr := url.Parse(value.URL); parseErr == nil {
			endpoint = traceURL(u)
		}
		return t.clean(value.Op) + " " + endpoint
	case *net.OpError:
		message := value.Op + " " + value.Net
		if value.Source != nil {
			message += " " + value.Source.String()
		}
		if value.Addr != nil {
			message += " -> " + value.Addr.String()
		}
		return t.clean(message)
	case *net.DNSError:
		message := "DNS lookup " + value.Name
		if value.IsNotFound {
			message += ": name not found"
		} else if value.IsTimeout {
			message += ": timeout"
		} else if value.IsTemporary {
			message += ": temporary failure"
		}
		return t.clean(message)
	case *os.SyscallError:
		return t.clean(value.Syscall)
	case syscall.Errno:
		return value.Error()
	case *tls.CertificateVerificationError:
		return "TLS certificate verification failed"
	case x509.UnknownAuthorityError:
		return "certificate signed by unknown authority"
	case x509.HostnameError:
		return "certificate does not match hostname " + t.clean(value.Host)
	case x509.CertificateInvalidError:
		return fmt.Sprintf("invalid certificate: reason %d", value.Reason)
	case tls.RecordHeaderError:
		return "invalid TLS record header"
	case tls.AlertError:
		return value.Error()
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe, net.ErrClosed} {
		if err == known {
			return known.Error()
		}
	}
	return "error details omitted"
}

var traceURLPattern = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)
var traceSecretPattern = regexp.MustCompile(`(?i)((?:[\w-]*(?:password|passwd|pwd|cookie|token|csrf|nonce|signature|sign|authorization|sessionid|session_id)[\w-]*|fldextra[24]|encpwd|rcn)["']?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;)&}]+)`)

func traceSecrets(req *http.Request) []string {
	secrets := []string{req.URL.String()}
	add := func(value string) {
		if value == "" {
			return
		}
		secrets = append(secrets, value, url.QueryEscape(value), url.PathEscape(value))
	}
	collect := func(values url.Values) {
		for key, values := range values {
			if traceSecretKey(key) {
				for _, value := range values {
					add(value)
				}
			}
		}
	}
	collect(req.URL.Query())
	if req.URL.User != nil {
		add(req.URL.User.Username())
		if password, ok := req.URL.User.Password(); ok {
			add(password)
		}
	}
	for key, values := range req.Header {
		if traceSecretKey(key) {
			for _, value := range values {
				add(value)
			}
		}
	}
	for _, cookie := range req.Cookies() {
		add(cookie.Value)
	}
	if req.GetBody != nil && strings.HasPrefix(req.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		body, err := req.GetBody()
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(body, 64*1024+1))
			body.Close()
			if readErr == nil && len(data) <= 64*1024 {
				if values, parseErr := url.ParseQuery(string(data)); parseErr == nil {
					collect(values)
				}
			}
		}
	}
	return secrets
}

func traceSecretKey(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"password", "passwd", "pwd", "cookie", "token", "csrf", "nonce", "sign", "authorization", "sessionid", "session_id"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return key == "v" || key == "rcn" || key == "encpwd" || key == "fldextra2" || key == "fldextra4"
}

func traceURL(u *url.URL) string {
	safe := url.URL{Scheme: u.Scheme, Host: u.Host, Path: safeEndpointPath(u.Path)}
	return safe.String()
}

func traceBodyShape(contentType string, length int64) string {
	if length == 0 {
		return "empty"
	}
	contentType = strings.ToLower(contentType)
	switch {
	case strings.Contains(contentType, "html"):
		return "html"
	case strings.Contains(contentType, "json"):
		return "json"
	case strings.Contains(contentType, "javascript"):
		return "javascript"
	case strings.HasPrefix(contentType, "text/"):
		return "text"
	default:
		return "binary"
	}
}

func traceTimestamp(value time.Time) string  { return value.Format(time.RFC3339Nano) }
func durationMS(value time.Duration) float64 { return float64(value) / float64(time.Millisecond) }
func makeTracePhase(start, end time.Time) tracePhase {
	return tracePhase{StartedAt: traceTimestamp(start), CompletedAt: traceTimestamp(end), ElapsedMS: durationMS(end.Sub(start))}
}
