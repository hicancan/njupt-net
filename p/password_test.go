package p

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestPortalPasswordSubmissionAndAcceptance(t *testing.T) {
	f := newPortalFixture(t, "pc")
	account, password := "student", "old &secret"
	image, err := f.portal.Captcha(context.Background())
	if err != nil || string(image) != "test-image" {
		t.Fatalf("captcha: %v", err)
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
