package p

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestCaptchaCheckRequiresFreshImageAndPreservesNativeRejection(t *testing.T) {
	f := newPortalFixture(t, "pc")
	ctx := context.Background()
	if _, err := f.portal.CheckCaptcha(ctx, "wrong"); err == nil {
		t.Fatal("checked an answer without an issued image")
	}
	if _, err := f.portal.Captcha(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := f.portal.CheckCaptcha(ctx, "wrong")
	if err == nil || result == nil || result.Outcome != Rejected || result.Verified {
		t.Fatalf("captcha rejection was lost: result=%+v error=%v", result, err)
	}
	if _, err := f.portal.CheckCaptcha(ctx, "wrong"); err == nil {
		t.Fatal("reused a previously checked image")
	}
	path := "/eportal/portal/captcha/check"
	if len(f.queries[path]) != 1 || f.queries[path][0].Get("captcha") != "wrong" {
		t.Fatalf("unexpected captcha check: %v", f.queries[path])
	}
}

func TestPortalPasswordSubmissionAndAcceptance(t *testing.T) {
	f := newPortalFixture(t, "pc")
	account, password := "student", "old &secret"
	image, err := f.portal.Captcha(context.Background())
	if err != nil || string(image) != "test-image" {
		t.Fatalf("captcha: %v", err)
	}
	if len(f.requests) != 1 || f.requests[0] != "/eportal/portal/captcha" {
		t.Fatalf("captcha fetched unrelated configuration or browser resources: %v", f.requests)
	}
	result, err := f.portal.ChangePassword(context.Background(), account, password, "new &secret", "AB12")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != Accepted || result.Verified {
		t.Fatal("server acceptance must not claim a verified password change")
	}
	q := f.queries["/eportal/portal/change_pass"][0]
	for key, want := range map[string]string{"user_account": "student", "user_old_password": "old &secret", "user_new_password": "new &secret"} {
		decoded, err := base64.StdEncoding.DecodeString(q.Get(key))
		if err != nil || string(decoded) != want {
			t.Fatalf("wrong change password %s", key)
		}
	}
	if q.Get("registerMode") != "1" || q.Get("captcha") != "AB12" {
		t.Fatalf("wrong conditional change fields: %v", q)
	}
}
