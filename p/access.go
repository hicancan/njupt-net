package p

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hicancan/njupt-net/v3/network"
)

// Terminal is the online session belonging to the selected source address.
type Terminal struct {
	Account string `json:"account"`
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Session string `json:"session,omitempty"`
}

type Status struct {
	Source   string    `json:"source"`
	Online   bool      `json:"online"`
	Terminal *Terminal `json:"terminal"`
}

// Outcome separates server acceptance from rejection and an unreadable reply.
type Outcome string

const (
	Accepted     Outcome = "accepted"
	Rejected     Outcome = "rejected"
	Unknown      Outcome = "unknown"
	NotSubmitted Outcome = "not_submitted"
)

type OperationResult struct {
	Outcome     Outcome `json:"outcome"`
	Verified    bool    `json:"verified"`
	Message     string  `json:"message,omitempty"`
	Status      *Status `json:"status,omitempty"`
	SelfURL     string  `json:"self_url,omitempty"`
	OperatorURL string  `json:"operator_url,omitempty"`
}

func (p *Portal) Status(ctx context.Context) (*Status, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	return p.status(ctx)
}

func (p *Portal) status(ctx context.Context) (*Status, error) {
	raw, err := p.call(ctx, "online_list", url.Values{"user_account": {""}, "user_password": {""}, "wlan_user_mac": {strings.ToUpper(p.mac)}, "wlan_user_ip": {base64Text(p.ip)}, "wlan_user_ipv6": {base64Text(p.ipv6)}})
	if err != nil {
		return nil, err
	}
	obj, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}
	state := &Status{Source: p.source}
	switch scalar(obj["result"]) {
	case "0":
		if scalar(obj["msg"]) != "获取用户在线信息数据为空！" {
			return nil, fmt.Errorf("portal rejected the online-session query: %s", scalar(obj["msg"]))
		}
		if _, ok := obj["list"]; ok {
			return nil, fmt.Errorf("portal offline response unexpectedly contains an online-session list")
		}
		if _, ok := obj["total"]; ok {
			return nil, fmt.Errorf("portal offline response unexpectedly contains a session total")
		}
		return state, nil
	case "1":
	default:
		return nil, fmt.Errorf("portal status has an unknown result")
	}
	var rows []json.RawMessage
	if err = json.Unmarshal(obj["list"], &rows); err != nil || rows == nil {
		return nil, fmt.Errorf("portal status must contain an online-session list")
	}
	total, err := strconv.Atoi(scalar(obj["total"]))
	if err != nil || total != len(rows) {
		return nil, fmt.Errorf("portal status total does not match its session list")
	}
	for _, row := range rows {
		entry, err := decodeObject(row)
		if err != nil {
			return nil, err
		}
		address := scalar(entry["online_ip"])
		if net.ParseIP(address) == nil {
			return nil, fmt.Errorf("portal session has an invalid terminal IP")
		}
		if address != p.ip {
			continue
		}
		if state.Terminal != nil {
			return nil, fmt.Errorf("portal returned multiple sessions for the selected terminal IP")
		}
		account := scalar(entry["user_account"])
		if account == "" {
			return nil, fmt.Errorf("portal terminal session is missing its account")
		}
		mac, err := normalizeMAC(scalar(entry["online_mac"]))
		if err != nil {
			return nil, err
		}
		p.mac = mac
		state.Terminal = &Terminal{Account: account, IP: address, MAC: mac, Session: scalar(entry["online_session"])}
	}
	state.Online = state.Terminal != nil
	return state, nil
}

func (p *Portal) Login(ctx context.Context, account, password, operator string) (*OperationResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := validateCredential(account, password); err != nil {
		return nil, err
	}
	suffixes := map[string]string{"campus": "", "njxy": "@njxy", "cmcc": "@cmcc"}
	suffix, ok := suffixes[operator]
	if !ok {
		return nil, fmt.Errorf("operator must be campus, njxy or cmcc")
	}
	if strings.ContainsAny(account, ",@") {
		return nil, fmt.Errorf("portal account must not contain a terminal prefix or operator suffix")
	}
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	before, err := p.status(ctx)
	if err != nil {
		return nil, err
	}
	if before.Online {
		return &OperationResult{Outcome: NotSubmitted, Status: before}, fmt.Errorf("terminal is already online; no password authentication was submitted")
	}
	macType := "0"
	if p.terminal == 2 || (p.terminal >= 3 && scalar(p.settings["ipad_terminal_identity"]) == "1") {
		macType = "1"
	}
	account += suffix
	wireAccount := account
	if scalar(p.settings["account_prefix"]) == "1" {
		wireAccount = "," + macType + "," + wireAccount
	}
	encoded := scalar(p.settings["no_filter_accandpwd"])
	if encoded == "1" {
		wireAccount, password = base64Text(wireAccount), base64Text(password)
	}
	data := p.terminalFields()
	for key, value := range map[string]string{"login_method": "1", "is_base64encode": encoded, "user_account": wireAccount, "user_password": password, "authex_enable": "", "terminal_type": strconv.Itoa(p.terminal), "lang": "zh-cn", "user_agent": network.UserAgent, "enable_r3": "0", "mac_type": macType, "rcn": scalar(p.settings["rcn"]), "operate": "portal_login", "business_type": "1"} {
		data.Set(key, value)
	}
	raw, err := p.call(ctx, "login", data)
	if err != nil {
		return p.unknown(ctx, "login", err)
	}
	result, err := operationResult(raw, "login")
	if err != nil {
		return result, err
	}
	result.Status, err = p.waitState(ctx, true)
	if err != nil {
		return result, fmt.Errorf("portal accepted login; online verification failed: %w", err)
	}
	uid := result.Status.Terminal.Account
	if uid != account && uid != ","+macType+","+account {
		return result, fmt.Errorf("portal online account does not match the submitted account")
	}
	result.Verified = true
	return result, nil
}

func (p *Portal) Logout(ctx context.Context) (*OperationResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	before, err := p.status(ctx)
	if err != nil {
		return nil, err
	}
	if !before.Online {
		return &OperationResult{Outcome: NotSubmitted, Status: before}, fmt.Errorf("terminal is already offline; no logout was submitted")
	}
	data := p.terminalFields()
	for key, value := range map[string]string{"login_method": "1", "user_account": "drcom", "user_password": "123", "ac_logout": scalar(p.settings["ac_logout"]), "register_mode": scalar(p.settings["register_mode"])} {
		data.Set(key, value)
	}
	raw, err := p.call(ctx, "logout", data)
	if err != nil {
		return p.unknown(ctx, "logout", err)
	}
	result, err := operationResult(raw, "logout")
	if err != nil {
		return result, err
	}
	result.Status, err = p.waitState(ctx, false)
	if err != nil {
		return result, fmt.Errorf("portal accepted logout; offline verification failed: %w", err)
	}
	result.Verified = true
	return result, nil
}

// Radius accounting propagates after a successful reply. Observe that change;
// the credential submission itself is never repeated.
func (p *Portal) waitState(ctx context.Context, online bool) (*Status, error) {
	ctx, cancel := context.WithTimeout(ctx, p.stateTimeout)
	defer cancel()
	var last *Status
	for {
		state, err := p.status(ctx)
		if err != nil {
			return last, err
		}
		last = state
		if state.Online == online {
			return state, nil
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, fmt.Errorf("terminal state did not converge within the verification deadline: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// A lost response remains unknown even if a single subsequent observation is
// online. That observation cannot prove the submitted password was accepted.
func (p *Portal) unknown(ctx context.Context, operation string, cause error) (*OperationResult, error) {
	status, err := p.status(ctx)
	result := &OperationResult{Outcome: Unknown, Status: status}
	if err != nil {
		return result, fmt.Errorf("portal %s outcome unknown: %w; state observation failed: %v", operation, cause, err)
	}
	return result, fmt.Errorf("portal %s outcome unknown: %w; terminal state was observed without resubmission", operation, cause)
}

func operationResult(raw json.RawMessage, operation string) (*OperationResult, error) {
	obj, err := decodeObject(raw)
	if err != nil {
		return &OperationResult{Outcome: Unknown}, err
	}
	result := &OperationResult{Outcome: Unknown, Message: scalar(obj["msg"]), SelfURL: scalar(obj["self_auth_url"]), OperatorURL: scalar(obj["self_auth_url2"])}
	switch scalar(obj["result"]) {
	case "1", "ok":
		result.Outcome = Accepted
		return result, nil
	case "0":
		result.Outcome = Rejected
		return result, fmt.Errorf("portal rejected %s: %s", operation, result.Message)
	default:
		return result, fmt.Errorf("portal %s has an unknown result", operation)
	}
}

type ErrorInfo struct {
	Outcome       Outcome `json:"outcome"`
	Message       string  `json:"message,omitempty"`
	Code          string  `json:"code,omitempty"`
	PromptChinese string  `json:"prompt_chinese,omitempty"`
	PromptEnglish string  `json:"prompt_english,omitempty"`
}

func (p *Portal) ErrorInfo(ctx context.Context) (*ErrorInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	raw, err := p.call(ctx, "err_code", url.Values{"wlan_user_ip": {p.ip}, "wlan_user_ipv6": {p.ipv6}, "wlan_user_mac": {p.mac}})
	if err != nil {
		return nil, err
	}
	result, resultErr := operationResult(raw, "error information")
	obj, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}
	return &ErrorInfo{Outcome: result.Outcome, Message: result.Message, Code: scalar(obj["error_code"]), PromptChinese: scalar(obj["error_prompt_zh"]), PromptEnglish: scalar(obj["error_prompt_en"])}, resultErr
}
