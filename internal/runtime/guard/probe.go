package guard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type connectivityProbe struct {
	url      string
	validate func(*http.Response, []byte) bool
}

// Probe implements concurrent connectivity checks and local IPv4 detection.
type Probe struct {
	timeout time.Duration
	probes  []connectivityProbe
}

// NewProbe returns the supported connectivity prober.
func NewProbe(timeout time.Duration) *Probe {
	return &Probe{
		timeout: timeout,
		probes: []connectivityProbe{
			{
				url: "http://connectivitycheck.gstatic.com/generate_204",
				validate: func(resp *http.Response, _ []byte) bool {
					return resp.StatusCode == http.StatusNoContent
				},
			},
			{
				url: "http://captive.apple.com/hotspot-detect.html",
				validate: func(resp *http.Response, body []byte) bool {
					return resp.StatusCode == http.StatusOK && strings.Contains(string(body), "Success")
				},
			},
			{
				url: "http://www.msftconnecttest.com/connecttest.txt",
				validate: func(resp *http.Response, body []byte) bool {
					return resp.StatusCode == http.StatusOK && strings.Contains(string(body), "Microsoft Connect Test")
				},
			},
		},
	}
}

// CheckConnectivity runs concurrent probes and returns on the first success.
func (p *Probe) CheckConnectivity(ctx context.Context) (bool, string) {
	type result struct {
		ok      bool
		message string
	}
	results := make(chan result, len(p.probes))
	for _, probe := range p.probes {
		go func(probe connectivityProbe) {
			ok, message := p.runProbe(ctx, probe)
			results <- result{ok: ok, message: message}
		}(probe)
	}

	failures := make([]string, 0, len(p.probes))
	for range p.probes {
		select {
		case <-ctx.Done():
			return false, "connectivity probe canceled"
		case result := <-results:
			if result.ok {
				return true, result.message
			}
			failures = append(failures, result.message)
		}
	}
	return false, fmt.Sprintf("all connectivity probes failed within %.1fs: %s", p.timeout.Seconds(), strings.Join(failures, "; "))
}

func (p *Probe) runProbe(ctx context.Context, probe connectivityProbe) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.url, nil)
	if err != nil {
		return false, probe.url + " -> request create failed"
	}
	client := &http.Client{Timeout: p.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false, probe.url + " -> " + typeMessage(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return false, probe.url + " -> body read failed"
	}
	if probe.validate(resp, body) {
		return true, "connectivity ok via " + probe.url
	}
	return false, fmt.Sprintf("%s -> unexpected response status=%d", probe.url, resp.StatusCode)
}

// DetectLocalIPv4 returns the preferred local IPv4 used for outbound traffic.
func (p *Probe) DetectLocalIPv4(ctx context.Context) (string, error) {
	targets := []string{"223.5.5.5:80", "1.1.1.1:80", "8.8.8.8:80"}
	var lastErr error
	for _, target := range targets {
		dialer := &net.Dialer{Timeout: minDuration(p.timeout, 2*time.Second)}
		conn, err := dialer.DialContext(ctx, "udp4", target)
		if err != nil {
			lastErr = err
			continue
		}
		local := conn.LocalAddr()
		_ = conn.Close()
		udpAddr, ok := local.(*net.UDPAddr)
		if !ok || udpAddr == nil {
			continue
		}
		ip := strings.TrimSpace(udpAddr.IP.String())
		if isCandidateIPv4(ip) {
			return ip, nil
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no suitable IPv4 detected")
	}
	return "", lastErr
}

func isCandidateIPv4(ip string) bool {
	if ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return false
	}
	if strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "169.254.") || ip == "192.168.137.1" {
		return false
	}
	return true
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func typeMessage(err error) string {
	if err == nil {
		return "unknown error"
	}
	return strings.TrimSpace(fmt.Sprintf("%T", err))
}
