package integration

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hicancan/njupt-net/v3/p"
)

// The DNS outage is confined to this test process. Campus sessions remain online.
func TestLiveCampusWithoutDNS(t *testing.T) {
	link, cfg := liveLink(t)
	var lookups atomic.Int64
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		lookups.Add(1)
		return nil, errors.New("test DNS unavailable")
	}}
	defer func() { net.DefaultResolver = previous }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_, err := net.DefaultResolver.LookupIP(ctx, "ip4", "dns-unavailable.invalid.")
	cancel()
	if err == nil || lookups.Load() == 0 {
		t.Fatal("DNS outage fixture did not reject a lookup")
	}
	before := lookups.Load()
	portal, err := p.New(link, "pc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := portal.Configure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := portal.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, alias := range aliases(cfg) {
		t.Run(alias, func(t *testing.T) {
			s := management(t, link, cfg.Accounts[alias])
			if _, err := s.OperatorBinding(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Online(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("bridge", TestLivePortalBridge)
	if lookups.Load() != before {
		t.Fatal("campus requests attempted DNS resolution")
	}
	t.Log("portal, account management and signed bridge completed without DNS queries")
}
