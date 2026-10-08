package zfw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

const connectionTestJSON = `{"sessionId":"session-123","loginTime":"time","ip":"10.0.0.1","mac":"112233445566","useTime":"60","upFlow":"10","downFlow":"20","hostName":null,"terminalType":"#Windows","brasid":"bras","userId":1}`

func TestSelfOfflineRequiresBooleanSuccessAndNeverRepeats(t *testing.T) {
	for _, response := range []string{`{"success":true}`, `{"success":false}`, `{}`} {
		t.Run(response, func(t *testing.T) {
			calls := 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Self/dashboard/getOnlineList" {
					if calls == 0 {
						fmt.Fprint(w, "["+connectionTestJSON+"]")
					} else {
						fmt.Fprint(w, "[]")
					}
					return
				}
				calls++
				if r.URL.Query().Get("sessionid") != "session-123" {
					t.Error("offline lost its target session")
				}
				fmt.Fprint(w, response)
			})
			s.identity = "fixture-account"
			result, err := s.Offline(context.Background(), "session-123")
			if (err == nil) != (response == `{"success":true}`) || calls != 1 {
				t.Fatalf("calls = %d, error = %v", calls, err)
			}
			if result == nil || (response == `{}` && result.Outcome != Unknown) || (response == `{"success":false}` && result.Outcome != Rejected) {
				t.Fatalf("submission outcome lost: result=%+v error=%v", result, err)
			}
		})
	}
}

func TestOfflineDoesNotSubmitAnAbsentSession(t *testing.T) {
	for _, response := range []struct{ name, body string }{
		{"empty-list", `[]`},
		{"different-id", "[" + strings.Replace(connectionTestJSON, "session-123", "session-other", 1) + "]"},
		{"matching-prefix", "[" + strings.Replace(connectionTestJSON, "session-123", "session-1234", 1) + "]"},
	} {
		t.Run(response.name, func(t *testing.T) {
			writes, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Self/dashboard/getOnlineList" {
					reads++
					fmt.Fprint(w, response.body)
					return
				}
				writes++
				fmt.Fprint(w, `{"success":true}`)
			})
			result, err := s.Offline(context.Background(), "session-123")
			if err == nil || result == nil || result.SessionID != "session-123" || result.Outcome != NotSubmitted || result.Accepted || result.Verified || writes != 0 || reads != 1 {
				t.Fatalf("offline=%+v writes=%d reads=%d error=%v", result, writes, reads, err)
			}
		})
	}
}

func TestOfflineDoesNotSubmitAfterFailedPreconditionQuery(t *testing.T) {
	for _, response := range []struct {
		name, body string
		status     int
	}{
		{"http", "unavailable", http.StatusServiceUnavailable},
		{"invalid-json", "not json", http.StatusOK},
		{"expired-session", selfTestLoginPage(), http.StatusOK},
	} {
		t.Run(response.name, func(t *testing.T) {
			writes, reads := 0, 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Self/dashboard/getOnlineList" {
					reads++
					w.WriteHeader(response.status)
					fmt.Fprint(w, response.body)
					return
				}
				writes++
				fmt.Fprint(w, `{"success":true}`)
			})
			result, err := s.Offline(context.Background(), "session-123")
			if err == nil || result == nil || result.Outcome != NotSubmitted || result.Accepted || result.Verified || writes != 0 || reads != 1 {
				t.Fatalf("offline=%+v writes=%d reads=%d error=%v", result, writes, reads, err)
			}
		})
	}
}

func TestHistoryDecodesAllObservedColumns(t *testing.T) {
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[[1,2,"10.0.0.1","112233445566",60,1024,3,0,null,"#Windows","server text",17]]`)
	})
	records, err := s.History(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("history=%+v error=%v", records, err)
	}
	record := records[0]
	if record.LoginTime != "1" || record.Duration != "60" || record.IP != "10.0.0.1" || record.HostName != nil || record.ServerField10 != "server text" || record.ServerField11 != "17" {
		t.Fatalf("history columns changed: %+v", record)
	}
	encoded, err := json.Marshal(record)
	if err != nil || !strings.Contains(string(encoded), `"server_field_11":17`) {
		t.Fatalf("history output=%s error=%v", encoded, err)
	}
	var decoded LoginRecord
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.ServerField11 != "17" || decoded.IP != record.IP || decoded.Duration != record.Duration || decoded.HostName != nil {
		t.Fatalf("public history record cannot read its own JSON: decoded=%+v error=%v", decoded, err)
	}
}

func TestHistoryRejectsShiftedOrMismatchedColumns(t *testing.T) {
	for _, body := range []string{
		`[[1,2,"ip","mac",60,1024,3,0,null,"terminal"]]`,
		`[[1,2,"ip","mac","60",1024,3,0,null,"terminal","server",17]]`,
		`[[1,2,"ip","mac",null,1024,3,0,null,"terminal","server",17]]`,
	} {
		s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		if _, err := s.History(context.Background()); err == nil {
			t.Fatalf("accepted malformed history %s", body)
		}
	}
}

func TestOfflineWaitsWithoutResubmitting(t *testing.T) {
	writes, reads := 0, 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/dashboard/tooffline" {
			writes++
			fmt.Fprint(w, `{"success":true}`)
			return
		}
		reads++
		if reads <= 2 {
			fmt.Fprint(w, "["+connectionTestJSON+"]")
			return
		}
		fmt.Fprint(w, `[]`)
	})
	result, err := s.Offline(context.Background(), "session-123")
	if err != nil || !result.Accepted || !result.Verified || writes != 1 || reads != 3 {
		t.Fatalf("offline=%+v writes=%d reads=%d error=%v", result, writes, reads, err)
	}
}

func TestOfflineKeepsAcceptedSeparateFromVerified(t *testing.T) {
	writes := 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/dashboard/tooffline" {
			writes++
			fmt.Fprint(w, `{"success":true}`)
			return
		}
		fmt.Fprint(w, "["+connectionTestJSON+"]")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result, err := s.Offline(ctx, "session-123")
	if err == nil || result == nil || result.Outcome != Accepted || !result.Accepted || result.Verified || writes != 1 {
		t.Fatalf("offline=%+v writes=%d error=%v", result, writes, err)
	}
}

func TestOfflineKeepsAcceptedAfterFailedObservation(t *testing.T) {
	writes, reads := 0, 0
	s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Self/dashboard/tooffline" {
			writes++
			fmt.Fprint(w, `{"success":true}`)
			return
		}
		reads++
		if writes == 0 {
			fmt.Fprint(w, "["+connectionTestJSON+"]")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	result, err := s.Offline(context.Background(), "session-123")
	if err == nil || result == nil || result.Outcome != Accepted || !result.Accepted || result.Verified || writes != 1 || reads != 2 {
		t.Fatalf("offline=%+v writes=%d reads=%d error=%v", result, writes, reads, err)
	}
}
