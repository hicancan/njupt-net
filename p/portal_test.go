package p

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hicancan/njupt-net/v4/network"
)

func TestPortalSourceAndDefaultEndpointComeFromLink(t *testing.T) {
	link, err := network.NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(link.Close)
	portal, err := New(link, "pc")
	if err != nil {
		t.Fatal(err)
	}
	if portal.source != link.Source() || portal.api != "https://p.njupt.edu.cn:804/eportal/portal/" {
		t.Fatalf("unexpected default source or endpoint: %+v", portal)
	}
	if _, err := New(nil, "pc"); err == nil {
		t.Fatal("accepted a missing network link")
	}
	if _, err := New(link, "unrecognized"); err == nil {
		t.Fatal("accepted an unknown terminal type")
	}
}

func TestPortalSelectsOnlyDeployedPorts(t *testing.T) {
	link, err := network.NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(link.Close)
	for _, port := range []int{801, 802, 803, 804} {
		portal, err := NewAt(link, "pc", port)
		if err != nil {
			t.Fatal(err)
		}
		scheme := "http"
		if port == 802 || port == 804 {
			scheme = "https"
		}
		want := fmt.Sprintf("%s://p.njupt.edu.cn:%d/eportal/portal/", scheme, port)
		if portal.api != want || portal.source != link.Source() {
			t.Fatalf("port %d: endpoint=%s source=%s", port, portal.api, portal.source)
		}
	}
	for _, port := range []int{-1, 0, 443, 800, 805, 65536} {
		if _, err := NewAt(link, "pc", port); err == nil {
			t.Fatalf("accepted undeployed port %d", port)
		}
	}
}

func TestPortalConfigurationUsesOnlyNativeAPI(t *testing.T) {
	f := newPortalFixture(t, "pc")
	config, err := f.portal.Configure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.Source != "10.9.8.7" || config.Endpoint != f.portal.api || config.Terminal != 1 ||
		config.Program != "fresh-program" || config.Page != "fresh-page" || !config.AccountPrefix ||
		config.EncodeCredentials || config.LoginMethod != 1 || config.CheckOnlineMethod != 1 ||
		config.RegisterMode != "1" || config.ACLogout != "1" {
		t.Fatalf("unexpected typed native configuration: %+v", config)
	}
	if len(f.requests) != 1 || f.requests[0] != "/eportal/portal/page/loadConfig" {
		t.Fatalf("configuration fetched resources outside its native endpoint: %v", f.requests)
	}
	query := f.queries["/eportal/portal/page/loadConfig"][0]
	if len(query) != 1 || query.Get("callback") != "dr1" {
		t.Fatalf("configuration sent unnecessary browser parameters: %v", query)
	}
	f.online, f.uid = true, "student"
	state, err := f.portal.Status(context.Background())
	if err != nil || !state.Online || state.Terminal.MAC != "AABBCCDDEEFF" {
		t.Fatalf("status after configuration: state=%+v error=%v", state, err)
	}
	query = f.queries["/eportal/portal/online_list"][0]
	if len(query) != 1 || query.Get("callback") != "dr2" || len(f.requests) != 2 {
		t.Fatalf("status reused browser configuration instead of its native contract: %v requests=%v", query, f.requests)
	}
}

func TestPortalStatusDoesNotRequireConfiguration(t *testing.T) {
	for _, online := range []bool{false, true} {
		t.Run(fmt.Sprintf("online=%t", online), func(t *testing.T) {
			f := newPortalFixture(t, "pc")
			f.config = "unavailable configuration"
			f.online, f.uid = online, "student"
			state, err := f.portal.Status(context.Background())
			if err != nil || state.Source != "10.9.8.7" || state.Online != online {
				t.Fatalf("native status depends on configuration: state=%+v error=%v", state, err)
			}
			if len(f.requests) != 1 || f.requests[0] != "/eportal/portal/online_list" {
				t.Fatalf("status fetched extra resources: %v", f.requests)
			}
			if query := f.queries["/eportal/portal/online_list"][0]; len(query) != 1 || query.Get("callback") != "dr1" {
				t.Fatalf("status sent unnecessary browser parameters: %v", query)
			}
		})
	}
}

func TestPortalStrictJSONP(t *testing.T) {
	for _, body := range []string{`wrong({"result":1})`, `dr7({"result":1});alert(1);`, `dr7([1])`, `dr7(null)`, `dr7({"result":1}) garbage`, `dr7({bad})`, "dr7({\"msg\":\"\xff\"})", `dr7({"result":1}{"result":2})`} {
		if _, err := decodeJSONP([]byte(body), "dr7"); err == nil {
			t.Fatalf("accepted unsafe JSONP %q", body)
		}
	}
	for _, body := range []string{`dr7({"result":1})`, ` dr7({"result":1}); `} {
		if obj, err := decodeJSONP([]byte(body), "dr7"); err != nil || scalar(obj["result"]) != "1" {
			t.Fatalf("valid JSONP rejected: %v", err)
		}
	}
}

func TestPortalConfigurationDoesNotSelectNativeLoginProtocol(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.config = strings.Replace(portalTestConfig, `"login_method":1`, `"login_method":0`, 1)
	if _, err := f.portal.Configure(context.Background()); err == nil {
		t.Fatal("configuration accepted an unsupported browser login method")
	}
	if _, err := f.portal.Login(context.Background(), "student", "secret", "campus"); err != nil {
		t.Fatalf("browser configuration prevented native password authentication: %v", err)
	}
	if len(f.queries["/eportal/portal/page/loadConfig"]) != 1 || len(f.queries["/eportal/portal/login"]) != 1 {
		t.Fatal("native login fetched or depended on the rejected browser configuration")
	}
}

func TestPortalRejectsIncompleteNativeConfiguration(t *testing.T) {
	for _, config := range []string{
		`{"code":0}`,
		`{"code":1,"data":null}`,
		strings.Replace(portalTestConfig, `"rcn":"fresh-nonce"`, `"rcn":""`, 1),
		strings.Replace(portalTestConfig, `"account_prefix":"1"`, `"account_prefix":"2"`, 1),
		strings.Replace(portalTestConfig, `"program_index":"fresh-program",`, "", 1),
	} {
		t.Run(config, func(t *testing.T) {
			f := newPortalFixture(t, "pc")
			f.config = config
			if _, err := f.portal.Configure(context.Background()); err == nil {
				t.Fatal("accepted an incomplete or unsupported native configuration")
			}
			if f.portal.configured || f.portal.settings != nil {
				t.Fatal("failed configuration was cached for authentication")
			}
		})
	}
}
