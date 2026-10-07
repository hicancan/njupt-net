package p

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/hicancan/njupt-net/v3/network"
)

func TestPortalSourceComesFromLink(t *testing.T) {
	link, err := network.NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(link.Close)
	portal, err := New(link, "pc")
	if err != nil {
		t.Fatal(err)
	}
	if portal.source != link.Source() {
		t.Fatal("portal terminal source differs from its HTTP link")
	}
	if _, err := New(nil, "pc"); err == nil {
		t.Fatal("accepted a missing network link")
	}
	if _, err := New(link, "unrecognized"); err == nil {
		t.Fatal("accepted an unknown terminal type")
	}
}

func TestPortalConfigurationAndContext(t *testing.T) {
	f := newPortalFixture(t, "pc")
	config, err := f.portal.Configure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.Source != "10.9.8.7" || config.Program != "fresh-program" || config.Version != "fresh-version" || !config.AccountPrefix || config.EncodeCredentials || config.LoginMethod != 1 {
		t.Fatalf("unexpected typed runtime configuration: %+v", config)
	}
	if f.portal.program != "fresh-program" || f.portal.page != "fresh-page" || f.portal.version != "fresh-version" {
		t.Fatalf("dynamic context not loaded: %s %s %s", f.portal.program, f.portal.page, f.portal.version)
	}
	load := f.queries["/eportal/portal/page/loadConfig"][0]
	if load.Get("wlan_user_ip") != base64.StdEncoding.EncodeToString([]byte("10.9.8.7")) || load.Get("wlan_vlan_id") != "77" || load.Get("jsVersion") != "4.X" {
		t.Fatalf("unexpected bootstrap contract: %v", load)
	}
	f.online, f.uid = true, "student"
	if _, err := f.portal.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := f.queries["/eportal/portal/online_list"][0]
	if status.Get("program_index") != "fresh-program" || status.Get("page_index") != "fresh-page" || status.Get("jsVersion") != "fresh-version" {
		t.Fatalf("stale request context: %v", status)
	}
	if f.portal.mac != "AABBCCDDEEFF" {
		t.Fatal("status did not replace placeholder MAC")
	}
}

func TestPortalRejectsWrongTerminalSource(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.portal.source = "10.9.8.8"
	if _, err := f.portal.Configure(context.Background()); err == nil {
		t.Fatal("accepted portal context for another source address")
	}
	if len(f.queries["/eportal/portal/page/loadConfig"]) != 0 {
		t.Fatal("used an inconsistent source context")
	}
}

func TestPortalStrictJSONP(t *testing.T) {
	for _, body := range []string{`wrong({"result":1})`, `dr7({"result":1});alert(1);`, `dr7([1])`, `dr7(null)`, `dr7({"result":1}) garbage`, `dr7({bad})`} {
		if _, err := decodeJSONP([]byte(body), "dr7"); err == nil {
			t.Fatalf("accepted unsafe JSONP %q", body)
		}
	}
	for _, body := range []string{`dr7({"result":1})`, ` dr7({"result":1}); `} {
		if _, err := decodeJSONP([]byte(body), "dr7"); err != nil {
			t.Fatalf("valid JSONP rejected: %v", err)
		}
	}
}

func TestPortalRejectsChangedProtocol(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.config = strings.Replace(portalTestConfig, `"login_method":1`, `"login_method":0`, 1)
	if _, err := f.portal.Configure(context.Background()); err == nil {
		t.Fatal("silently accepted a different authentication protocol")
	}
	if len(f.queries["/eportal/portal/login"]) != 0 {
		t.Fatal("tried a fallback login")
	}
}

func TestPortalLiteralAssignmentsDoNotExecuteExpressions(t *testing.T) {
	if _, err := assignment(`var jsVersion=fetch('secret');`, "jsVersion"); err == nil {
		t.Fatal("accepted non-literal JavaScript")
	}
	if _, err := assignment(`var jsVersion='4.'+'5';`, "jsVersion"); err == nil {
		t.Fatal("accepted a JavaScript expression")
	}
	value, err := assignment(`var fileVersion="xyz";`, "fileVersion")
	if err != nil || value != "xyz" {
		t.Fatalf("literal parsing: %v", err)
	}
}
