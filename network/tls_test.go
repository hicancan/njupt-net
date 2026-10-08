package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
)

func TestNewSubmissionConnectionsResumeTLSWithoutChangingIdentity(t *testing.T) {
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		t.Run(tls.VersionName(version), func(t *testing.T) {
			const host = "session-cache-client.invalid"
			certificate, roots := directTestCertificate(t, host)
			type observation struct {
				resumed bool
				remote  string
			}
			observed := make(chan observation, 3)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				observed <- observation{r.TLS.DidResume, r.RemoteAddr}
				fmt.Fprint(w, r.TLS.ServerName)
			}))
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: version, MaxVersion: version}
			server.StartTLS()
			defer server.Close()
			link := directTestLink(t)
			client := link.ClientFor(host, netip.MustParseAddr("127.0.0.1"))
			directTestTrust(t, client, roots)
			endpoint := directTestURL(t, server.URL, host, "/")
			var first string
			for number := range 2 {
				data, _, _, err := Request(context.Background(), client, http.MethodGet, endpoint, nil)
				if err != nil || string(data) != host {
					t.Fatalf("TLS submission changed its identity: body=%q error=%v", data, err)
				}
				state := <-observed
				if state.resumed != (number == 1) || number == 1 && state.remote == first {
					t.Fatalf("submission must use new TCP and resume only the second handshake: %+v", state)
				}
				first = state.remote
			}
			if _, _, _, err := Read(context.Background(), client, endpoint, nil); err != nil {
				t.Fatal(err)
			}
			if state := <-observed; !state.resumed || state.remote == first {
				t.Fatalf("read pool did not share the TLS session cache on its own connection: %+v", state)
			}
			wrong := link.ClientFor("wrong-session-cache.invalid", netip.MustParseAddr("127.0.0.1"))
			directTestTrust(t, wrong, roots)
			_, _, _, err := Request(context.Background(), wrong, http.MethodGet, directTestURL(t, server.URL, "wrong-session-cache.invalid", "/"), nil)
			var hostnameError x509.HostnameError
			if !errors.As(err, &hostnameError) {
				t.Fatalf("TLS session cache bypassed hostname verification: %v", err)
			}
			untrusted := link.ClientFor(host, netip.MustParseAddr("127.0.0.1"))
			_, _, _, err = Request(context.Background(), untrusted, http.MethodGet, endpoint, nil)
			var authorityError x509.UnknownAuthorityError
			if !errors.As(err, &authorityError) {
				t.Fatalf("TLS session cache bypassed the current trusted roots: %v", err)
			}
		})
	}
}

func TestResumedTLSDoesNotReplayALostSubmission(t *testing.T) {
	var submissions atomic.Int32
	var resumed atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/warm" {
			fmt.Fprint(w, "ok")
			return
		}
		submissions.Add(1)
		resumed.Store(r.TLS.DidResume)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		connection.Close()
	}))
	defer server.Close()
	link := directTestLink(t)
	link.requests.single.TLSClientConfig.RootCAs = x509.NewCertPool()
	link.requests.single.TLSClientConfig.RootCAs.AddCert(server.Certificate())
	client := link.Client()
	if _, _, _, err := Request(context.Background(), client, http.MethodGet, server.URL+"/warm", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Request(context.Background(), client, http.MethodGet, server.URL+"/modify", nil); err == nil {
		t.Fatal("lost mutation response was treated as successful")
	}
	if submissions.Load() != 1 || !resumed.Load() {
		t.Fatalf("resumed TLS changed single-submission semantics: requests=%d resumed=%t", submissions.Load(), resumed.Load())
	}
}
