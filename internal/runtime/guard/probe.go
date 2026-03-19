package guard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/workflow"
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

type localIPv4Candidate struct {
	InterfaceName string
	IP            string
	PrefixBits    int
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
func (p *Probe) DetectLocalIPv4(ctx context.Context) (workflow.LocalIPSelection, error) {
	routedIP, routeErr := detectRoutedIPv4(ctx, p.timeout)
	candidates, candidateErr := enumerateLocalIPv4Candidates()
	if selected, reason, ok := selectBestLocalIPv4(candidates, routedIP); ok {
		return workflow.LocalIPSelection{
			SelectedIP:      selected,
			RoutedIP:        routedIP,
			SelectionReason: reason,
		}, nil
	}

	if isCandidateIPv4(routedIP) {
		return workflow.LocalIPSelection{
			SelectedIP:      routedIP,
			RoutedIP:        routedIP,
			SelectionReason: "routed-fallback",
		}, nil
	}
	if routeErr != nil {
		return workflow.LocalIPSelection{}, routeErr
	}
	if candidateErr != nil {
		return workflow.LocalIPSelection{}, candidateErr
	}
	return workflow.LocalIPSelection{}, errors.New("no suitable IPv4 detected")
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

func detectRoutedIPv4(ctx context.Context, timeout time.Duration) (string, error) {
	targets := []string{"223.5.5.5:80", "1.1.1.1:80", "8.8.8.8:80"}
	var lastErr error
	for _, target := range targets {
		dialer := &net.Dialer{Timeout: minDuration(timeout, 2*time.Second)}
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

func enumerateLocalIPv4Candidates() ([]localIPv4Candidate, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	candidates := []localIPv4Candidate{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, bits, ok := extractIPv4Candidate(addr)
			if !ok {
				continue
			}
			candidates = append(candidates, localIPv4Candidate{
				InterfaceName: iface.Name,
				IP:            ip,
				PrefixBits:    bits,
			})
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("no local IPv4 candidates found")
	}
	return candidates, nil
}

func extractIPv4Candidate(addr net.Addr) (string, int, bool) {
	switch value := addr.(type) {
	case *net.IPNet:
		ip := strings.TrimSpace(value.IP.String())
		if !isCandidateIPv4(ip) {
			return "", 0, false
		}
		ones, _ := value.Mask.Size()
		return ip, ones, true
	case *net.IPAddr:
		ip := strings.TrimSpace(value.IP.String())
		if !isCandidateIPv4(ip) {
			return "", 0, false
		}
		return ip, 32, true
	default:
		return "", 0, false
	}
}

func selectBestLocalIPv4(candidates []localIPv4Candidate, routedIP string) (string, string, bool) {
	if len(candidates) == 0 {
		return "", "", false
	}
	type rankedCandidate struct {
		localIPv4Candidate
		score  int
		reason string
	}

	ranked := make([]rankedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		score, reason := scoreLocalIPv4Candidate(candidate, routedIP)
		ranked = append(ranked, rankedCandidate{
			localIPv4Candidate: candidate,
			score:              score,
			reason:             reason,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].InterfaceName != ranked[j].InterfaceName {
			return ranked[i].InterfaceName < ranked[j].InterfaceName
		}
		return ranked[i].IP < ranked[j].IP
	})
	return ranked[0].IP, ranked[0].reason, true
}

func scoreLocalIPv4Candidate(candidate localIPv4Candidate, routedIP string) (int, string) {
	name := strings.ToLower(strings.TrimSpace(candidate.InterfaceName))
	ip := strings.TrimSpace(candidate.IP)
	score := 0
	reasons := []string{}

	if ip == routedIP {
		score += 80
		reasons = append(reasons, "matches-routed-ip")
	}
	if isPreferredCampusIPv4(ip) {
		score += 240
		reasons = append(reasons, "campus-ip")
	}
	if strings.HasPrefix(ip, "172.") {
		score += 40
		reasons = append(reasons, "private-172")
	}
	if strings.HasPrefix(ip, "192.168.") {
		score += 20
		reasons = append(reasons, "private-192")
	}
	if candidate.PrefixBits >= 30 {
		score -= 120
		reasons = append(reasons, "narrow-prefix")
	}
	if isPreferredPhysicalInterface(name) {
		score += 220
		reasons = append(reasons, "preferred-interface")
	}
	if isDisfavoredVirtualInterface(name) {
		score -= 500
		reasons = append(reasons, "virtual-interface")
	}
	return score, strings.Join(reasons, ",")
}

func isPreferredCampusIPv4(ip string) bool {
	return strings.HasPrefix(ip, "10.")
}

func isPreferredPhysicalInterface(name string) bool {
	keywords := []string{"wlan", "wi-fi", "wifi", "wireless", "ethernet", "以太网", "eth", "en"}
	for _, keyword := range keywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

func isDisfavoredVirtualInterface(name string) bool {
	keywords := []string{
		"singbox",
		"tun",
		"tap",
		"vpn",
		"wireguard",
		"wg",
		"tailscale",
		"zerotier",
		"docker",
		"veth",
		"vethernet",
		"default switch",
		"hyper-v",
		"vmware",
		"virtual",
		"wsl",
		"utun",
		"bridge",
		"br-",
	}
	for _, keyword := range keywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
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
