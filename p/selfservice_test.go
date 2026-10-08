package p

import (
	"context"
	"net/url"
	"reflect"
	"strconv"
	"testing"
)

func TestPortalSelfBridgePreservesTargetAndDoesNotFollow(t *testing.T) {
	for selfType := 0; selfType <= 2; selfType++ {
		t.Run(strconv.Itoa(selfType), func(t *testing.T) {
			f := newPortalFixture(t, "pc")
			f.config = "configuration must not be fetched"
			f.statusResponse = "status must not be fetched"
			result, err := f.portal.SelfURL(context.Background(), "student", "old &secret", selfType)
			if err != nil {
				t.Fatal(err)
			}
			q := f.queries["/eportal/portal/self"][0]
			want := url.Values{"callback": {"dr1"}, "self_type": {strconv.Itoa(selfType)}, "user_account": {"student"}, "user_password": {"old &secret"}}
			if !reflect.DeepEqual(q, want) {
				t.Fatalf("wrong Self bridge contract: %v", q)
			}
			if result.Outcome != Accepted || result.Type != selfType || result.URL != "http://10.10.244.240:8080/Self/login/eportalLogin?params=fixture-params&timestamp=fixture-timestamp&sign=fixture-sign" {
				t.Fatalf("bridge target was lost: %+v", result)
			}
			if len(f.queries["/Self/"]) != 0 {
				t.Fatal("portal followed the management bridge")
			}
			if len(f.requests) != 1 || f.requests[0] != "/eportal/portal/self" {
				t.Fatalf("Self bridge fetched resources outside its native request: %v", f.requests)
			}
		})
	}
}
