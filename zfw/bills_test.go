package zfw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The CLI consumes the public encoding, which differs deliberately from the
// server's camel-case and uppercase wire fields. Decoding alone cannot check it.
func TestPublicJSONUsesBusinessFieldNames(t *testing.T) {
	checkObject := func(t *testing.T, data []byte, expected map[string]string) map[string]json.RawMessage {
		t.Helper()
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		if _, exists := object["type"]; exists {
			t.Fatalf("unrelated type field appeared in public JSON: %s", data)
		}
		for key, want := range expected {
			if value, exists := object[key]; !exists || string(value) != want {
				t.Fatalf("public field %s = %s, want %s in %s", key, value, want, data)
			}
		}
		return object
	}
	t.Run("connection", func(t *testing.T) {
		s := selfTestClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "["+connectionTestJSON+"]") })
		connections, err := s.Online(context.Background())
		if err != nil || len(connections) != 1 {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(connections[0])
		if err != nil {
			t.Fatal(err)
		}
		object := checkObject(t, encoded, map[string]string{"session_id": `"session-123"`, "login_time": `"time"`, "user_id": "1", "host_name": "null"})
		if _, exists := object["sessionId"]; exists {
			t.Fatal("server field sessionId escaped into public JSON")
		}
		var decoded Connection
		if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(decoded, connections[0]) {
			t.Fatalf("public Connection cannot read its own JSON: decoded=%+v error=%v", decoded, err)
		}
	})
	for _, kind := range []string{"online", "monthly", "operations"} {
		t.Run(kind, func(t *testing.T) {
			table, _ := billTableFor(kind)
			s := billTestSelf(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == table.page {
					fmt.Fprint(w, billTestPage(table.id))
					return
				}
				fmt.Fprint(w, billTestJSON(kind))
			})
			page, err := s.Bills(context.Background(), BillQuery{Kind: kind})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(page)
			if err != nil {
				t.Fatal(err)
			}
			var decoded BillPage
			if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(decoded, *page) {
				t.Fatalf("public BillPage cannot read its own JSON: decoded=%+v error=%v", decoded, err)
			}
			root := checkObject(t, encoded, map[string]string{"kind": `"` + kind + `"`})
			payload := checkObject(t, root[kind], map[string]string{"total": "1"})
			if kind == "online" {
				checkObject(t, payload["summary"], map[string]string{"count": "1", "flow": "1.25"})
				var rows []json.RawMessage
				if err := json.Unmarshal(payload["rows"], &rows); err != nil || len(rows) != 1 {
					t.Fatalf("online rows=%s error=%v", payload["rows"], err)
				}
				checkObject(t, rows[0], map[string]string{"area": "0", "login_time": "1", "user_ip": `"10.0.0.1"`, "cost_style_id": "3"})
			} else if kind == "monthly" {
				checkObject(t, payload["summary"], map[string]string{"use_time": "3.5", "base_money": "1.25", "used_money": "2.5", "flow": "4.5"})
			}
		})
	}
}

func billTestJSON(kind string) string {
	switch kind {
	case "online":
		return `{"total":1,"rows":[{"area":0,"chinanetDownFlow":0,"chinanetUpFlow":0,"costId":1,"costMoney":0,"costStyleId":3,"extend":"","flow":1.25,"internetDownFlow":0,"internetUpFlow":0,"loginTime":1,"logoutTime":2,"macAddress":"112233445566","mainControlId":0,"mutliGroupId":0,"nasIp":"10.0.0.2","nasPort":1,"otherFlow":0,"time":60,"userGroupId":1,"userId":1,"userIp":"10.0.0.1","userName":"fixture-account","userPhone":"","userRealName":""}],"summary":{"COU":1,"FLOW":1.25}}`
	case "monthly":
		return `{"total":1,"rows":[[1,2,"plan",1.25,2.5,3.5,4.5,3]],"summary":{"USETIME":3.5,"USEBASEMONEY":1.25,"USEFLOW":4.5,"USEDMONEY":2.5}}`
	case "operations":
		return `{"total":1,"rows":[[1,"description","terminal",7,"remark"]]}`
	}
	panic("unknown fixture bill kind")
}

func billTestPage(id string) string {
	return `<html><script>$.get("/Self/dashboard/refreshaccount", { csrftoken: 'bill-token' });</script><a href="/Self/login/logout">退出</a><table id="` + id + `"></table>` +
		`<select id="year"><option value="2024">2024</option><option value="2025" selected>2025</option></select></html>`
}

func billTestSelf(t *testing.T, handler http.HandlerFunc) *Session {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := server.Client()
	client.Jar, err = cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &Session{client: client, base: base, identity: "fixture-account", authenticated: true}
}

func TestBillsTableParameters(t *testing.T) {
	tests := []struct {
		name string
		q    BillQuery
		want url.Values
	}{
		{"online", BillQuery{Kind: "online", Start: "2024-12-01", End: "2024-12-31"}, url.Values{
			"startTime": {"2024-12-01"}, "endTime": {"2024-12-31"}, "pageNumber": {"1"}, "pageSize": {"10"}, "sortName": {"loginTime"}, "sortOrder": {"DESC"}, "searchText": {""},
		}},
		{"monthly", BillQuery{Kind: "monthly", Year: 2024, Page: 2, Size: 25, Sort: "7", Order: "asc"}, url.Values{
			"year": {"2024"}, "pageNumber": {"2"}, "pageSize": {"25"}, "sortName": {"7"}, "sortOrder": {"ASC"}, "searchText": {""},
		}},
		{"operations", BillQuery{Kind: "operations", Start: "2024-12-01", End: "2024-12-31", Page: 3, Size: 50, Sort: "4"}, url.Values{
			"startTime": {"2024-12-01"}, "endTime": {"2024-12-31"}, "pageNumber": {"3"}, "pageSize": {"50"}, "sortName": {"4"}, "sortOrder": {"DESC"}, "searchText": {""},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, _ := billTableFor(tt.q.Kind)
			var seen []string
			s := billTestSelf(t, func(w http.ResponseWriter, r *http.Request) {
				seen = append(seen, r.URL.Path)
				if r.Method != http.MethodGet {
					t.Errorf("method = %s", r.Method)
				}
				switch r.URL.Path {
				case table.page:
					fmt.Fprint(w, billTestPage(table.id))
				case table.query:
					params := r.URL.Query()
					if params.Has("ajaxCsrfToken") || params.Get("t") == "" {
						t.Errorf("unexpected AJAX parameters; this deployment defines no AJAX CSRF token")
					}
					params.Del("t")
					if !reflect.DeepEqual(params, tt.want) {
						t.Errorf("params = %v, want %v", params, tt.want)
					}
					fmt.Fprint(w, billTestJSON(tt.q.Kind))
				default:
					http.NotFound(w, r)
				}
			})
			got, err := s.Bills(context.Background(), tt.q)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != tt.q.Kind {
				t.Errorf("bill result kind changed: %+v", got)
			}
			switch got.Kind {
			case "online":
				if got.Online.Total != 1 || got.Online.Rows[0].Flow != "1.25" || *got.Online.Summary.Count != "1" {
					t.Fatalf("online bill data changed: %+v", got.Online)
				}
			case "monthly":
				if got.Monthly.Rows[0].Plan != "plan" || got.Monthly.Rows[0].Flow != "4.5" || got.Monthly.Summary.UsedMoney != "2.5" {
					t.Fatalf("monthly bill columns changed: %+v", got.Monthly)
				}
			case "operations":
				if got.Operations.Rows[0].Description != "description" || got.Operations.Rows[0].ServerField3 != "7" || got.Operations.Rows[0].Remark != "remark" {
					t.Fatalf("operation bill columns changed: %+v", got.Operations)
				}
			}
			if !reflect.DeepEqual(seen, []string{table.page, table.query}) {
				t.Errorf("requests = %v", seen)
			}
		})
	}
}

func TestExportBillsScopeAndSession(t *testing.T) {
	workbook := append([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}, []byte("fixture")...)
	for _, kind := range []string{"online", "monthly", "operations"} {
		for _, all := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/all=%t", kind, all), func(t *testing.T) {
				table, _ := billTableFor(kind)
				q := BillQuery{Kind: kind, Page: 2, Size: 50}
				if kind == "monthly" {
					q.Year = 2024
				} else {
					q.Start, q.End = "2024-12-01", "2024-12-31"
				}
				var seen []string
				cached := false
				s := billTestSelf(t, func(w http.ResponseWriter, r *http.Request) {
					seen = append(seen, r.URL.Path)
					if r.URL.Path == table.page {
						http.SetCookie(w, &http.Cookie{Name: "bill-session", Value: "same", Path: "/"})
						fmt.Fprint(w, billTestPage(table.id))
						return
					}
					if cookie, err := r.Cookie("bill-session"); err != nil || cookie.Value != "same" {
						t.Errorf("management session was not preserved")
					}
					switch r.URL.Path {
					case table.query:
						cached = r.URL.Query().Get("pageNumber") == "2" && r.URL.Query().Get("pageSize") == "50"
						fmt.Fprint(w, billTestJSON(kind))
					case table.export:
						want := url.Values{"type": {"1"}}
						if all {
							want.Set("type", "2")
						} else if !cached {
							t.Error("current-page export was not preceded by its query")
						}
						if kind == "operations" || kind == "online" && all {
							want.Set("startTime", q.Start)
							want.Set("endTime", q.End)
						}
						if kind == "monthly" && all {
							want.Set("year", "2024")
						}
						if kind == "operations" {
							want.Set("sortName", "0")
							want.Set("sortOrder", "DESC")
						}
						if got := r.URL.Query(); !reflect.DeepEqual(got, want) {
							t.Errorf("export params = %v, want %v", got, want)
						}
						w.Header().Set("Content-Disposition", "attachment; filename=账单.xls")
						w.Header().Set("Content-Type", "file;charset=UTF-8")
						w.Write(workbook)
					default:
						http.NotFound(w, r)
					}
				})
				got, err := s.ExportBills(context.Background(), q, all)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, workbook) {
					t.Error("workbook changed")
				}
				wantSeen := []string{table.page, table.query, table.export}
				if all {
					wantSeen = []string{table.page, table.export}
				}
				if !reflect.DeepEqual(seen, wantSeen) {
					t.Errorf("requests = %v, want %v", seen, wantSeen)
				}
			})
		}
	}
}

func TestExportBillsRejectsFakeSuccess(t *testing.T) {
	tests := []struct {
		name, disposition, body string
	}{
		{"HTML login", "", `<html><form action="/Self/login/verify"></form></html>`},
		{"HTML attachment", "attachment; filename=账单.xls", `<html>business error</html>`},
		{"JSON attachment", "attachment; filename=账单.xls", `{"success":false}`},
		{"wrong filename", "attachment; filename=账单.html", string([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})},
		{"inline workbook", "inline; filename=账单.xls", string([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})},
		{"extra parameter", "attachment; filename=账单.xls;other=1", string([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})},
		{"empty filename", "attachment; filename=.xls", string([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := billTestSelf(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Self/bill/userOnlineLog" {
					fmt.Fprint(w, billTestPage("userOnlineList"))
					return
				}
				w.Header().Set("Content-Disposition", tt.disposition)
				w.Header().Set("Content-Type", "file;charset=UTF-8")
				fmt.Fprint(w, tt.body)
			})
			if _, err := s.ExportBills(context.Background(), BillQuery{Kind: "online"}, true); err == nil {
				t.Fatal("accepted invalid XLS export")
			}
		})
	}
}

func TestBillsRejectUnexpectedTableResponse(t *testing.T) {
	for _, body := range []string{`{"success":false}`, `{"total":-1,"rows":[]}`, `{"total":1,"rows":{}}`, `{"total":1,"rows":null}`, `{"total":"1","rows":[]}`} {
		t.Run(body, func(t *testing.T) {
			s := billTestSelf(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Self/bill/userOnlineLog" {
					fmt.Fprint(w, billTestPage("userOnlineList"))
				} else {
					fmt.Fprint(w, body)
				}
			})
			if _, err := s.Bills(context.Background(), BillQuery{Kind: "online"}); err == nil {
				t.Fatal("accepted invalid table response")
			}
		})
	}
}

func TestNormalizeBillDateRange(t *testing.T) {
	now := time.Date(2026, 10, 6, 17, 0, 0, 0, time.UTC)
	q, err := normalizeBill(BillQuery{Kind: "online"}, now)
	if err != nil || q.Start != "2026-10-07" || q.End != q.Start {
		t.Fatalf("school calendar default = %+v, %v", q, err)
	}
	for _, dates := range [][2]string{{"2024-01-01", "2024-03-01"}, {"2024-12-01", "2025-01-30"}, {"2024-02-29", "2024-02-29"}} {
		if _, err := normalizeBill(BillQuery{Kind: "online", Start: dates[0], End: dates[1]}, now); err != nil {
			t.Errorf("rejected valid calendar range %v: %v", dates, err)
		}
	}
	for _, dates := range [][2]string{{"2024-01-01", "2024-03-02"}, {"2024-12-01", "2025-01-31"}, {"2025-02-29", "2025-03-01"}, {"2024-12-02", "2024-12-01"}, {"2026-10-07", "2026-10-08"}, {"2024-1-01", "2024-01-31"}, {"2024-01-01", ""}} {
		if _, err := normalizeBill(BillQuery{Kind: "online", Start: dates[0], End: dates[1]}, now); err == nil {
			t.Errorf("accepted invalid calendar range %v", dates)
		}
	}
}

func TestNormalizeBillRejectsUnrelatedAndInvalidOptions(t *testing.T) {
	for _, q := range []BillQuery{
		{Kind: "unknown"}, {Kind: "online", Year: 2024}, {Kind: "monthly", Start: "2024-01-01", End: "2024-01-02"},
		{Kind: "online", Page: -1}, {Kind: "online", Size: 25}, {Kind: "monthly", Size: 20},
		{Kind: "monthly", Year: -1}, {Kind: "monthly", Sort: "8"}, {Kind: "operations", Sort: "3"},
		{Kind: "online", Sort: "userName"}, {Kind: "operations", Order: "sideways"},
	} {
		if _, err := normalizeBill(q, time.Now()); err == nil {
			t.Errorf("accepted invalid query %+v", q)
		}
	}
}

func TestBillsMonthlyYearComesFromPage(t *testing.T) {
	for _, year := range []int{0, 2024, 2023} {
		t.Run(fmt.Sprint(year), func(t *testing.T) {
			var queriedYear string
			s := billTestSelf(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Self/bill/monthPay" {
					fmt.Fprint(w, billTestPage("monthPay"))
				} else {
					queriedYear = r.URL.Query().Get("year")
					fmt.Fprint(w, billTestJSON("monthly"))
				}
			})
			_, err := s.Bills(context.Background(), BillQuery{Kind: "monthly", Year: year})
			if year == 2023 {
				if err == nil || queriedYear != "" {
					t.Error("queried a year not offered by the page")
				}
				return
			}
			want := "2025"
			if year == 2024 {
				want = "2024"
			}
			if err != nil || queriedYear != want {
				t.Errorf("year = %s, want %s; error %v", queriedYear, want, err)
			}
		})
	}
}

func TestBillsRejectWrongPage(t *testing.T) {
	s := billTestSelf(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.ReplaceAll(billTestPage("userOnlineList"), "userOnlineList", "differentTable"))
	})
	if _, err := s.Bills(context.Background(), BillQuery{Kind: "online"}); err == nil {
		t.Fatal("accepted a page without the expected bill table")
	}
}
