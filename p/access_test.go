package p

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type portalRoundTrip func(*http.Request) (*http.Response, error)

func TestPortalErrorInfoIsTyped(t *testing.T) {
	f := newPortalFixture(t, "pc")
	result, err := f.portal.ErrorInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != Accepted || result.PromptChinese != "test" {
		t.Fatalf("error prompt was lost: %+v", result)
	}
}

func TestPortalLoginEncodesCredentialsAndVerifies(t *testing.T) {
	for _, terminal := range []string{"pc", "mobile", "hipad", "vipad"} {
		t.Run(terminal, func(t *testing.T) {
			f := newPortalFixture(t, terminal)
			account, password := "student", "space &+=?#中文"
			raw, err := f.portal.Login(context.Background(), account, password, "cmcc")
			if err != nil {
				t.Fatal(err)
			}
			q := f.queries["/eportal/portal/login"][0]
			prefix := ",0,"
			macType := "0"
			if terminal == "mobile" {
				prefix, macType = ",1,", "1"
			}
			if q.Get("user_account") != prefix+"student@cmcc" || q.Get("user_password") != password {
				t.Fatal("credentials were changed by URL encoding")
			}
			if q.Get("wlan_user_ip") != "10.9.8.7" || q.Get("wlan_user_mac") != "000000000000" || q.Get("mac_type") != macType || q.Get("business_type") != "1" || q.Get("rcn") != "fresh-nonce" {
				t.Fatalf("incorrect login context: %v", q)
			}
			if len(q["lang"]) != 2 || q["lang"][0] != "zh-cn" || q["lang"][1] != "zh" {
				t.Fatalf("wrong language wrapper: %v", q["lang"])
			}
			if !raw.Verified || raw.Outcome != Accepted || raw.Status.Terminal.Account != "student@cmcc" || raw.Status.Terminal.Session != "42" {
				t.Fatal("login did not produce verified result")
			}
			if len(f.queries["/eportal/portal/login"]) != 1 || len(f.queries["/eportal/portal/online_list"]) != 2 {
				t.Fatal("login must submit once and verify status")
			}
		})
	}
}

func TestPortalAlreadyOnlineDoesNotAuthenticate(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.online = true
	f.uid = "someone-else"
	_, err := f.portal.Login(context.Background(), "student", "wrong", "campus")
	if err == nil || !strings.Contains(err.Error(), "already online") {
		t.Fatalf("expected explicit already-online error, got %v", err)
	}
	if len(f.queries["/eportal/portal/login"]) != 0 {
		t.Fatal("submitted authentication against an existing online session")
	}
}

func TestPortalLoginWaitsForAccountingWithoutResubmission(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.loginDelayQueries = 1
	f.portal.stateTimeout = 2 * time.Second
	raw, err := f.portal.Login(context.Background(), "student", "secret", "campus")
	if err != nil {
		t.Fatal(err)
	}
	if !raw.Verified || len(f.queries["/eportal/portal/login"]) != 1 || len(f.queries["/eportal/portal/online_list"]) != 3 {
		t.Fatal("asynchronous accounting must be observed without another login")
	}
}

func TestPortalStatusUsesMatchingSessionNotResultFlag(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.statusResponse = `{"result":1,"list":[{"user_account":"other","online_ip":"10.9.8.8","online_mac":"001122334455"}],"total":1}`
	raw, err := f.portal.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if raw.Online {
		t.Fatal("an online session on another source was mistaken for this terminal")
	}
	for _, response := range []string{
		`{"result":0,"msg":"Radius unavailable"}`,
		`{"result":0,"msg":"获取用户在线信息数据为空！","list":[],"total":0}`,
		`{"result":1,"uid":"legacy","ss4":"001122334455"}`,
		`{"result":1,"list":[],"total":1}`,
		`{"result":1,"list":[{"user_account":"a","online_ip":"10.9.8.7","online_mac":"001122334455"},{"user_account":"b","online_ip":"10.9.8.7","online_mac":"001122334455"}],"total":2}`,
	} {
		f.statusResponse = response
		if _, err = f.portal.Status(context.Background()); err == nil {
			t.Fatal("accepted a missing, inconsistent or ambiguous session list")
		}
	}
}

func TestPortalLoginRejectsUnprovenSuccess(t *testing.T) {
	for _, response := range []string{`{"result":0,"msg":"invalid password"}`, `{"result":2}`, `{"result":1}`} {
		t.Run(response, func(t *testing.T) {
			f := newPortalFixture(t, "pc")
			f.loginResponse = response
			f.loginNoStateChange = true
			if _, err := f.portal.Login(context.Background(), "student", "secret", "campus"); err == nil {
				t.Fatal("accepted rejected, unknown or unverified login")
			}
			if len(f.queries["/eportal/portal/login"]) != 1 {
				t.Fatal("authentication was repeated")
			}
		})
	}
}

func TestPortalLogoutVerifiesAndUsesDeployedFields(t *testing.T) {
	f := newPortalFixture(t, "pc")
	f.online = true
	f.uid = "student"
	raw, err := f.portal.Logout(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	q := f.queries["/eportal/portal/logout"][0]
	if q.Get("user_account") != "drcom" || q.Get("user_password") != "123" || q.Get("ac_logout") != "1" || q.Get("register_mode") != "1" || q.Get("wlan_user_ip") != "10.9.8.7" {
		t.Fatalf("incorrect logout contract: %v", q)
	}
	if raw.Status.Online {
		t.Fatal("logout was not verified offline")
	}
	if _, err := f.portal.Logout(context.Background()); err == nil {
		t.Fatal("offline terminal should not trigger another logout")
	}
	if len(f.queries["/eportal/portal/logout"]) != 1 {
		t.Fatal("logout was repeated")
	}
}

func TestPortalUnknownResponseNeverRetries(t *testing.T) {
	f := newPortalFixture(t, "pc")
	if _, err := f.portal.Configure(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Break callbacks only for the credential submission, while leaving the
	// status requests readable. The mutated server models an ambiguous result.
	underlying := f.portal.client.Transport
	f.portal.client.Transport = portalRoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/login") {
			return nil, fmt.Errorf("connection failed")
		}
		return underlying.RoundTrip(r)
	})
	result, err := f.portal.Login(context.Background(), "student", "do-not-leak?&=", "campus")
	if err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("ambiguous result not preserved: %v", err)
	}
	if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), "user_password") {
		t.Fatal("error contains secret query")
	}
	if result.Outcome != Unknown || result.Verified || result.Status == nil {
		t.Fatal("ambiguous submission was not distinguished from rejection")
	}
	if len(f.queries["/eportal/portal/online_list"]) != 2 {
		t.Fatal("ambiguous submission must observe status once")
	}
}
