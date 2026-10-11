package network

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type traceTestTransport func(*http.Request) (*http.Response, error)

func (transport traceTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func traceTestRecords(t *testing.T, data string) []traceRecord {
	t.Helper()
	var records []traceRecord
	decoder := json.NewDecoder(strings.NewReader(data))
	for {
		var record traceRecord
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			return records
		}
		if err != nil {
			t.Fatalf("diagnostic output is not NDJSON: %v", err)
		}
		records = append(records, record)
	}
}

func TestTraceDisabledDoesNotInstallHooksOrReadPolicy(t *testing.T) {
	state := new(traceState)
	body := io.NopCloser(strings.NewReader("unchanged"))
	policyCalls := 0
	ctx := WithTracePayload(context.Background(), func([]byte) (map[string]any, string) { policyCalls++; return nil, "unused" })
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/", nil)
	response, err := traceRoundTrip(state, traceTestTransport(func(request *http.Request) (*http.Response, error) {
		if httptrace.ContextClientTrace(request.Context()) != nil {
			t.Fatal("disabled tracing installed hooks")
		}
		return &http.Response{Body: body}, nil
	}), request)
	if err != nil || response.Body != body || policyCalls != 0 || state.sequence.Load() != 0 {
		t.Fatalf("disabled tracing changed HTTP: response=%v error=%v policy=%d", response, err, policyCalls)
	}
}

func TestTraceSplitsRequestsAndPreservesFixedLink(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "application/json; token=header-secret")
		_, _ = io.WriteString(writer, `{"result":0,"msg":"native rejection","ret_code":2}`)
	}))
	defer server.Close()
	link := directTestLink(t)
	client := link.ClientFor("trace-client.invalid", netip.MustParseAddr("127.0.0.1"))
	var output strings.Builder
	link.SetTrace(&output)
	if !TraceEnabled(client) || !TraceEnabled(link.Client()) {
		t.Fatal("clients do not share the enabled sink")
	}
	for index, path := range []string{"/authentication", "/observation"} {
		ctx := WithTraceCommand(context.Background(), "7", "p login")
		ctx = WithTracePayload(ctx, func(body []byte) (map[string]any, string) {
			var payload map[string]any
			if json.Unmarshal(body, &payload) != nil {
				return nil, "invalid_json"
			}
			return payload, "json"
		})
		endpoint := directTestURL(t, server.URL, "trace-client.invalid", path)
		var err error
		if index == 0 {
			_, _, _, err = Request(ctx, client, http.MethodGet, endpoint, nil)
		} else {
			_, _, _, err = Read(ctx, client, endpoint, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	records := traceTestRecords(t, output.String())
	if len(records) != 2 || requests.Load() != 2 {
		t.Fatalf("HTTP count changed: requests=%d traces=%d", requests.Load(), len(records))
	}
	for index, record := range records {
		if record.RequestID != uint64(index+1) || record.CommandID != "7" || record.Command != "p login" || record.Source != "127.0.0.1" || record.InterfaceIndex <= 0 {
			t.Fatalf("request association/source missing: %+v", record)
		}
		if len(record.DNS) != 0 || len(record.Connect) != 1 || len(record.Connections) != 1 || record.FirstByte == nil || record.BodyRead == nil || !record.BodyComplete || record.StatusCode != 200 {
			t.Fatalf("fixed link phases are incomplete: %+v", record)
		}
		if !strings.HasPrefix(record.Connect[0].Address, "127.0.0.1:") || !strings.HasPrefix(record.Connections[0].LocalAddress, "127.0.0.1:") || !strings.HasPrefix(record.Connections[0].RemoteAddress, "127.0.0.1:") {
			t.Fatalf("actual socket endpoints missing: %+v", record)
		}
		if record.Payload["msg"] != "native rejection" || record.ContentType != "application/json" {
			t.Fatalf("payload/media type changed: %+v", record)
		}
	}
	link.SetTrace(nil)
	if TraceEnabled(client) {
		t.Fatal("existing client did not disable tracing")
	}
	_, _, _, err := Read(context.Background(), client, directTestURL(t, server.URL, "trace-client.invalid", "/disabled"), nil)
	if err != nil || len(traceTestRecords(t, output.String())) != 2 || requests.Load() != 3 || link.TraceError() != nil {
		t.Fatalf("disabled sink changed behavior: %v", err)
	}
}

func TestTraceTLSAndReusedReadConnection(t *testing.T) {
	const host = "trace-tls.invalid"
	certificate, roots := directTestCertificate(t, host)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { _, _ = io.WriteString(writer, "ok") }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	defer server.Close()
	link := directTestLink(t)
	client := link.ClientFor(host, netip.MustParseAddr("127.0.0.1"))
	directTestTrust(t, client, roots)
	client.Transport.(*linkTransport).trace = link.trace
	var output strings.Builder
	link.SetTrace(&output)
	for range 2 {
		if _, _, _, err := Read(context.Background(), client, directTestURL(t, server.URL, host, "/"), nil); err != nil {
			t.Fatal(err)
		}
	}
	records := traceTestRecords(t, output.String())
	if len(records) != 2 || len(records[0].TLS) != 1 || len(records[1].TLS) != 0 || len(records[1].Connect) != 0 || !records[1].Connections[0].Reused {
		t.Fatalf("TLS and read reuse are not separate phases: %+v", records)
	}
}

func TestTraceRedactsSecretsAndOmitsUnselectedBodies(t *testing.T) {
	const password = "secret-pass &+=中文"
	const broadband = "broadband-secret"
	var output strings.Builder
	state := &traceState{source: "192.0.2.1", interfaceIndex: 4}
	state.target.Store(&traceTarget{writer: &output})
	request, _ := http.NewRequest(http.MethodPost, "http://example.invalid/Self%3Bjsessionid%3Dpath-secret/submit?token=query-secret&user_password="+url.QueryEscape(password), strings.NewReader(url.Values{"FLDEXTRA2": {broadband}, "FLDEXTRA4": {"second-password"}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "Bearer auth-secret")
	request.AddCookie(&http.Cookie{Name: "SESSION", Value: "cookie-secret"})
	request = request.WithContext(WithTracePayload(request.Context(), func([]byte) (map[string]any, string) {
		return map[string]any{"result": 0, "msg": password + " " + broadband + " second-password cookie-secret prefix:https://self.invalid/Self%3Bjsessionid%3Dnew-session/bridge?token=new-token&sign=new-sign csrf=new-csrf"}, "json"
	}))
	response, err := traceRoundTrip(state, traceTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json; token=media-secret"}}, Body: io.NopCloser(strings.NewReader("body-secret"))}, nil
	}), request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	encoded := output.String()
	for _, secret := range []string{password, broadband, "second-password", "path-secret", "query-secret", "auth-secret", "cookie-secret", "new-session", "new-token", "new-sign", "new-csrf", "media-secret", "body-secret"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("trace leaked %q: %s", secret, encoded)
		}
	}
	records := traceTestRecords(t, encoded)
	if len(records) != 1 || records[0].Payload["result"] != float64(0) || !strings.Contains(records[0].URL, "jsessionid") {
		t.Fatalf("diagnostic metadata was lost: %s", encoded)
	}
	output.Reset()
	request, _ = http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	response, _ = traceRoundTrip(state, traceTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader("<input value='html-password-secret'>"))}, nil
	}), request)
	_, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if strings.Contains(output.String(), "html-password-secret") || traceTestRecords(t, output.String())[0].BodyShape != "html" {
		t.Fatal("an unselected HTML body entered the trace")
	}
}

type traceFailWriter struct{ failure error }

func (writer traceFailWriter) Write([]byte) (int, error) { return 0, writer.failure }

func TestTraceWriterFailurePreservesHTTPResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { _, _ = io.WriteString(writer, "ok") }))
	defer server.Close()
	link := directTestLink(t)
	failure := errors.New("diagnostic disk failure")
	link.SetTrace(traceFailWriter{failure})
	body, _, _, err := Read(context.Background(), link.Client(), server.URL, nil)
	if err != nil || string(body) != "ok" || !errors.Is(link.TraceError(), failure) {
		t.Fatalf("writer failure changed HTTP or was lost: body=%q http=%v trace=%v", body, err, link.TraceError())
	}
}

func TestTraceConnectFailureContainsActualEndpointAndErrorTypes(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	link := directTestLink(t)
	var output strings.Builder
	link.SetTrace(&output)
	_, _, _, err = Request(context.Background(), link.Client(), http.MethodGet, "http://"+address+"/", nil)
	if err == nil {
		t.Fatal("closed local endpoint unexpectedly connected")
	}
	records := traceTestRecords(t, output.String())
	if len(records) != 1 || len(records[0].Connect) != 1 || records[0].Connect[0].Address != address || len(records[0].Errors) == 0 || len(records[0].Connections) != 0 {
		t.Fatalf("failed dial did not preserve endpoint/error chain: %+v", records)
	}
}

func TestTraceConcurrentLinesAndLateHooks(t *testing.T) {
	var output strings.Builder
	state := &traceState{source: "192.0.2.1", interfaceIndex: 4}
	state.target.Store(&traceTarget{writer: &output})
	var pending sync.WaitGroup
	for range 24 {
		pending.Go(func() {
			request, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
			response, err := traceRoundTrip(state, traceTestTransport(func(request *http.Request) (*http.Response, error) {
				hooks := httptrace.ContextClientTrace(request.Context())
				hooks.ConnectStart("tcp4", "192.0.2.10:80")
				hooks.ConnectStart("tcp4", "192.0.2.11:80")
				hooks.ConnectDone("tcp4", "192.0.2.11:80", nil)
				hooks.ConnectDone("tcp4", "192.0.2.10:80", context.DeadlineExceeded)
				body := &traceLateBody{reader: strings.NewReader("ok"), hooks: hooks}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
			}), request)
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = io.ReadAll(response.Body)
			_ = response.Body.Close()
			body := response.Body.(*traceBody).body.(*traceLateBody)
			body.hooks.ConnectStart("tcp4", "192.0.2.99:80")
			body.hooks.ConnectDone("tcp4", "192.0.2.99:80", nil)
			body.hooks.GotFirstResponseByte()
		})
	}
	pending.Wait()
	records := traceTestRecords(t, output.String())
	ids := make(map[uint64]bool)
	if len(records) != 24 {
		t.Fatalf("concurrent writes lost lines: %d", len(records))
	}
	for _, record := range records {
		if ids[record.RequestID] || len(record.Connect) != 2 || record.Connect[0].Address != "192.0.2.11:80" || record.Connect[1].Address != "192.0.2.10:80" || len(record.Connect[1].Errors) != 1 {
			t.Fatalf("request pairing/late callbacks changed finalized trace: %+v", record)
		}
		ids[record.RequestID] = true
	}
}

type traceLateBody struct {
	reader *strings.Reader
	hooks  *httptrace.ClientTrace
}

func (body *traceLateBody) Read(data []byte) (int, error) { return body.reader.Read(data) }
func (body *traceLateBody) Close() error                  { return nil }

func TestTraceUnknownErrorTextCannotLeakNewServerSecrets(t *testing.T) {
	var output strings.Builder
	state := new(traceState)
	state.target.Store(&traceTarget{writer: &output})
	request, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	failure := errors.New("prefix:https://other.invalid/bridge?token=unknown-token header Set-Cookie:new-cookie password=new-password")
	_, observed := traceRoundTrip(state, traceTestTransport(func(*http.Request) (*http.Response, error) { return nil, failure }), request)
	if observed != failure || strings.Contains(output.String(), "unknown-token") || strings.Contains(output.String(), "new-cookie") || strings.Contains(output.String(), "new-password") {
		t.Fatalf("unknown errors leaked text or changed business error: %v %s", observed, output.String())
	}
	if records := traceTestRecords(t, output.String()); len(records) != 1 || records[0].Errors[0].Type != "*errors.errorString" {
		t.Fatal("unknown error chain type was lost")
	}
}

type traceErrorBody struct {
	reader                *strings.Reader
	readError, closeError error
}

func (body *traceErrorBody) Read(data []byte) (int, error) {
	n, err := body.reader.Read(data)
	if errors.Is(err, io.EOF) && body.readError != nil {
		return n, body.readError
	}
	return n, err
}
func (body *traceErrorBody) Close() error { return body.closeError }

func TestTracePreservesReadAndCloseErrorsInOneCompletedLine(t *testing.T) {
	for _, readError := range []error{nil, io.ErrUnexpectedEOF} {
		var output strings.Builder
		state := new(traceState)
		state.target.Store(&traceTarget{writer: &output})
		request, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
		response, err := traceRoundTrip(state, traceTestTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &traceErrorBody{reader: strings.NewReader("ok"), readError: readError, closeError: io.ErrClosedPipe}}, nil
		}), request)
		if err != nil {
			t.Fatal(err)
		}
		data, observedRead := io.ReadAll(response.Body)
		if string(data) != "ok" || observedRead != readError || response.Body.Close() != io.ErrClosedPipe {
			t.Fatal("trace changed body read or close result")
		}
		_ = response.Body.Close()
		records := traceTestRecords(t, output.String())
		if len(records) != 1 || records[0].BodyComplete != (readError == nil) || records[0].BodyBytes != 2 || !strings.Contains(output.String(), "io: read/write on closed pipe") {
			t.Fatalf("read/EOF/close was not one final observation: %+v", records)
		}
		if readError != nil && !strings.Contains(output.String(), "unexpected EOF") {
			t.Fatal("read error was lost while closing the body")
		}
	}
}

func TestTracePayloadLimitDoesNotRetainOversizedBody(t *testing.T) {
	var output strings.Builder
	state := new(traceState)
	state.target.Store(&traceTarget{writer: &output})
	policyCalls := 0
	ctx := WithTracePayload(context.Background(), func([]byte) (map[string]any, string) {
		policyCalls++
		return map[string]any{"body": "oversized-secret"}, "json"
	})
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid/", nil)
	body := strings.Repeat("x", tracePayloadLimit+17)
	response, _ := traceRoundTrip(state, traceTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}), request)
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(data) != body || policyCalls != 0 {
		t.Fatal("diagnostic capture limit changed the body or invoked the projection")
	}
	records := traceTestRecords(t, output.String())
	if len(records) != 1 || records[0].BodyBytes != int64(len(body)) || records[0].Payload != nil || strings.Contains(output.String(), "oversized-secret") {
		t.Fatal("oversized response acquired a diagnostic payload")
	}
}
