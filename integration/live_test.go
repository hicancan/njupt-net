package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hicancan/njupt-net/v3/network"
	"github.com/hicancan/njupt-net/v3/p"
	"github.com/hicancan/njupt-net/v3/zfw"
)

// Live tests are explicit compositions of the public core API. Normal tests
// neither contact campus services nor change the terminal's Internet session.
type credential struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type liveConfig struct {
	Accounts map[string]credential `json:"accounts"`
}

func liveLink(t *testing.T) (*network.Link, liveConfig) {
	t.Helper()
	if os.Getenv("NJUPT_NET_LIVE") != "1" {
		t.Skip("set NJUPT_NET_LIVE=1 and NJUPT_NET_SOURCE for campus integration")
	}
	source, err := network.SourceAddress("", os.Getenv("NJUPT_NET_SOURCE"))
	if err != nil {
		t.Fatal(err)
	}
	link, err := network.NewLink(source, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(link.Close)
	path := os.Getenv("NJUPT_NET_CONFIG")
	if path == "" {
		path = filepath.Join("..", "config.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg liveConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Accounts) == 0 {
		t.Fatal("live configuration has no accounts")
	}
	return link, cfg
}

func aliases(cfg liveConfig) []string {
	result := make([]string, 0, len(cfg.Accounts))
	for alias := range cfg.Accounts {
		result = append(result, alias)
	}
	sort.Strings(result)
	return result
}

func management(t *testing.T, link *network.Link, cred credential) *zfw.Session {
	t.Helper()
	s := zfw.New(link)
	if err := s.Login(context.Background(), cred.Account, cred.Password); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.Logout(ctx); err != nil {
			t.Errorf("close owned management session: %v", err)
		}
	})
	return s
}

func TestLiveReadOnly(t *testing.T) {
	link, cfg := liveLink(t)
	ctx := context.Background()
	portal, err := p.New(link, "pc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := portal.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := portal.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Source != link.Source() {
		t.Fatal("portal status changed the selected source")
	}
	for _, alias := range aliases(cfg) {
		t.Run(alias, func(t *testing.T) {
			s := management(t, link, cfg.Accounts[alias])
			checks := []struct {
				name string
				run  func() error
			}{
				{"account", func() error { _, err := s.Overview(ctx); return err }},
				{"profile", func() error { _, err := s.Profile(ctx); return err }},
				{"online", func() error { _, err := s.Online(ctx); return err }},
				{"history", func() error { _, err := s.History(ctx); return err }},
				{"consume", func() error { _, err := s.ConsumeProtect(ctx); return err }},
				{"operator", func() error { _, err := s.OperatorBinding(ctx); return err }},
				{"mauth", func() error { _, err := s.Mauth(ctx); return err }},
				{"recharge", func() error { _, err := s.Recharge(ctx); return err }},
				{"refresh", func() error { return s.RefreshAccount(ctx) }},
			}
			for _, check := range checks {
				t.Run(check.name, func(t *testing.T) {
					if err := check.run(); err != nil {
						t.Fatal(err)
					}
				})
			}
			if _, err := s.Devices(ctx, 1, 10); err != nil {
				if !strings.Contains(err.Error(), "empty JSON response") {
					t.Errorf("devices: %v", err)
				} else {
					t.Log("known server limitation: MAC list has an empty response")
				}
			}
		})
	}
	for _, kind := range []string{"notice", "help", "agreement"} {
		t.Run(kind, func(t *testing.T) {
			if _, err := zfw.New(link).PublicPage(ctx, kind); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLiveBills(t *testing.T) {
	link, cfg := liveLink(t)
	for _, alias := range aliases(cfg) {
		t.Run(alias, func(t *testing.T) {
			s := management(t, link, cfg.Accounts[alias])
			for _, kind := range []string{"online", "monthly", "operations"} {
				t.Run(kind, func(t *testing.T) {
					q := zfw.BillQuery{Kind: kind}
					page, err := s.Bills(context.Background(), q)
					if err != nil {
						t.Fatal(err)
					}
					if page.Kind != kind {
						t.Fatal("bill response has a different kind")
					}
					for _, all := range []bool{false, true} {
						t.Run(fmt.Sprintf("all=%t", all), func(t *testing.T) {
							data, err := s.ExportBills(context.Background(), q, all)
							if err != nil {
								t.Fatal(err)
							}
							t.Logf("verified XLS response: %d bytes", len(data))
						})
					}
				})
			}
		})
	}
}

// The portal bridge and password form are explicit management login methods.
// This test consumes only a fresh bridge for the currently owned terminal.
func TestLivePortalBridge(t *testing.T) {
	link, cfg := liveLink(t)
	portal, err := p.New(link, "pc")
	if err != nil {
		t.Fatal(err)
	}
	cred, _ := ownedTerminal(t, portal, cfg)
	bridge, err := portal.SelfURL(context.Background(), cred.Account, cred.Password, 1)
	if err != nil {
		t.Fatal(err)
	}
	s := zfw.New(link)
	if err := s.LoginBridge(context.Background(), cred.Account, bridge.URL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.Logout(ctx); err != nil {
			t.Errorf("close bridge management session: %v", err)
		}
	})
	account, err := s.Overview(context.Background())
	if err != nil || account.Account != cred.Account {
		t.Fatalf("bridge did not verify the selected account: %v", err)
	}
	if _, err := s.Online(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Log("fresh portal bridge verified the expected account and subsequent same-origin management query")
}

func ownedTerminal(t *testing.T, portal *p.Portal, cfg liveConfig) (credential, string) {
	t.Helper()
	state, err := portal.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Online || state.Terminal == nil {
		t.Fatal("cycle requires an existing owned terminal to restore")
	}
	operator, account := "campus", terminalAccount(state.Terminal.Account)
	for _, suffix := range []string{"@cmcc", "@njxy"} {
		if strings.HasSuffix(account, suffix) {
			operator, account = strings.TrimPrefix(suffix, "@"), strings.TrimSuffix(account, suffix)
		}
	}
	for _, alias := range aliases(cfg) {
		if cfg.Accounts[alias].Account == account {
			return cfg.Accounts[alias], operator
		}
	}
	t.Fatal("terminal is not owned by a configured account; refusing to offline it")
	return credential{}, ""
}

// The portal reports the same account either with or without its documented
// terminal prefix. Both representations have the same account ownership.
func terminalAccount(account string) string {
	for _, prefix := range []string{",0,", ",1,"} {
		if strings.HasPrefix(account, prefix) {
			return strings.TrimPrefix(account, prefix)
		}
	}
	return account
}

func waitOffline(ctx context.Context, portal *p.Portal) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		state, err := portal.Status(ctx)
		if err != nil {
			return err
		}
		if !state.Online {
			return nil
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Recovery is independent of the operation under test and never hides failure.
func restoreTerminal(t *testing.T, link *network.Link, cred credential, operator string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		portal, err := p.New(link, "pc")
		if err != nil {
			t.Error(err)
			return
		}
		state, err := portal.Status(ctx)
		if err != nil {
			t.Errorf("recovery status: %v", err)
			return
		}
		if !state.Online {
			if _, err := portal.Login(ctx, cred.Account, cred.Password, operator); err != nil {
				t.Errorf("independent recovery login: %v", err)
				return
			}
			state, err = portal.Status(ctx)
			if err != nil {
				t.Error(err)
				return
			}
		}
		account := cred.Account
		if operator != "campus" {
			account += "@" + operator
		}
		if state.Terminal == nil || terminalAccount(state.Terminal.Account) != account {
			t.Error("recovery did not confirm the original account")
			return
		}
		if _, err := link.Probe(ctx); err != nil {
			t.Errorf("recovery external connectivity: %v", err)
		}
	})
}

func TestLivePortalCycle(t *testing.T) {
	if os.Getenv("NJUPT_NET_CYCLE") != "1" {
		t.Skip("set NJUPT_NET_CYCLE=1 to offline and restore the owned terminal")
	}
	link, cfg := liveLink(t)
	portal, err := p.New(link, "pc")
	if err != nil {
		t.Fatal(err)
	}
	cred, operator := ownedTerminal(t, portal, cfg)
	restoreTerminal(t, link, cred, operator)
	if _, err := portal.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := portal.Login(context.Background(), cred.Account, cred.Password, operator); err != nil {
		t.Fatal(err)
	}
	if _, err := link.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Explicit composition: Self offline is not a fallback for p logout.
func TestLiveSelfOfflinePortalLogin(t *testing.T) {
	if os.Getenv("NJUPT_NET_CYCLE") != "1" {
		t.Skip("set NJUPT_NET_CYCLE=1 for the explicit Self-offline/p-login cycle")
	}
	link, cfg := liveLink(t)
	portal, err := p.New(link, "pc")
	if err != nil {
		t.Fatal(err)
	}
	cred, operator := ownedTerminal(t, portal, cfg)
	s := management(t, link, cred)
	connections, err := s.Online(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sessionID := ""
	for _, connection := range connections {
		if connection.IP == link.Source() {
			if sessionID != "" {
				t.Fatal("multiple online sessions match selected source")
			}
			sessionID = connection.SessionID
		}
	}
	if sessionID == "" {
		t.Fatal("management account has no connection matching selected source")
	}
	restoreTerminal(t, link, cred, operator)
	if _, err := s.Offline(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	portal, err = p.New(link, "pc")
	if err != nil {
		t.Fatal(err)
	}
	if err := waitOffline(context.Background(), portal); err != nil {
		t.Fatal(err)
	}
	if _, err := portal.Login(context.Background(), cred.Account, cred.Password, operator); err != nil {
		t.Fatal(err)
	}
	if _, err := link.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
}
