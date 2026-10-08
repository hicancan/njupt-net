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
	Outcome     Outcome         `json:"outcome"`
	Verified    bool            `json:"verified"`
	Message     string          `json:"message,omitempty"`
	RetCode     json.RawMessage `json:"ret_code,omitempty"`
	Status      *Status         `json:"status,omitempty"`
	SelfURL     string          `json:"self_url,omitempty"`
	OperatorURL string          `json:"operator_url,omitempty"`
}

func (p *Portal) Status(ctx context.Context) (*Status, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status(ctx)
}

func (p *Portal) status(ctx context.Context) (*Status, error) {
	obj, err := p.call(ctx, "online_list", nil)
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
		if address != p.source {
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

// Login submits one native password authentication without a status preflight.
// A server acceptance is verified against the requested terminal identity.
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
	macType := "0"
	if p.terminal == 2 {
		macType = "1"
	}
	account += suffix
	data := url.Values{
		"enable_r3": {"0"}, "login_method": {"1"},
		"terminal_type": {strconv.Itoa(p.terminal)},
		"user_account":  {"," + macType + "," + account}, "user_password": {password},
		"wlan_user_ip": {p.source}, "wlan_user_ipv6": {""},
	}
	raw, err := p.call(ctx, "login", data)
	if err != nil {
		return unknown("login", err)
	}
	result, err := operationResult(raw, "login")
	if err != nil {
		return result, err
	}
	result.Status, err = p.waitState(ctx, func(state *Status) bool {
		return state.Online && (state.Terminal.Account == account || state.Terminal.Account == ","+macType+","+account)
	})
	if err != nil {
		return result, fmt.Errorf("portal accepted login; online verification failed: %w", err)
	}
	result.Verified = true
	return result, nil
}

func (p *Portal) Logout(ctx context.Context) (*OperationResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	before, err := p.status(ctx)
	if err != nil {
		return nil, err
	}
	if !before.Online {
		return &OperationResult{Outcome: NotSubmitted, Status: before}, fmt.Errorf("terminal is already offline; no logout was submitted")
	}
	if err := p.ensure(ctx); err != nil {
		return nil, err
	}
	data := p.terminalFields()
	for key, value := range map[string]string{"login_method": "1", "user_account": "drcom", "user_password": "123", "ac_logout": scalar(p.settings["ac_logout"]), "register_mode": scalar(p.settings["register_mode"])} {
		data.Set(key, value)
	}
	raw, err := p.call(ctx, "logout", data)
	if err != nil {
		return unknown("logout", err)
	}
	result, err := operationResult(raw, "logout")
	if err != nil {
		return result, err
	}
	result.Status, err = p.waitState(ctx, func(state *Status) bool { return !state.Online })
	if err != nil {
		return result, fmt.Errorf("portal accepted logout; offline verification failed: %w", err)
	}
	result.Verified = true
	return result, nil
}

// Radius accounting propagates after a successful reply. Observe that change;
// the credential submission itself is never repeated.
func (p *Portal) waitState(ctx context.Context, matches func(*Status) bool) (*Status, error) {
	ctx, cancel := context.WithTimeout(ctx, p.stateTimeout)
	defer cancel()
	var last *Status
	for {
		started := time.Now()
		state, err := p.status(ctx)
		if err != nil {
			return last, err
		}
		last = state
		if matches(state) {
			return state, nil
		}
		timer := time.NewTimer(max(0, 50*time.Millisecond-time.Since(started)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, fmt.Errorf("terminal state did not converge within the verification deadline: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// A state query cannot prove acceptance of an unreadable submission response.
// The caller may explicitly observe Status without changing this outcome.
func unknown(operation string, cause error) (*OperationResult, error) {
	return &OperationResult{Outcome: Unknown}, fmt.Errorf("portal %s outcome unknown: %w", operation, cause)
}

func operationResult(obj map[string]json.RawMessage, operation string) (*OperationResult, error) {
	result := &OperationResult{Outcome: Unknown, Message: scalar(obj["msg"]),
		RetCode: append(json.RawMessage(nil), obj["ret_code"]...),
		SelfURL: scalar(obj["self_auth_url"]), OperatorURL: scalar(obj["self_auth_url2"])}
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
	obj, err := p.call(ctx, "err_code", url.Values{"wlan_user_ip": {p.source}, "wlan_user_ipv6": {""}, "wlan_user_mac": {p.mac}})
	if err != nil {
		return nil, err
	}
	result, resultErr := operationResult(obj, "error information")
	return &ErrorInfo{Outcome: result.Outcome, Message: result.Message, Code: scalar(obj["error_code"]), PromptChinese: scalar(obj["error_prompt_zh"]), PromptEnglish: scalar(obj["error_prompt_en"])}, resultErr
}
