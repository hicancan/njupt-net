package zfw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestSelfDeviceEmptyBodyIsAProtocolFailure(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/service/myMac" {
			fmt.Fprint(w, selfTestPage("", "fixture-account"))
			return
		}
		if r.URL.Query().Get("sortName") != "2" || r.URL.Query().Get("sortOrder") != "DESC" {
			t.Error("device sort differs from the page")
		}
	})
	if _, err := s.Devices(context.Background(), 1, 10); err == nil || !strings.Contains(err.Error(), "empty JSON") {
		t.Fatalf("empty body was treated as a valid list: %v", err)
	}
}

func TestSelfUnbindUsesTheInlineUnbindToken(t *testing.T) {
	calls := 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/service/myMac" {
			fmt.Fprint(w, selfTestPage(`<script>function unbindmac(unbindmac) { window.location.href = '/Self/service/unbindmac?mac=' + unbindmac + "&ajaxCsrfToken=" + 'unbind-token'; }</script>`, "fixture-account"))
			return
		}
		if r.URL.Path == "/Self/service/getMacList" {
			return
		}
		calls++
		if r.URL.Query().Get("ajaxCsrfToken") != "unbind-token" || r.URL.Query().Get("mac") != "112233445566" || r.URL.Query().Has("t") {
			t.Error("unbind did not reproduce its actual navigation query")
		}
		fmt.Fprint(w, selfTestPage(`<script>(function(msg) { if(msg != "") { swal({text:msg}); } })('解除绑定成功');</script>`, "fixture-account"))
	})
	result, err := s.Unbind(context.Background(), "112233445566")
	if err == nil || result.Verified || result.Outcome != Unknown || result.Message != "解除绑定成功" || calls != 1 {
		t.Fatalf("result = %v, calls = %d, error = %v", result, calls, err)
	}
}

func TestSelfUnbindDoesNotTreatFailureMessageAsSuccess(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/service/myMac" {
			fmt.Fprint(w, selfTestPage(`<script>var target = "&ajaxCsrfToken=" + 'unbind-token';</script>`, "fixture-account"))
			return
		}
		if r.URL.Path == "/Self/service/getMacList" {
			fmt.Fprint(w, `{"total":1,"rows":[[1,"112233445566","terminal","time","10.0.0.1"]]}`)
			return
		}
		fmt.Fprint(w, selfTestPage(`<script>(function(msg) { if(msg != "") { swal({text:msg}); } })('解除绑定失败');</script>`, "fixture-account"))
	})
	result, err := s.Unbind(context.Background(), "112233445566")
	if err == nil || result.Verified || result.Outcome != Unknown || result.Message != "解除绑定失败" {
		t.Fatalf("failure message was accepted: %v, %v", result, err)
	}
}

func TestUnbindConfirmsAbsenceAcrossEveryMACPage(t *testing.T) {
	for _, targetRemains := range []bool{true, false} {
		t.Run(fmt.Sprint(targetRemains), func(t *testing.T) {
			writes, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/Self/service/myMac":
					fmt.Fprint(w, selfTestPage(`<script>var target = "&ajaxCsrfToken=" + 'unbind-token';</script>`, "fixture-account"))
				case "/Self/service/unbindmac":
					writes++
					fmt.Fprint(w, selfTestPage(`<script>var state = "true"; (function(msg) { if(msg != "") { swal({text:msg}); } })('处理完成');</script>`, "fixture-account"))
				case "/Self/service/getMacList":
					reads++
					page, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
					if r.URL.Query().Get("pageSize") != "100" {
						t.Error("MAC verification used an unexpected page size")
					}
					rows := [][]any{}
					for number := (page-1)*100 + 1; number <= min(page*100, 101); number++ {
						mac := fmt.Sprintf("%012x", number)
						if (writes == 0 || targetRemains) && number == 101 {
							mac = "112233445566"
						}
						rows = append(rows, []any{0, mac, nil, nil, nil})
					}
					json.NewEncoder(w).Encode(map[string]any{"total": 101, "rows": rows})
				default:
					t.Error("unexpected MAC operation endpoint")
				}
			})
			result, err := s.Unbind(context.Background(), "112233445566")
			if (err == nil) == targetRemains || result == nil || result.Verified == targetRemains || result.Outcome != Accepted || writes != 1 || reads != 4 {
				t.Fatalf("result=%+v targetRemains=%t writes=%d reads=%d error=%v", result, targetRemains, writes, reads, err)
			}
		})
	}
}

func TestUnbindDoesNotSubmitAnAlreadyAbsentMAC(t *testing.T) {
	writes, reads := 0, 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Self/service/myMac":
			fmt.Fprint(w, selfTestPage(`<script>var target = "&ajaxCsrfToken=" + 'unbind-token';</script>`, "fixture-account"))
		case "/Self/service/getMacList":
			reads++
			fmt.Fprint(w, `{"total":0,"rows":[]}`)
		case "/Self/service/unbindmac":
			writes++
		default:
			t.Error("unexpected MAC operation endpoint")
		}
	})
	result, err := s.Unbind(context.Background(), "112233445566")
	if err != nil || result == nil || result.Outcome != NotSubmitted || !result.Verified || writes != 0 || reads != 1 {
		t.Fatalf("already absent MAC was submitted: result=%+v writes=%d reads=%d error=%v", result, writes, reads, err)
	}
}

func TestUnbindSeparatesAcceptanceFromTheFinalBindingState(t *testing.T) {
	for _, test := range []struct {
		name          string
		beforeUnknown bool
		state         string
		outcome       Outcome
	}{
		{name: "unknown-before-message-is-not-an-acknowledgement", beforeUnknown: true, outcome: Unknown},
		{name: "known-before-message-is-not-an-acknowledgement", outcome: Unknown},
		{name: "explicit-rejection-survives-final-absence", state: "false", outcome: Rejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			writes, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/Self/service/myMac":
					fmt.Fprint(w, selfTestPage(`<script>var target = "&ajaxCsrfToken=" + 'unbind-token';</script>`, "fixture-account"))
				case "/Self/service/getMacList":
					reads++
					if writes == 0 {
						if !test.beforeUnknown {
							fmt.Fprint(w, `{"total":1,"rows":[[1,"112233445566",null,null,null]]}`)
						}
						return
					}
					fmt.Fprint(w, `{"total":0,"rows":[]}`)
				case "/Self/service/unbindmac":
					writes++
					fmt.Fprint(w, selfTestPage(`<script>var state = "`+test.state+`"; (function(msg) { if(msg != "") { swal({text:msg}); } })('解除绑定失败');</script>`, "fixture-account"))
				default:
					t.Error("unexpected MAC operation endpoint")
				}
			})
			result, err := s.Unbind(context.Background(), "112233445566")
			if err == nil || result == nil || result.Outcome != test.outcome || !result.Verified || result.Message != "解除绑定失败" || writes != 1 || reads != 2 {
				t.Fatalf("final absence replaced submission evidence: result=%+v writes=%d reads=%d error=%v", result, writes, reads, err)
			}
		})
	}
}

func TestUnbindDoesNotTreatAnIncompleteMACPageAsAbsence(t *testing.T) {
	writes := 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Self/service/myMac":
			fmt.Fprint(w, selfTestPage(`<script>var target = "&ajaxCsrfToken=" + 'unbind-token';</script>`, "fixture-account"))
		case "/Self/service/unbindmac":
			writes++
			fmt.Fprint(w, selfTestPage(`<script>(function(msg) { if(msg != "") { swal({text:msg}); } })('完成');</script>`, "fixture-account"))
		case "/Self/service/getMacList":
			fmt.Fprint(w, `{"total":1,"rows":[]}`)
		}
	})
	result, err := s.Unbind(context.Background(), "112233445566")
	if err == nil || result == nil || result.Verified || result.Outcome != Unknown || writes != 1 {
		t.Fatalf("incomplete list was mistaken for removal: result=%+v writes=%d error=%v", result, writes, err)
	}
}
