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
			fmt.Fprint(w, selfTestOperatorPage("old-account", "old-password", "mobile-account", "mobile-password", "old-password / new-password / mobile-password被拒绝", "false"))
			return
		}
		fmt.Fprint(w, selfTestOperatorPage("old-account", "old-password", "mobile-account", "mobile-password", "", ""))
	})
	result, err := s.BindOperator(context.Background(), "njxy", "new-account", "new-password")
	if err == nil || result == nil || result.Outcome != Rejected || result.Verified || posts != 1 {
		t.Fatalf("rejection result=%+v posts=%d error=%v", result, posts, err)
	}
	data, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, password := range []string{"old-password", "new-password", "mobile-password"} {
		if strings.Contains(string(data), password) || strings.Contains(err.Error(), password) {
			t.Error("rejection output exposed a password")
		}
	}
}

func TestUnbindOperatorClearsOnlySelectedFields(t *testing.T) {
	for _, operator := range []string{"njxy", "cmcc"} {
		t.Run(operator, func(t *testing.T) {
			telecom, telecomPassword, mobile, mobilePassword := "telecom-account", "telecom-password", "mobile-account", "mobile-password"
			posts, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					reads++
					fmt.Fprint(w, selfTestOperatorPage(telecom, telecomPassword, mobile, mobilePassword, "", ""))
					return
				}
				posts++
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.PostForm.Get("csrftoken") != "operator-form-token" || len(r.PostForm) != 5 {
					t.Error("operator POST used an incorrect token or redundant fields")
				}
				if operator == "njxy" {
					if r.PostForm.Get("FLDEXTRA1") != "" || r.PostForm.Get("FLDEXTRA2") != "" || r.PostForm.Get("FLDEXTRA3") != mobile || r.PostForm.Get("FLDEXTRA4") != mobilePassword {
						t.Error("telecom unbinding did not preserve the mobile fields")
					}
					telecom, telecomPassword = "", ""
				} else {
					if r.PostForm.Get("FLDEXTRA3") != "" || r.PostForm.Get("FLDEXTRA4") != "" || r.PostForm.Get("FLDEXTRA1") != telecom || r.PostForm.Get("FLDEXTRA2") != telecomPassword {
						t.Error("mobile unbinding did not preserve the telecom fields")
					}
					mobile, mobilePassword = "", ""
				}
				fmt.Fprint(w, selfTestOperatorPage("echo-account", "echo-password", "echo-account", "echo-password", "telecom-password / mobile-password已解除", "true"))
			})
			result, err := s.UnbindOperator(context.Background(), operator)
			if err != nil || result == nil || result.Outcome != Accepted || !result.Verified || result.Account != "" || posts != 1 || reads != 2 {
				t.Fatalf("unbind result=%+v posts=%d reads=%d error=%v", result, posts, reads, err)
			}
			selected, other := result.Bindings.NJXY, result.Bindings.CMCC
			if operator == "cmcc" {
				selected, other = other, selected
			}
			if selected.Account != "" || selected.PasswordSet || other.Account == "" || !other.PasswordSet {
				t.Fatalf("incorrect final bindings: %+v", result.Bindings)
			}
			encoded, err := json.Marshal(result)
			if err != nil || strings.Contains(string(encoded), "telecom-password") || strings.Contains(string(encoded), "mobile-password") || strings.Contains(string(encoded), "echo-password") {
				t.Error("operator unbinding output exposed a password")
			}
		})
	}
}

func TestUnbindOperatorAlreadyEmptyDoesNotSubmit(t *testing.T) {
	for _, operator := range []string{"njxy", "cmcc"} {
		t.Run(operator, func(t *testing.T) {
			posts, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
				} else {
					reads++
				}
				fmt.Fprint(w, selfTestOperatorPage("", "", "", "", "", ""))
			})
			result, err := s.UnbindOperator(context.Background(), operator)
			if err != nil || result == nil || result.Outcome != NotSubmitted || !result.Verified || posts != 0 || reads != 1 {
				t.Fatalf("empty unbind result=%+v posts=%d reads=%d error=%v", result, posts, reads, err)
			}
		})
	}
}

func TestUnbindOperatorRequiresPersistedClearAndPreservedOtherFields(t *testing.T) {
	for _, change := range []string{"account_retained", "password_retained", "other_account_changed", "other_password_changed"} {
		t.Run(change, func(t *testing.T) {
			posts, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
					fmt.Fprint(w, selfTestOperatorPage("", "", "mobile-account", "mobile-password", "已受理", "true"))
					return
				}
				reads++
				if reads == 1 {
					fmt.Fprint(w, selfTestOperatorPage("old-account", "old-password", "mobile-account", "mobile-password", "", ""))
					return
				}
				account, password, otherAccount, otherPassword := "", "", "mobile-account", "mobile-password"
				switch change {
				case "account_retained":
					account = "old-account"
				case "password_retained":
					password = "old-password"
				case "other_account_changed":
					otherAccount = "different-account"
				case "other_password_changed":
					otherPassword = "different-password"
				}
				fmt.Fprint(w, selfTestOperatorPage(account, password, otherAccount, otherPassword, "", ""))
			})
			result, err := s.UnbindOperator(context.Background(), "njxy")
			if err == nil || result == nil || result.Outcome != Accepted || result.Verified || posts != 1 || reads != 2 {
				t.Fatalf("unverified clear result=%+v posts=%d reads=%d error=%v", result, posts, reads, err)
			}
		})
	}
}

func TestUnbindOperatorRejectedOrUnknownIsNotRetried(t *testing.T) {
	for _, state := range []string{"false", ""} {
		t.Run("state="+state, func(t *testing.T) {
			posts, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
					fmt.Fprint(w, selfTestOperatorPage("", "", "mobile-account", "mobile-password", "old-password / mobile-password", state))
					return
				}
				reads++
				fmt.Fprint(w, selfTestOperatorPage("old-account", "old-password", "mobile-account", "mobile-password", "", ""))
			})
			result, err := s.UnbindOperator(context.Background(), "njxy")
			want := Rejected
			if state == "" {
				want = Unknown
			}
			if err == nil || result == nil || result.Outcome != want || result.Verified || posts != 1 || reads != 1 {
				t.Fatalf("non-acceptance result=%+v posts=%d reads=%d error=%v", result, posts, reads, err)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "old-password") || strings.Contains(string(encoded), "mobile-password") || strings.Contains(err.Error(), "old-password") || strings.Contains(err.Error(), "mobile-password") {
				t.Error("non-acceptance output exposed a password")
			}
		})
	}
}

func TestOperatorArgumentsAreValidatedBeforeRequests(t *testing.T) {
	requests := 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) { requests++ })
	for _, value := range []struct{ operator, account, password string }{
		{"invalid", "account", "password"}, {"njxy", "", "password"}, {"cmcc", "account", ""}, {"njxy", "", ""},
	} {
		if _, err := s.BindOperator(context.Background(), value.operator, value.account, value.password); err == nil {
			t.Error("invalid binding arguments were accepted")
		}
	}
	if _, err := s.UnbindOperator(context.Background(), "invalid"); err == nil {
		t.Error("invalid unbinding operator was accepted")
	}
	if requests != 0 {
		t.Fatalf("made %d requests for invalid arguments", requests)
	}
}
