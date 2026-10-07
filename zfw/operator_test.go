package zfw

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"testing"
)

func selfTestOperatorPage(telecom, telecomPassword, mobile, mobilePassword, message, state string) string {
	form := `<form method="post" action="/Self/service/bind-operator"><input type="hidden" name="csrftoken" value="operator-form-token">`
	for i, value := range []string{telecom, telecomPassword, mobile, mobilePassword} {
		form += fmt.Sprintf(`<input name="FLDEXTRA%d" value="%s">`, i+1, html.EscapeString(value))
	}
	form += `</form><script>(function (msg) { if (msg != "") { var state = "` + state + `"; swal({text:msg}); } })('` + message + `');</script>`
	return selfTestPage(form, "fixture-account")
}

func TestSelfBindOperatorPreservesTheOtherOperator(t *testing.T) {
	for _, operator := range []string{"njxy", "cmcc"} {
		t.Run(operator, func(t *testing.T) {
			telecom, telecomPassword, mobile, mobilePassword := "telecom-account", "telecom-password", "mobile-account", "mobile-password"
			posts := 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					fmt.Fprint(w, selfTestOperatorPage(telecom, telecomPassword, mobile, mobilePassword, "", ""))
					return
				}
				posts++
				r.ParseForm()
				if r.PostForm.Get("csrftoken") != "operator-form-token" || len(r.PostForm) != 5 {
					t.Error("operator POST used an incorrect token or redundant fields")
				}
				if operator == "njxy" {
					if r.PostForm.Get("FLDEXTRA3") != mobile || r.PostForm.Get("FLDEXTRA4") != mobilePassword {
						t.Error("mobile binding was modified")
					}
					telecom, telecomPassword = r.PostForm.Get("FLDEXTRA1"), r.PostForm.Get("FLDEXTRA2")
				} else {
					if r.PostForm.Get("FLDEXTRA1") != telecom || r.PostForm.Get("FLDEXTRA2") != telecomPassword {
						t.Error("telecom binding was modified")
					}
					mobile, mobilePassword = r.PostForm.Get("FLDEXTRA3"), r.PostForm.Get("FLDEXTRA4")
				}
				fmt.Fprint(w, selfTestOperatorPage(telecom, telecomPassword, mobile, mobilePassword, "绑定成功", "true"))
			})
			result, err := s.BindOperator(context.Background(), operator, "new-account", "new-password")
			if err != nil {
				t.Fatal(err)
			}
			if posts != 1 || !result.Verified || result.Outcome != Accepted {
				t.Fatalf("posts = %d; result = %v", posts, result)
			}
			encoded, _ := json.Marshal(result)
			for _, password := range []string{"new-password", "telecom-password", "mobile-password", "server-secret"} {
				if strings.Contains(string(encoded), password) {
					t.Error("operator response exposed a password")
				}
			}
		})
	}
}

func TestOperatorAcceptanceRequiresIndependentReadback(t *testing.T) {
	posts, reads := 0, 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			fmt.Fprint(w, selfTestOperatorPage("new-account", "new-password", "mobile-account", "mobile-password", "已受理", "true"))
			return
		}
		reads++
		fmt.Fprint(w, selfTestOperatorPage("old-account", "old-password", "mobile-account", "mobile-password", "", ""))
	})
	result, err := s.BindOperator(context.Background(), "njxy", "new-account", "new-password")
	if err == nil || result == nil || result.Outcome != Accepted || result.Verified || posts != 1 || reads != 2 || result.Bindings.NJXY.Account != "old-account" {
		t.Fatalf("accepted POST echo was treated as persisted binding: result=%+v posts=%d reads=%d error=%v", result, posts, reads, err)
	}
}

func TestOperatorRejectedResultDoesNotLeakPassword(t *testing.T) {
	posts := 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			fmt.Fprint(w, selfTestOperatorPage("old-account", "old-password", "mobile-account", "mobile-password", "new-password被拒绝", "false"))
			return
		}
		fmt.Fprint(w, selfTestOperatorPage("old-account", "old-password", "mobile-account", "mobile-password", "", ""))
	})
	result, err := s.BindOperator(context.Background(), "njxy", "new-account", "new-password")
	if err == nil || result == nil || result.Outcome != Rejected || result.Verified || posts != 1 {
		t.Fatalf("rejection result=%+v posts=%d error=%v", result, posts, err)
	}
	data, marshalErr := json.Marshal(result)
	if marshalErr != nil || strings.Contains(string(data), "new-password") {
		t.Fatalf("rejection output exposed password: %s, %v", data, marshalErr)
	}
}
