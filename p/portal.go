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
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/hicancan/njupt-net/v3/network"
)

// RuntimeConfig is the current terminal context and supported portal protocol.
// The authentication nonce remains private to the portal.
type RuntimeConfig struct {
	Source               string `json:"source"`
	Terminal             int    `json:"terminal"`
	Program              string `json:"program"`
	Page                 string `json:"page"`
	Version              string `json:"version"`
	IP                   string `json:"ip"`
	IPv6                 string `json:"ipv6,omitempty"`
	MAC                  string `json:"mac"`
	VLAN                 string `json:"vlan"`
	ACIP                 string `json:"ac_ip,omitempty"`
	ACName               string `json:"ac_name,omitempty"`
	SSID                 string `json:"ssid,omitempty"`
	Area                 string `json:"area,omitempty"`
	APMAC                string `json:"ap_mac"`
	Gateway              string `json:"gateway"`
	LoginMethod          int    `json:"login_method"`
	CheckOnlineMethod    int    `json:"check_online_method"`
	AccountPrefix        bool   `json:"account_prefix"`
	IPadTerminalIdentity bool   `json:"ipad_terminal_identity"`
	EncodeCredentials    bool   `json:"encode_credentials"`
	RegisterMode         string `json:"register_mode"`
	ACLogout             string `json:"ac_logout"`
}

func New(link *network.Link, terminal string) (*Portal, error) {
	if link == nil {
		return nil, fmt.Errorf("portal requires a network link")
	}
	return newPortal(link.Client(), link.Source(), terminal)
}

func (p *Portal) runtimeConfig() *RuntimeConfig {
	return &RuntimeConfig{Source: p.source, Terminal: p.terminal, Program: p.program, Page: p.page, Version: p.version, IP: p.ip, IPv6: p.ipv6, MAC: p.mac, VLAN: p.vlan, ACIP: p.acIP, ACName: p.acName, SSID: p.ssid, Area: p.area, APMAC: p.apMAC, Gateway: p.gateway, LoginMethod: 1, CheckOnlineMethod: 1, AccountPrefix: scalar(p.settings["account_prefix"]) == "1", IPadTerminalIdentity: scalar(p.settings["ipad_terminal_identity"]) == "1", EncodeCredentials: scalar(p.settings["no_filter_accandpwd"]) == "1", RegisterMode: scalar(p.settings["register_mode"]), ACLogout: scalar(p.settings["ac_logout"])}
}

// Portal operates on the terminal reached through its client's bound source.
// A Portal is a single context: concurrent operations are serialized.
type Portal struct {
	mu                                                            sync.Mutex
	client                                                        *http.Client
	source                                                        string
	terminal                                                      int
	base                                                          string
	api                                                           string
	sequence                                                      uint64
	stateTimeout                                                  time.Duration
	configured                                                    bool
	program, page, version                                        string
	ip, ipv6, mac, vlan, acIP, acName, ssid, area, apMAC, gateway string
	settings                                                      map[string]json.RawMessage
}

func newPortal(client *http.Client, source, terminal string) (*Portal, error) {
	if client == nil {
		return nil, fmt.Errorf("portal requires an HTTP client")
	}
	ip := net.ParseIP(source)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() {
		return nil, fmt.Errorf("portal requires a concrete IPv4 source")
	}
	types := map[string]int{"pc": 1, "mobile": 2, "hipad": 3, "vipad": 4}
	t, ok := types[terminal]
	if !ok {
		return nil, fmt.Errorf("terminal must be pc, mobile, hipad or vipad")
	}
	return &Portal{client: client, source: ip.String(), terminal: t, base: "https://p.njupt.edu.cn", api: "https://p.njupt.edu.cn:804/eportal/portal/", stateTimeout: 10 * time.Second}, nil
}

// Configure obtains the current page, program, protocol settings and JS version.
// Only the deployed HTTPS 804 / method 1 contract is accepted.
func (p *Portal) Configure(ctx context.Context) (*RuntimeConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.configure(ctx)
}

func (p *Portal) configure(ctx context.Context) (*RuntimeConfig, error) {
	p.configured = false
	p.program, p.page, p.version = "", "", "4.X" // a41's bootstrap value, before a40 loads.
	html, finalURL, _, err := network.Request(ctx, p.client, http.MethodGet, p.base+"/a79.htm", nil)
	if err != nil {
		return nil, err
	}
	pageText := string(html)
	fileVersion, err := assignment(pageText, "fileVersion")
	if err != nil {
		return nil, err
	}
	ip, err := assignment(pageText, "v46ip")
	if err != nil {
		return nil, err
	}
	p.ip = strings.TrimSpace(ip)
	if p.ip != p.source {
		return nil, fmt.Errorf("portal terminal IP does not match the selected source")
	}
	p.mac, err = assignment(pageText, "ss4")
	if err != nil {
		return nil, err
	}
	p.mac, err = normalizeMAC(p.mac)
	if err != nil {
		return nil, err
	}
	p.vlan, err = assignment(pageText, "vlanid")
	if err != nil {
		return nil, err
	}
	p.ipv6, p.acIP, p.acName, p.ssid, p.area = "", "", "", "", ""
	p.apMAC, p.gateway = "000000000000", "000000000000"
	// AC redirects can carry the terminal context in the current page URL.
	q := finalURL.Query()
	for key, dst := range map[string]*string{"wlanacip": &p.acIP, "wlanacname": &p.acName, "ssid": &p.ssid, "areaID": &p.area, "UserV6IP": &p.ipv6, "vlanid": &p.vlan} {
		if q.Has(key) {
			*dst = q.Get(key)
		}
	}
	if q.Has("wlanuserip") && q.Get("wlanuserip") != p.source {
		return nil, fmt.Errorf("redirect terminal IP does not match the selected source")
	}
	if q.Has("wlanusermac") {
		p.mac, err = normalizeMAC(q.Get("wlanusermac"))
		if err != nil {
			return nil, err
		}
	}
	if q.Has("apmac") {
		p.apMAC, err = normalizeMAC(q.Get("apmac"))
		if err != nil {
			return nil, err
		}
	}
	if q.Has("gw_id") {
		p.gateway, err = normalizeMAC(q.Get("gw_id"))
		if err != nil {
			return nil, err
		}
	}
	js, _, _, err := network.Request(ctx, p.client, http.MethodGet, p.base+"/a41.js", url.Values{"version": {fileVersion}})
	if err != nil {
		return nil, err
	}
	for key, want := range map[string]string{"enableHttps": "1", "enHTTPSPort": "804", "page_data_encrypt": "0", "apg_switch": "0"} {
		got, e := assignment(string(js), key)
		if e != nil {
			return nil, e
		}
		if got != want {
			return nil, fmt.Errorf("unsupported portal setting %s", key)
		}
	}
	data := url.Values{"wlan_vlan_id": {p.vlan}, "wlan_user_ip": {base64Text(p.ip)}, "wlan_user_ipv6": {base64Text(p.ipv6)}, "wlan_user_ssid": {p.ssid}, "wlan_user_areaid": {p.area}, "wlan_ac_ip": {base64Text(p.acIP)}, "wlan_ap_mac": {p.apMAC}, "gw_id": {p.gateway}}
	raw, err := p.call(ctx, "page/loadConfig", data)
	if err != nil {
		return nil, err
	}
	obj, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}
	if scalar(obj["code"]) != "1" {
		return nil, fmt.Errorf("portal configuration was rejected")
	}
	p.settings, err = decodeObject(obj["data"])
	if err != nil {
		return nil, fmt.Errorf("invalid portal configuration data: %w", err)
	}
	for _, key := range []string{"program_index", "page_index", "login_method", "check_online_method", "account_prefix", "ipad_terminal_identity", "no_filter_accandpwd", "enable_r3", "en_md5", "register_mode", "ac_logout", "rcn"} {
		if _, ok := p.settings[key]; !ok {
			return nil, fmt.Errorf("portal configuration is missing %s", key)
		}
	}
	for key, want := range map[string]string{"login_method": "1", "check_online_method": "1", "enable_r3": "0", "en_md5": "0"} {
		if scalar(p.settings[key]) != want {
			return nil, fmt.Errorf("unsupported portal setting %s", key)
		}
	}
	for _, key := range []string{"account_prefix", "ipad_terminal_identity", "no_filter_accandpwd"} {
		v := scalar(p.settings[key])
		if v != "0" && v != "1" {
			return nil, fmt.Errorf("invalid portal setting %s", key)
		}
	}
	p.program, p.page = scalar(p.settings["program_index"]), scalar(p.settings["page_index"])
	if p.program == "" || p.page == "" || scalar(p.settings["rcn"]) == "" {
		return nil, fmt.Errorf("portal configuration contains an empty program, page or nonce")
	}
	js, _, _, err = network.Request(ctx, p.client, http.MethodGet, p.base+"/a40.js", url.Values{"v": {"_" + fileVersion}})
	if err != nil {
		return nil, err
	}
	p.version, err = assignment(string(js), "jsVersion")
	if err != nil {
		return nil, err
	}
	if p.version == "" {
		return nil, fmt.Errorf("empty portal JS version")
	}
	p.configured = true
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
	return url.Values{"wlan_user_ip": {p.ip}, "wlan_user_ipv6": {p.ipv6}, "wlan_user_mac": {p.mac}, "wlan_vlan_id": {p.vlan}, "wlan_ac_ip": {p.acIP}, "wlan_ac_name": {p.acName}}
}

func (p *Portal) call(ctx context.Context, path string, data url.Values) (json.RawMessage, error) {
	if data == nil {
		data = url.Values{}
	}
	p.sequence++
	callback := "dr" + strconv.FormatUint(p.sequence, 10)
	nonce, err := requestNonce()
	if err != nil {
		return nil, err
	}
	data.Set("program_index", p.program)
	data.Set("page_index", p.page)
	data.Set("callback", callback)
	data.Set("jsVersion", p.version)
	data.Set("v", nonce)
	// The browser wrapper appends its language even when login data has zh-cn.
	data.Add("lang", "zh")
	response, _, _, err := network.Request(ctx, p.client, http.MethodGet, p.api+path, data)
	if err != nil {
		return nil, err
	}
	return decodeJSONP(response, callback)
}

func decodeJSONP(data []byte, callback string) (json.RawMessage, error) {
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
	if !utf8.Valid(data) || !json.Valid(data) {
		return nil, fmt.Errorf("portal JSONP does not contain valid UTF-8 JSON")
	}
	if _, err := decodeObject(data); err != nil {
		return nil, err
	}
	return json.RawMessage(bytes.Clone(data)), nil
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

// Read only literal assignments; fetched JavaScript is never executed.
func assignment(text, name string) (string, error) {
	re := regexp.MustCompile(`(?:^|[;\r\n])\s*(?:var\s+)?` + regexp.QuoteMeta(name) + `\s*=\s*(?:'([^'\\]*)'|"([^"\\]*)"|([0-9]+))\s*(?:[;,]|$)`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return "", fmt.Errorf("portal page is missing literal %s", name)
	}
	if m[1] != "" {
		return m[1], nil
	}
	if m[2] != "" {
		return m[2], nil
	}
	return m[3], nil
}
