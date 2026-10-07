package p

import (
	"context"
	"testing"
)

func TestPortalSelfBridgePreservesTargetAndDoesNotFollow(t *testing.T) {
	f := newPortalFixture(t, "pc")
	result, err := f.portal.SelfURL(context.Background(), "student", "old &secret", 1)
	if err != nil {
		t.Fatal(err)
	}
	q := f.queries["/eportal/portal/self"][0]
	if q.Get("wlan_user_ip") != "168364039" || q.Get("user_account") != "student" || q.Get("self_type") != "1" {
		t.Fatalf("wrong Self bridge contract: %v", q)
	}
	if result.Outcome != Accepted || result.Type != 1 || result.URL != "http://10.10.244.240:8080/Self/login/eportalLogin?params=fixture-params&timestamp=fixture-timestamp&sign=fixture-sign" {
		t.Fatalf("bridge target was lost: %+v", result)
	}
	if len(f.queries["/Self/"]) != 0 {
		t.Fatal("portal followed the management bridge")
	}
}
