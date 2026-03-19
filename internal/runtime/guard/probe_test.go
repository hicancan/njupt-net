package guard

import "testing"

func TestSelectBestLocalIPv4PrefersCampusWLANOverTunnelRoute(t *testing.T) {
	candidates := []localIPv4Candidate{
		{InterfaceName: "singbox_tun", IP: "172.18.0.1", PrefixBits: 30},
		{InterfaceName: "WLAN", IP: "10.163.177.138", PrefixBits: 17},
		{InterfaceName: "vEthernet (Default Switch)", IP: "172.23.128.1", PrefixBits: 20},
		{InterfaceName: "win11", IP: "10.8.0.2", PrefixBits: 24},
	}

	selected, reason, ok := selectBestLocalIPv4(candidates, "172.18.0.1")
	if !ok {
		t.Fatal("expected selected candidate")
	}
	if selected != "10.163.177.138" {
		t.Fatalf("expected WLAN campus IP, got %q", selected)
	}
	if reason == "" {
		t.Fatal("expected selection reason")
	}
}

func TestSelectBestLocalIPv4FallsBackToOnlyCandidate(t *testing.T) {
	candidates := []localIPv4Candidate{
		{InterfaceName: "singbox_tun", IP: "172.18.0.1", PrefixBits: 30},
	}

	selected, reason, ok := selectBestLocalIPv4(candidates, "172.18.0.1")
	if !ok {
		t.Fatal("expected selected candidate")
	}
	if selected != "172.18.0.1" {
		t.Fatalf("expected only candidate, got %q", selected)
	}
	if reason == "" {
		t.Fatal("expected selection reason")
	}
}

func TestVirtualInterfaceScoringIsDisfavored(t *testing.T) {
	virtual, _ := scoreLocalIPv4Candidate(localIPv4Candidate{
		InterfaceName: "singbox_tun",
		IP:            "172.18.0.1",
		PrefixBits:    30,
	}, "172.18.0.1")
	physical, _ := scoreLocalIPv4Candidate(localIPv4Candidate{
		InterfaceName: "WLAN",
		IP:            "10.163.177.138",
		PrefixBits:    17,
	}, "172.18.0.1")
	if virtual >= physical {
		t.Fatalf("expected physical score to beat virtual score: virtual=%d physical=%d", virtual, physical)
	}
}
