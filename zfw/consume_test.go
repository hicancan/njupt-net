package zfw

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestConsumeUnlimitedUsesTheNumericAmount(t *testing.T) {
	for _, limit := range []string{"999999", "999999.0", "999999.00", "999999.000", "999998.999"} {
		t.Run(limit, func(t *testing.T) {
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				page := selfTestPage(`<form action="/Self/service/changeConsumeProtect"><input name="consumeLimit"></form>`, "fixture-account")
				fmt.Fprint(w, strings.Replace(page, `"installmentFlag":999999`, `"installmentFlag":`+limit, 1))
			})
			result, err := s.ConsumeProtect(context.Background())
			wantUnlimited := limit != "999998.999"
			if err != nil || result.Unlimited != wantUnlimited || result.Limit.String() != limit {
				t.Fatalf("consume=%+v wantUnlimited=%t error=%v", result, wantUnlimited, err)
			}
		})
	}
}

func TestSelfConsumeProtectVerifiesServerValue(t *testing.T) {
	for _, applied := range []bool{true, false} {
		t.Run(fmt.Sprint(applied), func(t *testing.T) {
			posts := 0
			limit := "999999"
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
					r.ParseForm()
					if r.PostForm.Get("csrftoken") != "consume-form-token" {
						t.Error("consume used the page token instead of the form token")
					}
					if applied {
						limit = r.PostForm.Get("consumeLimit")
					}
				}
				page := selfTestPage(`<form action="/Self/service/changeConsumeProtect"><input type="hidden" name="csrftoken" value="consume-form-token"><input name="consumeLimit"></form>`, "fixture-account")
				fmt.Fprint(w, strings.Replace(page, `"installmentFlag":999999`, `"installmentFlag":`+limit, 1))
			})
			result, err := s.SetConsumeProtect(context.Background(), "12.500")
			if applied && (err != nil || result.Limit.String() != "12.500" || result.Outcome != Accepted || !result.Verified) {
				t.Fatalf("result = %v; error = %v", result, err)
			}
			if !applied && (err == nil || result == nil || result.Outcome != Unknown || result.Verified || !strings.Contains(err.Error(), "did not apply")) {
				t.Fatalf("unapplied write reported success: %v", err)
			}
			if posts != 1 {
				t.Fatalf("write was repeated %d times", posts)
			}
		})
	}
}

func TestConsumePostEchoIsNotFinalStateVerification(t *testing.T) {
	posts, reads := 0, 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		limit := "999999"
		if r.Method == http.MethodPost {
			posts++
			limit = "12.500"
		} else {
			reads++
		}
		page := selfTestPage(`<form action="/Self/service/changeConsumeProtect"><input type="hidden" name="csrftoken" value="consume-form-token"><input name="consumeLimit"></form>`, "fixture-account")
		fmt.Fprint(w, strings.Replace(page, `"installmentFlag":999999`, `"installmentFlag":`+limit, 1))
	})
	result, err := s.SetConsumeProtect(context.Background(), "12.500")
	if err == nil || result == nil || result.Verified || result.Outcome != Unknown || result.Limit != "999999" || posts != 1 || reads != 2 {
		t.Fatalf("POST echo was treated as persisted state: result=%+v posts=%d reads=%d error=%v", result, posts, reads, err)
	}
}

func TestConsumeSubmissionFollowsItsReadOnlyResultOnce(t *testing.T) {
	posts, reads := 0, 0
	limit := "999999"
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Self/service/changeConsumeProtect":
			posts++
			r.ParseForm()
			limit = r.PostForm.Get("consumeLimit")
			http.Redirect(w, r, "/Self/service/consumeProtect", http.StatusSeeOther)
		case "/Self/service/consumeProtect":
			reads++
			if r.Method != http.MethodGet {
				t.Fatal("consume result page received another write")
			}
			page := selfTestPage(`<form action="/Self/service/changeConsumeProtect"><input name="csrftoken" value="consume-token"></form>`, "fixture-account")
			fmt.Fprint(w, strings.Replace(page, `"installmentFlag":999999`, `"installmentFlag":`+limit, 1))
		default:
			t.Fatal("consume submission reached an unrelated endpoint")
		}
	})
	result, err := s.SetConsumeProtect(context.Background(), "12.5")
	if err != nil || posts != 1 || reads != 2 || result.Outcome != Accepted || !result.Verified {
		t.Fatalf("redirected consume result=%+v posts=%d reads=%d error=%v", result, posts, reads, err)
	}
}

func TestConsumePostPageDoesNotSupplyVerifiedState(t *testing.T) {
	for _, failure := range []string{"", "readback unavailable", "session expired"} {
		t.Run(failure, func(t *testing.T) {
			posts, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
					if failure == "session expired" {
						fmt.Fprint(w, selfTestLoginPage())
					} else {
						// The submit response acknowledges navigation, not persisted state.
						fmt.Fprint(w, `<html><body>已处理提交</body></html>`)
					}
					return
				}
				reads++
				if reads == 2 && failure == "readback unavailable" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				limit := "999999"
				if reads == 2 {
					limit = "12.500"
				}
				page := selfTestPage(`<form action="/Self/service/changeConsumeProtect"><input type="hidden" name="csrftoken" value="consume-form-token"><input name="consumeLimit"></form>`, "fixture-account")
				fmt.Fprint(w, strings.Replace(page, `"installmentFlag":999999`, `"installmentFlag":`+limit, 1))
			})
			result, err := s.SetConsumeProtect(context.Background(), "12.500")
			if posts != 1 {
				t.Fatalf("submitted %d writes", posts)
			}
			if failure == "" {
				if err != nil || result.Outcome != Accepted || !result.Verified || result.Limit != "12.500" || reads != 2 {
					t.Fatalf("fresh readback failed to establish persisted state: result=%+v reads=%d error=%v", result, reads, err)
				}
			} else {
				if err == nil || result.Outcome != Unknown || result.Verified || result.ConsumeLimit != nil {
					t.Fatalf("unverified POST supplied final state: result=%+v error=%v", result, err)
				}
				if failure == "session expired" && reads != 1 {
					t.Fatal("expired management session was read again")
				}
			}
		})
	}
}
