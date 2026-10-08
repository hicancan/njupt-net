package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Link owns an immutable source IPv4 and its HTTP transports.
// It does not own account credentials or authenticated website state.
type Link struct {
	source     string
	transport  *http.Transport
	timeout    time.Duration
	mu         sync.Mutex
	transports []*http.Transport
}

// Source is the address used by every client created from this link.
func (n *Link) Source() string { return n.source }

// Connectivity is the result of the same-source external HTTP probe.
type Connectivity struct {
	Source   string `json:"source"`
	Internet bool   `json:"internet"`
	Probe    string `json:"probe"`
}

func NewLink(source string, timeout time.Duration) (*Link, error) {
	ip := net.ParseIP(source)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() {
		return nil, fmt.Errorf("link requires a concrete IPv4 source")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}
	dialer := &net.Dialer{LocalAddr: &net.TCPAddr{IP: ip}, Timeout: timeout}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp4", address)
	}, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout, DisableKeepAlives: true}
	return &Link{source: ip.String(), transport: transport, timeout: timeout, transports: []*http.Transport{transport}}, nil
}

// Client creates an independent CookieJar over this link's shared transport.
// The caller owns the website session and must close it through its protocol.
func (n *Link) Client() *http.Client {
	return n.client(n.transport)
}

// ClientFor connects a website directly to its deployed IPv4 address.
// Requests retain their URL, Host, TLS server name and certificate validation.
// The website hostname and its exact address are the only allowed destinations.
func (n *Link) ClientFor(host string, ip netip.Addr) *http.Client {
	transport := n.transport.Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if !ip.Is4() || ip.IsUnspecified() || host == "" || strings.ContainsAny(host, ":/\\?#@ \t\r\n") {
			return nil, fmt.Errorf("direct website connection requires a hostname and concrete IPv4")
		}
		name, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid website connection address")
		}
		if !strings.EqualFold(name, host) && name != ip.String() {
			return nil, fmt.Errorf("connection destination is outside the selected website")
		}
		return n.transport.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	n.mu.Lock()
	n.transports = append(n.transports, transport)
	n.mu.Unlock()
	return n.client(transport)
}

func (n *Link) client(transport *http.Transport) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Transport: transport, Jar: jar, Timeout: n.timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if req.Response != nil && (req.Response.StatusCode == http.StatusTemporaryRedirect || req.Response.StatusCode == http.StatusPermanentRedirect) {
			return fmt.Errorf("redirect would replay the original request")
		}
		if len(via) > 0 && (req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme || req.URL.User != nil) {
			return fmt.Errorf("unexpected redirect destination")
		}
		return nil
	}}
}

func (n *Link) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, transport := range n.transports {
		transport.CloseIdleConnections()
	}
}

// Probe checks the NCSI protocol through the same explicitly bound link.
func (n *Link) Probe(ctx context.Context) (*Connectivity, error) {
	client := n.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	data, _, _, err := Request(ctx, client, http.MethodGet, "http://www.msftconnecttest.com/connecttest.txt", nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(data)) != "Microsoft Connect Test" {
		return nil, fmt.Errorf("link connectivity probe returned unexpected content")
	}
	return &Connectivity{Source: n.Source(), Internet: true, Probe: "http://www.msftconnecttest.com/connecttest.txt"}, nil
}
