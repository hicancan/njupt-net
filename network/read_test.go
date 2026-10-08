package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadClientForPreservesTLSIdentityWithoutDNS(t *testing.T) {
	dnsCalls := directTestDNSCalls(t)
	const host = "read-tls-client.invalid"
	certificate, roots := directTestCertificate(t, host)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s", r.TLS.ServerName, r.Host)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	link := directTestLink(t)
	client := link.ClientFor(host, netip.MustParseAddr("127.0.0.1"))
	directTestTrust(t, client, roots)
	endpoint := directTestURL(t, server.URL, host, "/")
	for range 2 {
		data, final, _, err := Read(context.Background(), client, endpoint, nil)
		if err != nil || string(data) != host+" "+final.Host || final.String() != endpoint {
			t.Fatalf("read connection changed TLS identity: data=%q final=%v error=%v", data, final, err)
		}
	}
	wrong := link.ClientFor("wrong-read-client.invalid", netip.MustParseAddr("127.0.0.1"))
	directTestTrust(t, wrong, roots)
	_, _, _, err := Read(context.Background(), wrong, directTestURL(t, server.URL, "wrong-read-client.invalid", "/"), nil)
	var hostnameError x509.HostnameError
	if !errors.As(err, &hostnameError) {
		t.Fatalf("read pool did not verify logical TLS hostname: %v", err)
	}
	if dnsCalls.Load() != 0 {
		t.Fatal("fixed read connection consulted DNS")
	}
}

func TestReadReusesConnectionsAndRequestUsesItsOwnFreshConnection(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprintf("direct=%t", direct), func(t *testing.T) {
			addresses := make(chan string, 5)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				addresses <- r.RemoteAddr
				fmt.Fprint(w, "ok")
			}))
			defer server.Close()
			link := directTestLink(t)
			client, endpoint := link.Client(), server.URL
			if direct {
				client = link.ClientFor("read-client.invalid", netip.MustParseAddr("127.0.0.1"))
				endpoint = directTestURL(t, server.URL, "read-client.invalid", "/")
			}
			read := func() string {
				t.Helper()
				if _, _, _, err := Read(context.Background(), client, endpoint, nil); err != nil {
					t.Fatal(err)
				}
				return <-addresses
			}
			first := read()
			if second := read(); second != first {
				t.Fatal("read-only requests did not reuse their connection")
			}
			var mutations []string
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				if _, _, _, err := Request(context.Background(), client, method, endpoint, nil); err != nil {
					t.Fatal(err)
				}
				address := <-addresses
				if address == first || len(mutations) != 0 && address == mutations[0] {
					t.Fatal("a submission reused a read or submission connection")
				}
				mutations = append(mutations, address)
			}
			if last := read(); last != first {
				t.Fatal("submissions changed the independent read connection pool")
			}
		})
	}
}

func TestClientForSharesSiteConnectionsAndIsolatesCookies(t *testing.T) {
	type observed struct{ remote, cookie string }
	requests := make(chan observed, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- observed{r.RemoteAddr, r.Header.Get("Cookie")}
		http.SetCookie(w, &http.Cookie{Name: "identity", Value: r.URL.Query().Get("identity"), Path: "/"})
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()
	link := directTestLink(t)
	a := link.ClientFor("shared-client.invalid", netip.MustParseAddr("127.0.0.1"))
	b := link.ClientFor("SHARED-client.invalid", netip.MustParseAddr("127.0.0.1"))
	if a.Transport != b.Transport || a.Jar == b.Jar {
		t.Fatal("site transport ownership or Cookie isolation changed")
	}
	endpoint := directTestURL(t, server.URL, "shared-client.invalid", "/")
	for index, client := range []*http.Client{a, b, a} {
		if _, _, _, err := Read(context.Background(), client, endpoint, url.Values{"identity": {fmt.Sprint(index)}}); err != nil {
			t.Fatal(err)
		}
	}
	first, second, third := <-requests, <-requests, <-requests
	if first.remote != second.remote || second.remote != third.remote {
		t.Fatal("clients for the same pinned site did not share the read connection")
	}
	if first.cookie != "" || second.cookie != "" || third.cookie != "identity=0" {
		t.Fatalf("Cookie sessions were mixed: first=%q second=%q third=%q", first.cookie, second.cookie, third.cookie)
	}
	otherHost := link.ClientFor("other-client.invalid", netip.MustParseAddr("127.0.0.1"))
	otherIP := link.ClientFor("shared-client.invalid", netip.MustParseAddr("127.0.0.2"))
	if otherHost.Transport == a.Transport || otherIP.Transport == a.Transport {
		t.Fatal("different pinned sites share a transport")
	}
}

func TestRequestConnectionLossNeverReplaysGETOrPOSTAfterRead(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			var submissions atomic.Int32
			addresses := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				addresses <- r.RemoteAddr
				if r.URL.Path == "/read" {
					fmt.Fprint(w, "ok")
					return
				}
				submissions.Add(1)
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				connection.Close()
			}))
			defer server.Close()
			link := directTestLink(t)
			client := link.Client()
			if _, _, _, err := Read(context.Background(), client, server.URL+"/read", nil); err != nil {
				t.Fatal(err)
			}
			readAddress := <-addresses
			if _, _, _, err := Request(context.Background(), client, method, server.URL+"/modify", nil); err == nil {
				t.Fatal("lost submission response was treated as successful")
			}
			if address := <-addresses; address == readAddress {
				t.Fatal("submission used the established read connection")
			}
			if submissions.Load() != 1 {
				t.Fatalf("lost submission was replayed %d times", submissions.Load())
			}
		})
	}
}

func TestLinkCloseClosesOrdinaryAndFixedDestinationReadPools(t *testing.T) {
	closed := make(chan struct{}, 2)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	link := directTestLink(t)
	fixed := link.ClientFor("close-client.invalid", netip.MustParseAddr("127.0.0.1"))
	for _, request := range []struct {
		client *http.Client
		url    string
	}{
		{link.Client(), server.URL},
		{fixed, directTestURL(t, server.URL, "close-client.invalid", "/")},
	} {
		if _, _, _, err := Read(context.Background(), request.client, request.url, nil); err != nil {
			t.Fatal(err)
		}
	}
	link.Close()
	for range 2 {
		select {
		case <-closed:
		case <-time.After(2 * time.Second):
			t.Fatal("Link.Close left a read pool connection open")
		}
	}
}
