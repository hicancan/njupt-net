package zfw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestSelfMauthReadsJSONStringAndVerifiesChange(t *testing.T) {
	for _, changes := range []bool{true, false} {
		t.Run(fmt.Sprint(changes), func(t *testing.T) {
			state, operations := "默认", 0
			s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Self/dashboard/refreshMauthType" {
					json.NewEncoder(w).Encode(`<a href="dashboard/oprateMauthAction">` + state + `</a>`)
					return
				}
				if r.URL.Path != "/Self/dashboard/oprateMauthAction" || r.URL.RawQuery != "" || r.Method != http.MethodGet {
					t.Error("mauth operation invented parameters or changed the method")
				}
				operations++
				if changes {
					state = "启用"
				}
				fmt.Fprint(w, selfTestPage("", "fixture-account"))
			})
			s.identity = "fixture-account"
			result, err := s.ChangeMauth(context.Background())
			if changes && (err != nil || result.State != "启用" || result.PreviousState != "默认" || result.Outcome != Accepted || !result.Verified) {
				t.Fatalf("mauth = %v, %v", result, err)
			}
			if !changes && (err == nil || result == nil || result.Outcome != Unknown || result.Verified || result.PreviousState != "默认" || !strings.Contains(err.Error(), "did not change")) {
				t.Fatalf("unchanged state reported success: %v", err)
			}
			if operations != 1 {
				t.Fatalf("operation count = %d", operations)
			}
		})
	}
}
