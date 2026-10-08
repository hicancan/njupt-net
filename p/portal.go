package p

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/hicancan/njupt-net/v4/network"
)

const portalHost = "p.njupt.edu.cn"

// RuntimeConfig contains the selected endpoint and its native configuration.
// The authentication nonce remains private to the portal.
type RuntimeConfig struct {
	Source               string `json:"source"`
	Endpoint             string `json:"endpoint"`
	Terminal             int    `json:"terminal"`
	Program              string `json:"program"`
	Page                 string `json:"page"`
	LoginMethod          int    `json:"login_method"`
	CheckOnlineMethod    int    `json:"check_online_method"`
	AccountPrefix        bool   `json:"account_prefix"`
	IPadTerminalIdentity bool   `json:"ipad_terminal_identity"`
	EncodeCredentials    bool   `json:"encode_credentials"`
	RegisterMode         string `json:"register_mode"`
	ACLogout             string `json:"ac_logout"`
}

func New(link *network.Link, terminal string) (*Portal, error) {
	return NewAt(link, terminal, 804)
}

// NewAt selects one deployed API endpoint without discovery or port fallback.
func NewAt(link *network.Link, terminal string, port int) (*Portal, error) {
	if link == nil {
		return nil, fmt.Errorf("portal requires a network link")
	}
	return newPortal(link.ClientFor(portalHost, netip.AddrFrom4([4]byte{10, 10, 244, 11})), link.Source(), terminal, port)
}

func (p *Portal) runtimeConfig() *RuntimeConfig {
	return &RuntimeConfig{Source: p.source, Endpoint: p.api, Terminal: p.terminal,
		Program: scalar(p.settings["program_index"]), Page: scalar(p.settings["page_index"]),
		LoginMethod: 1, CheckOnlineMethod: 1, AccountPrefix: scalar(p.settings["account_prefix"]) == "1",
		IPadTerminalIdentity: scalar(p.settings["ipad_terminal_identity"]) == "1", EncodeCredentials: scalar(p.settings["no_filter_accandpwd"]) == "1",
		RegisterMode: scalar(p.settings["register_mode"]), ACLogout: scalar(p.settings["ac_logout"])}
}

// Portal operates on the terminal reached through its client's bound source.
// A Portal is a single context: concurrent operations are serialized.
type Portal struct {
	mu           sync.Mutex
	client       *http.Client
	source       string
	terminal     int
	api          string
	sequence     uint64
	stateTimeout time.Duration
	configured   bool
	mac          string
	settings     map[string]json.RawMessage
}

func newPortal(client *http.Client, source, terminal string, port int) (*Portal, error) {
	if client == nil {
		return nil, fmt.Errorf("portal requires an HTTP client")
	}
	ip := net.ParseIP(source)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() {
		return nil, fmt.Errorf("portal requires a concrete IPv4 source")
	}
	scheme := ""
	switch port {
	case 801, 803:
		scheme = "http"
	case 802, 804:
		scheme = "https"
	default:
		return nil, fmt.Errorf("portal port must be 801, 802, 803 or 804")
	}
	types := map[string]int{"pc": 1, "mobile": 2, "hipad": 3, "vipad": 4}
	t, ok := types[terminal]
	if !ok {
		return nil, fmt.Errorf("terminal must be pc, mobile, hipad or vipad")
	}
	return &Portal{client: client, source: ip.String(), terminal: t,
		api: fmt.Sprintf("%s://%s:%d/eportal/portal/", scheme, portalHost, port),
		mac: "000000000000", stateTimeout: 10 * time.Second}, nil
}

// Configure reads page/loadConfig directly. It does not fetch browser resources.
func (p *Portal) Configure(ctx context.Context) (*RuntimeConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.configure(ctx)
}

func (p *Portal) configure(ctx context.Context) (*RuntimeConfig, error) {
	p.configured, p.settings = false, nil
	obj, err := p.call(ctx, "page/loadConfig", nil)
	if err != nil {
		return nil, err
	}
	if scalar(obj["code"]) != "1" {
		return nil, fmt.Errorf("portal configuration was rejected")
	}
	settings, err := decodeObject(obj["data"])
	if err != nil {
		return nil, fmt.Errorf("invalid portal configuration data: %w", err)
	}
	for _, key := range []string{"program_index", "page_index", "login_method", "check_online_method", "account_prefix", "ipad_terminal_identity", "no_filter_accandpwd", "enable_r3", "en_md5", "register_mode", "ac_logout", "rcn"} {
		if _, ok := settings[key]; !ok {
			return nil, fmt.Errorf("portal configuration is missing %s", key)
		}
	}
	for key, want := range map[string]string{"login_method": "1", "check_online_method": "1", "enable_r3": "0", "en_md5": "0"} {
		if scalar(settings[key]) != want {
			return nil, fmt.Errorf("unsupported portal setting %s", key)
		}
	}
	for _, key := range []string{"account_prefix", "ipad_terminal_identity", "no_filter_accandpwd"} {
		v := scalar(settings[key])
		if v != "0" && v != "1" {
			return nil, fmt.Errorf("invalid portal setting %s", key)
		}
	}
	if scalar(settings["program_index"]) == "" || scalar(settings["page_index"]) == "" || scalar(settings["rcn"]) == "" {
		return nil, fmt.Errorf("portal configuration contains an empty program, page or nonce")
	}
	p.settings, p.configured = settings, true
	return p.runtimeConfig(), nil
}

func (p *Portal) ensure(ctx context.Context) error {
	if !p.configured {
		_, err := p.configure(ctx)
		return err
	}
	return nil
}

func (p *Portal) terminalFields() url.Values {
	return url.Values{"wlan_user_ip": {p.source}, "wlan_user_ipv6": {""}, "wlan_user_mac": {p.mac}, "wlan_vlan_id": {""}, "wlan_ac_ip": {""}, "wlan_ac_name": {""}}
}

func (p *Portal) call(ctx context.Context, path string, data url.Values) (map[string]json.RawMessage, error) {
	if data == nil {
		data = url.Values{}
	}
	p.sequence++
	callback := "dr" + strconv.FormatUint(p.sequence, 10)
	data.Set("callback", callback)
	if path == "logout" || path == "change_pass" {
		nonce, err := requestNonce()
		if err != nil {
			return nil, err
		}
		data.Set("program_index", scalar(p.settings["program_index"]))
		data.Set("page_index", scalar(p.settings["page_index"]))
		data.Set("v", nonce)
		data.Add("lang", "zh")
	}
	var response []byte
	var err error
	switch path {
	case "page/loadConfig", "online_list", "err_code":
		response, _, _, err = network.Read(ctx, p.client, p.api+path, data)
	default:
		response, _, _, err = network.Request(ctx, p.client, http.MethodGet, p.api+path, data)
	}
	if err != nil {
		return nil, err
	}
	return decodeJSONP(response, callback)
}

func decodeJSONP(data []byte, callback string) (map[string]json.RawMessage, error) {
	data = bytes.TrimSpace(data)
	prefix := []byte(callback + "(")
	if !bytes.HasPrefix(data, prefix) {
		return nil, fmt.Errorf("portal JSONP callback does not match the request")
	}
	data = bytes.TrimSpace(data[len(prefix):])
	if bytes.HasSuffix(data, []byte(";")) {
		data = bytes.TrimSpace(data[:len(data)-1])
	}
	if !bytes.HasSuffix(data, []byte(")")) {
		return nil, fmt.Errorf("invalid portal JSONP ending")
	}
	data = bytes.TrimSpace(data[:len(data)-1])
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("portal JSONP does not contain valid UTF-8 JSON")
	}
	return decodeObject(data)
}

func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("portal response must be a JSON object")
	}
	return obj, nil
}

func scalar(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

func base64Text(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
func validateCredential(account, password string) error {
	if account == "" || password == "" {
		return fmt.Errorf("account and password are required")
	}
	return nil
}
func requestNonce() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return "", fmt.Errorf("create portal request nonce: %w", err)
	}
	return strconv.FormatInt(n.Int64()+500, 10), nil
}
func normalizeMAC(s string) (string, error) {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), ":", ""), "-", "")
	if !regexp.MustCompile(`^[0-9a-fA-F]{12}$`).MatchString(s) {
		return "", fmt.Errorf("invalid portal terminal MAC")
	}
	return s, nil
}
