package network

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// These tests are intentionally not parallel: the resolver trap prevents any
// accidental dependency on the machine's DNS or an external network.
func directTestDNSCalls(t *testing.T) *atomic.Int32 {
	t.Helper()
	previous := net.DefaultResolver
	var calls atomic.Int32
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("test forbids external DNS")
	}}
	t.Cleanup(func() { net.DefaultResolver = previous })
	return &calls
}

func directTestURL(t *testing.T, serverURL, host, path string) string {
	t.Helper()
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Host = net.JoinHostPort(host, u.Port())
	u.Path = path
	return u.String()
}

func directTestLink(t *testing.T) *Link {
	t.Helper()
	link, err := NewLink("127.0.0.1", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(link.Close)
	return link
}

func TestClientForPreservesHostPortURLAndSourceWithoutDNS(t *testing.T) {
	dnsCalls := directTestDNSCalls(t)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	type observation struct{ host, remote string }
	received := make(chan observation, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- observation{r.Host, r.RemoteAddr}
		_, _ = io.WriteString(w, "direct")
	}))
	defer server.Close()
	link := directTestLink(t)
	client := link.ClientFor("njupt-p-client.invalid", netip.MustParseAddr("127.0.0.1"))
	for _, host := range []string{"njupt-p-client.invalid", "127.0.0.1"} {
		endpoint := directTestURL(t, server.URL, host, "/portal")
		data, final, _, err := Request(context.Background(), client, http.MethodGet, endpoint, nil)
		if err != nil || string(data) != "direct" {
			t.Fatalf("direct request failed: body=%q error=%v", data, err)
		}
		if final.String() != endpoint {
			t.Fatalf("logical URL was rewritten: got %q, want %q", final, endpoint)
		}
		observed := <-received
		logical, _ := url.Parse(endpoint)
		if observed.host != logical.Host {
			t.Fatalf("Host/port changed: got %q, want %q", observed.host, logical.Host)
		}
		remote, _, err := net.SplitHostPort(observed.remote)
		if err != nil || remote != link.Source() {
			t.Fatalf("wrong source: %q (%v)", observed.remote, err)
		}
	}
	if dnsCalls.Load() != 0 {
		t.Fatalf("fixed destination consulted DNS %d times", dnsCalls.Load())
	}
}

func TestClientForRejectsUnknownHostsAndInvalidDestinationsBeforeDial(t *testing.T) {
	dnsCalls := directTestDNSCalls(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	link := directTestLink(t)
	loopback := netip.MustParseAddr("127.0.0.1")
	for _, host := range []string{"other-client.invalid", "127.0.0.2"} {
		client := link.ClientFor("njupt-p-client.invalid", loopback)
		_, _, _, err := Request(context.Background(), client, http.MethodGet, directTestURL(t, server.URL, host, "/"), nil)
		if err == nil {
			t.Fatalf("unconfigured host %q was accepted", host)
		}
	}
	for _, test := range []struct {
		name, host string
		ip         netip.Addr
	}{
		{"zero address", "njupt-p-client.invalid", netip.Addr{}},
		{"IPv6", "njupt-p-client.invalid", netip.MustParseAddr("::1")},
		{"unspecified IPv4", "njupt-p-client.invalid", netip.MustParseAddr("0.0.0.0")},
		{"empty host", "", loopback},
		{"host with port", "njupt-p-client.invalid:443", loopback},
		{"host with scheme", "https://njupt-p-client.invalid", loopback},
		{"host with path", "njupt-p-client.invalid/path", loopback},
		{"host with whitespace", "njupt-p-client.invalid ", loopback},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := link.ClientFor(test.host, test.ip)
			// The literal alias is reachable; an invalid configuration must not
			// silently become an ordinary source-bound client.
			if _, _, _, err := Request(context.Background(), client, http.MethodGet, server.URL, nil); err == nil {
				t.Fatal("invalid destination configuration was accepted")
			}
		})
	}
	if requests.Load() != 0 || dnsCalls.Load() != 0 {
		t.Fatalf("rejected destination reached HTTP/DNS: HTTP=%d DNS=%d", requests.Load(), dnsCalls.Load())
	}
}

func TestClientForKeepsDestinationsAndCookiesIndependent(t *testing.T) {
	dnsCalls := directTestDNSCalls(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Host)
	}))
	defer server.Close()
	link := directTestLink(t)
	portal := link.ClientFor("njupt-p-client.invalid", netip.MustParseAddr("127.0.0.1"))
	management := link.ClientFor("njupt-zfw-client.invalid", netip.MustParseAddr("127.0.0.2"))
	if portal.Transport == management.Transport {
		t.Fatal("different fixed destinations share mutable transport state")
	}
	endpoint := directTestURL(t, server.URL, "njupt-p-client.invalid", "/")
	if _, _, _, err := Request(context.Background(), portal, http.MethodGet, endpoint, nil); err != nil {
		t.Fatalf("creating management client changed portal destination: %v", err)
	}
	freshPortal := link.ClientFor("njupt-p-client.invalid", netip.MustParseAddr("127.0.0.1"))
	if portal.Jar == nil || freshPortal.Jar == nil {
		t.Fatal("fixed destination clients need independent Cookie jars")
	}
	u, _ := url.Parse(endpoint)
	portal.Jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "portal"}})
	if len(freshPortal.Jar.Cookies(u)) != 0 {
		t.Fatal("fixed destination clients share website authentication")
	}
	if _, _, _, err := Request(context.Background(), link.Client(), http.MethodGet, server.URL, nil); err != nil {
		t.Fatalf("fixed destination changed ordinary client dialing: %v", err)
	}
	if dnsCalls.Load() != 0 {
		t.Fatalf("client isolation consulted external DNS %d times", dnsCalls.Load())
	}
}

func TestClientForLeavesOrdinaryClientDNSUnchanged(t *testing.T) {
	dnsCalls := directTestDNSCalls(t)
	link := directTestLink(t)
	_ = link.ClientFor("njupt-p-client.invalid", netip.MustParseAddr("127.0.0.1"))
	_, _, _, err := Request(context.Background(), link.Client(), http.MethodGet, "http://njupt-unmapped-client.invalid/probe", nil)
	if err == nil || dnsCalls.Load() == 0 {
		t.Fatalf("ordinary client no longer uses its resolver: DNS=%d error=%v", dnsCalls.Load(), err)
	}
}

func directTestCertificate(t *testing.T, host string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "direct client test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{host}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return tls.Certificate{Certificate: [][]byte{der, caDER}, PrivateKey: key}, roots
}

func directTestTrust(t *testing.T, client *http.Client, roots *x509.CertPool) {
	t.Helper()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", client.Transport)
	}
	config := &tls.Config{}
	if transport.TLSClientConfig != nil {
		config = transport.TLSClientConfig.Clone()
	}
	if config.InsecureSkipVerify {
		t.Fatal("fixed dialing disabled certificate verification")
	}
	config.RootCAs = roots
	transport.TLSClientConfig = config
}

func TestClientForPreservesTLSIdentityAndCertificateVerification(t *testing.T) {
	dnsCalls := directTestDNSCalls(t)
	const host = "njupt-p-client.invalid"
	certificate, roots := directTestCertificate(t, host)
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = fmt.Fprintf(w, "%s\n%s", r.TLS.ServerName, r.Host)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	link := directTestLink(t)
	address := netip.MustParseAddr("127.0.0.1")
	client := link.ClientFor(host, address)
	directTestTrust(t, client, roots)
	endpoint := directTestURL(t, server.URL, host, "/")
	data, final, _, err := Request(context.Background(), client, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(endpoint)
	if string(data) != host+"\n"+u.Host || final.String() != endpoint {
		t.Fatalf("TLS SNI/Host/URL were rewritten: body=%q final=%v", data, final)
	}
	wrong := link.ClientFor("wrong-client.invalid", address)
	directTestTrust(t, wrong, roots)
	_, _, _, err = Request(context.Background(), wrong, http.MethodGet, directTestURL(t, server.URL, "wrong-client.invalid", "/"), nil)
	var hostnameError x509.HostnameError
	if !errors.As(err, &hostnameError) {
		t.Fatalf("wrong logical TLS identity was not rejected: %v", err)
	}
	literal := link.ClientFor(host, address)
	directTestTrust(t, literal, roots)
	_, _, _, err = Request(context.Background(), literal, http.MethodGet, server.URL, nil)
	if !errors.As(err, &hostnameError) {
		t.Fatalf("literal origin did not verify its own IP identity: %v", err)
	}
	untrusted := link.ClientFor(host, address)
	_, _, _, err = Request(context.Background(), untrusted, http.MethodGet, endpoint, nil)
	var authorityError x509.UnknownAuthorityError
	if !errors.As(err, &authorityError) {
		t.Fatalf("untrusted test CA was accepted: %v", err)
	}
	if requests.Load() != 1 || dnsCalls.Load() != 0 {
		t.Fatalf("TLS failures reached HTTP or DNS: HTTP=%d DNS=%d", requests.Load(), dnsCalls.Load())
	}
}

func TestClientForKeepsRedirectSubmissionPolicy(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Redirect(w, r, "/submitted-again", status)
			}))
			defer server.Close()
			link := directTestLink(t)
			client := link.ClientFor("njupt-p-client.invalid", netip.MustParseAddr("127.0.0.1"))
			_, _, _, err := Request(context.Background(), client, http.MethodPost, directTestURL(t, server.URL, "njupt-p-client.invalid", "/submit"), url.Values{"password": {"fixture-password"}})
			if err == nil || calls.Load() != 1 {
				t.Fatalf("redirect replayed submission: calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}

func TestClientForRejectsRedirectToLiteralAlias(t *testing.T) {
	var calls atomic.Int32
	var destination string
	// Redirect to the same server, changing only the
	// origin hostname. Both names are allowed for an initial request, but
	// navigation may not move a website session between them.
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, destination, http.StatusFound)
	}))
	destination = "http://" + server.Listener.Addr().String() + "/other-origin"
	server.Start()
	defer server.Close()
	link := directTestLink(t)
	client := link.ClientFor("njupt-p-client.invalid", netip.MustParseAddr("127.0.0.1"))
	_, _, _, err := Request(context.Background(), client, http.MethodGet, directTestURL(t, server.URL, "njupt-p-client.invalid", "/"), nil)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("redirect changed the logical website origin: calls=%d error=%v", calls.Load(), err)
	}
}

func TestClientForPreservesContextCancellation(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	link := directTestLink(t)
	client := link.ClientFor("njupt-p-client.invalid", netip.MustParseAddr("127.0.0.1"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	endpoint := directTestURL(t, server.URL, "njupt-p-client.invalid", "/")
	go func() {
		_, _, _, err := Request(ctx, client, http.MethodGet, endpoint, nil)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not reach the pinned server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "test forbids external DNS") {
			t.Fatalf("lost request cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled direct request did not stop")
	}
}
