package p

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hicancan/njupt-net/v4/network"
)

func TestPortalTraceUsesOnlyNativeDiagnosticFields(t *testing.T) {
	policy := tracePayload("login", "dr8")
	payload, shape := policy([]byte(`dr8({"result":0,"msg":"从Radius获取错误代码失败！","ret_code":2,"password":"secret","self_auth_url":"https://self.invalid/bridge?token=secret"});`))
	if shape != "jsonp" || payload["result"] != json.Number("0") || payload["ret_code"] != json.Number("2") || payload["msg"] != "从Radius获取错误代码失败！" || len(payload) != 3 {
		t.Fatalf("unexpected diagnostic projection: shape=%q payload=%v", shape, payload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil || strings.Contains(string(encoded), "secret") {
		t.Fatalf("diagnostics exposed an unlisted native field: %s, %v", encoded, err)
	}
	if tracePayload("page/loadConfig", "dr8") != nil || tracePayload("login4self", "dr8") != nil {
		t.Fatal("browser configuration or bridge acquired a response-body policy")
	}
}

func TestPortalTraceOnlineIdentityAndErrorFields(t *testing.T) {
	payload, shape := tracePayload("online_list", "dr9")([]byte(`dr9({"result":1,"total":1,"list":[{"user_account":"student@cmcc","online_ip":"192.0.2.1","online_mac":"AABBCCDDEEFF","online_session":"session-observation","token":"secret","csrf":"secret"}]});`))
	rows, ok := payload["list"].([]any)
	if shape != "jsonp" || !ok || len(rows) != 1 {
		t.Fatalf("online projection missing identity: %q %v", shape, payload)
	}
	row := rows[0].(map[string]any)
	if len(row) != 4 || row["user_account"] != "student@cmcc" || row["online_session"] != "session-observation" {
		t.Fatalf("online identity changed or included extra fields: %v", row)
	}
	payload, shape = tracePayload("err_code", "dr10")([]byte(`dr10({"result":1,"error_code":"E3","error_prompt_zh":"提示","error_prompt_en":"prompt","unknown":"secret"});`))
	if shape != "jsonp" || payload["error_code"] != "E3" || payload["error_prompt_zh"] != "提示" || len(payload) != 4 {
		t.Fatalf("error diagnostic fields changed: %v", payload)
	}
}

func TestPortalTraceRejectsInvalidJSONPAndNestedFields(t *testing.T) {
	policy := tracePayload("login", "dr1")
	for _, body := range []string{`dr2({"result":1});`, `dr1({"result":1}`, `dr1(["secret"]);`, "dr1({\"msg\":\"\xff\"});"} {
		payload, shape := policy([]byte(body))
		if payload != nil || shape != "invalid_jsonp" {
			t.Fatalf("invalid body was retained: %q %v %q", body, payload, shape)
		}
	}
	payload, shape := policy([]byte(`dr1({"result":1,"msg":{"password":"secret"},"ret_code":["secret"]});`))
	if shape != "jsonp" || len(payload) != 1 {
		t.Fatalf("nested native fields entered diagnostics: %v", payload)
	}
}

func TestPortalTraceSeparatesAuthenticationFromIdentityVerification(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "application/javascript")
		switch request.URL.Path {
		case "/eportal/portal/login":
			fmt.Fprintf(writer, `%s({"result":1,"msg":"accepted","ret_code":0,"user_password":"trace-password-secret","self_auth_url":"https://self.invalid/bridge?token=trace-token-secret"});`, request.URL.Query().Get("callback"))
		case "/eportal/portal/online_list":
			fmt.Fprintf(writer, `%s({"result":1,"total":1,"list":[{"user_account":"student@cmcc","online_ip":"127.0.0.1","online_mac":"AABBCCDDEEFF","online_session":"42","csrf":"trace-csrf-secret"}]});`, request.URL.Query().Get("callback"))
		default:
			t.Errorf("unexpected native HTTP request: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	link, err := network.NewLink("127.0.0.1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer link.Close()
	portal, err := newPortal(link.ClientFor("p.njupt.edu.cn", netip.MustParseAddr("127.0.0.1")), "127.0.0.1", "pc", 801)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _ := url.Parse(server.URL)
	endpoint.Host = net.JoinHostPort("p.njupt.edu.cn", endpoint.Port())
	endpoint.Path = "/eportal/portal/"
	portal.api = endpoint.String()
	var output strings.Builder
	link.SetTrace(&output)
	result, err := portal.Login(network.WithTraceCommand(context.Background(), "3", "p login"), "student", "trace-password-secret", "cmcc")
	if err != nil || !result.Verified || requests.Load() != 2 {
		t.Fatalf("trace changed native login or verification: requests=%d result=%+v error=%v", requests.Load(), result, err)
	}
	decoder := json.NewDecoder(strings.NewReader(output.String()))
	for _, path := range []string{"/login", "/online_list"} {
		var record struct {
			URL, CommandID string
			Payload        map[string]any
		}
		// JSON field names are explicitly mapped for the public trace format.
		var raw map[string]json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(raw["url"], &record.URL)
		_ = json.Unmarshal(raw["command_id"], &record.CommandID)
		_ = json.Unmarshal(raw["payload"], &record.Payload)
		if !strings.HasSuffix(record.URL, path) || record.CommandID != "3" || record.Payload["result"] != float64(1) {
			t.Fatalf("native requests are not independently associated: %+v", record)
		}
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("trace introduced another request: %v", err)
	}
	for _, secret := range []string{"trace-password-secret", "trace-token-secret", "trace-csrf-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("portal diagnostics exposed %q", secret)
		}
	}
}
