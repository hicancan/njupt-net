// This executable models CLI responses for offline PowerShell workflow tests.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const source = "10.20.30.40"
const mac = "aabbccddeeff"

var fixtureStartedAt = time.Now()

type binding struct {
	Account     string `json:"account"`
	PasswordSet bool   `json:"password_set"`
}
type call struct {
	Args       []string `json:"args"`
	Command    string   `json:"command"`
	Account    string   `json:"account"`
	Executable string   `json:"executable"`
	At         int64    `json:"at"`
	ReceivedNS int64    `json:"received_ns"`
	ReturnedNS int64    `json:"returned_ns"`
	Write      string   `json:"write,omitempty"`
}
type state struct {
	Scenario      string                        `json:"scenario"`
	FailAt        int                           `json:"fail_at"`
	FailureMode   string                        `json:"failure_mode"`
	Calls         []call                        `json:"calls"`
	Online        bool                          `json:"online"`
	OnlineAccount string                        `json:"online_account"`
	Provider      string                        `json:"provider"`
	Bindings      map[string]map[string]binding `json:"bindings"`
	Residual      map[string]bool               `json:"residual"`
	Writes        map[string]int                `json:"writes"`
	Reads         map[string]int                `json:"reads"`
	OfflineIDs    []string                      `json:"offline_ids"`
	PortalReads   int                           `json:"portal_reads"`
	SessionStarts int                           `json:"session_starts"`
	SessionCloses int                           `json:"session_closes"`
	SessionArgs   []string                      `json:"session_args"`
	BoundAt       int64                         `json:"bound_at"`
	LoginDelayMS  int                           `json:"login_delay_ms"`
}
type config struct {
	Accounts map[string]struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	} `json:"accounts"`
	Broadband struct {
		Operator string `json:"operator"`
		Account  string `json:"account"`
		Password string `json:"password"`
	} `json:"broadband_account"`
}

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[len(args)-1] == "session" {
		os.Exit(runSession(args[:len(args)-1]))
	}
	os.Exit(runCommand(args, false))
}

func sessionState(change func(*state)) state {
	path := os.Getenv("NJUPT_NET_FAKE_STATE")
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		panic(err)
	}
	change(&s)
	data, err = json.MarshalIndent(s, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		panic(err)
	}
	return s
}

func runSession(globalArgs []string) int {
	s := sessionState(func(s *state) {
		s.SessionStarts++
		s.SessionArgs = append([]string{}, globalArgs...)
	})
	switch s.Scenario {
	case "session_start_error", "session_start_secret_error":
		message := "cannot select the initial campus interface"
		if s.Scenario == "session_start_secret_error" {
			message += ": BROADBAND_PASSWORD_91f4"
		}
		json.NewEncoder(os.Stderr).Encode(map[string]any{"command": "session", "data": nil, "error": map[string]string{"message": message}})
		return 2
	case "session_start_malformed":
		fmt.Fprintln(os.Stderr, "invalid session startup response")
		return 2
	case "session_start_eof":
		return 2
	}
	// The native session fixes its source when it opens the selected link.
	globalArgs = append([]string{}, globalArgs...)
	for index := 0; index+1 < len(globalArgs); index += 2 {
		if globalArgs[index] == "--interface" {
			globalArgs[index], globalArgs[index+1] = "--source", source
		}
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			Account string   `json:"account"`
			Args    []string `json:"args"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			panic(err)
		}
		if len(request.Args) == 1 && request.Args[0] == "close" {
			s := sessionState(func(s *state) { s.SessionCloses++ })
			envelope := map[string]any{"command": "close", "data": map[string]bool{"closed": true}, "exit_code": 0}
			if s.Scenario == "close_error" {
				envelope["data"], envelope["exit_code"] = nil, 1
				envelope["error"] = map[string]string{"message": "management session logout failed"}
			}
			if s.Scenario == "close_wrong_envelope" {
				envelope["command"] = "unrelated command"
			}
			if s.Scenario != "close_eof" {
				if err := json.NewEncoder(os.Stdout).Encode(envelope); err != nil {
					panic(err)
				}
				if s.Scenario == "close_extra_output" {
					fmt.Fprintln(os.Stdout, strings.Repeat("unexpected output ", 131072))
				}
			}
			return envelope["exit_code"].(int)
		}
		args := append([]string{}, globalArgs...)
		if request.Account != "" {
			args = append(args, "--account", request.Account)
		}
		args = append(args, request.Args...)
		runCommand(args, true)
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	return 0
}

func runCommand(args []string, session bool) int {
	path := os.Getenv("NJUPT_NET_FAKE_STATE")
	if path == "" {
		fmt.Fprintln(os.Stderr, "offline fixture requires a state file")
		return 2
	}
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		panic(err)
	}
	if s.Writes == nil {
		s.Writes = map[string]int{}
	}
	if s.Reads == nil {
		s.Reads = map[string]int{}
	}
	flags := map[string]string{}
	position := 0
	for position < len(args) && strings.HasPrefix(args[position], "--") {
		if position+1 >= len(args) {
			panic("incomplete global flag")
		}
		flags[args[position]] = args[position+1]
		position += 2
	}
	if position == len(args) {
		panic("missing fixture command")
	}
	command := args[position]
	position++
	if command == "p" || command == "zfw" {
		if position == len(args) {
			panic("missing fixture subcommand")
		}
		command += " " + args[position]
		position++
	}
	for position < len(args) {
		flag := args[position]
		position++
		if flag == "--bind" {
			flags[flag] = "true"
			continue
		}
		if position == len(args) {
			panic("incomplete fixture command flag")
		}
		flags[flag] = args[position]
		position++
	}
	alias := flags["--account"]
	program, err := os.Executable()
	if err != nil {
		panic(err)
	}
	entry := call{Args: append([]string{}, args...), Command: command, Account: alias, Executable: program, At: time.Now().UnixMilli(), ReceivedNS: time.Since(fixtureStartedAt).Nanoseconds()}
	switch {
	case command == "p login":
		entry.Write = alias + ":login"
	case command == "zfw offline":
		entry.Write = alias + ":offline"
	case command == "zfw operator" && flags["--unbind"] != "":
		entry.Write = alias + ":unbind"
	case command == "zfw operator" && flags["--bind"] != "":
		entry.Write = alias + ":bind"
	}
	s.Calls = append(s.Calls, entry)
	failed := s.FailAt > 0 && len(s.Calls) == s.FailAt
	save := func() {
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			panic(err)
		}
	}
	emit := func(result any, message string, exitCode int) int {
		s.Calls[len(s.Calls)-1].ReturnedNS = time.Since(fixtureStartedAt).Nanoseconds()
		save()
		if failed && s.FailureMode == "wrong_envelope" {
			command = "unrelated command"
		}
		out := os.Stdout
		if exitCode != 0 && !session {
			out = os.Stderr
		}
		envelope := map[string]any{"command": command, "data": result}
		if session {
			envelope["exit_code"] = exitCode
		}
		if message != "" {
			envelope["error"] = map[string]string{"message": message}
		}
		if err := json.NewEncoder(out).Encode(envelope); err != nil {
			panic(err)
		}
		return exitCode
	}
	if strings.HasPrefix(command, "p ") {
		switch flags["--port"] {
		case "801", "802", "803", "804":
		default:
			return emit(nil, "fixture requires an explicit supported portal port", 2)
		}
	} else if flags["--port"] != "" {
		return emit(nil, "portal port was passed to a non-portal command", 2)
	}
	if failed && s.FailureMode == "malformed_json" {
		save()
		out := os.Stderr
		if session {
			out = os.Stdout
		}
		fmt.Fprintln(out, "invalid fixture JSON")
		return 1
	}
	if failed && s.FailureMode == "secret_error" {
		return emit(nil, "injected error CAMPUS_OLD_PASSWORD_73e6 / CAMPUS_NEW_PASSWORD_82a1 / BROADBAND_PASSWORD_91f4", 1)
	}
	if failed && s.FailureMode != "accepted_unverified" && s.FailureMode != "accepted_unverified_success" && s.FailureMode != "unknown_mutation" && s.FailureMode != "wrong_envelope" {
		return emit(nil, "injected CLI failure", 1)
	}
	var cfg config
	if flags["--config"] != "" {
		data, err := os.ReadFile(flags["--config"])
		if err != nil || json.Unmarshal(data, &cfg) != nil {
			return emit(nil, "fixture could not load configuration", 1)
		}
	}
	if strings.HasPrefix(command, "zfw ") {
		credential, exists := cfg.Accounts[alias]
		if !exists || credential.Account == "" || credential.Password == "" {
			return emit(nil, "configured campus account and password are required", 1)
		}
	}
	baseAccount := func() string {
		value := strings.TrimPrefix(strings.TrimPrefix(s.OnlineAccount, ",0,"), ",1,")
		base, _, _ := strings.Cut(value, "@")
		return base
	}
	wasDisconnected := func() bool {
		for key, count := range s.Writes {
			if strings.HasSuffix(key, ":offline") && count > 0 {
				return true
			}
		}
		return false
	}
	status := func() map[string]any {
		online := s.Online
		if wasDisconnected() && !online && (s.Scenario == "offline_pending" || s.Scenario == "offline_delayed" && s.PortalReads <= 2) {
			online = true
		}
		var terminal any
		if online {
			address := source
			account := s.OnlineAccount
			terminalMAC := mac
			if s.Scenario == "before_offline_identity_changes" && s.Reads["p status"] >= 2 && !wasDisconnected() {
				account = "unrelated-campus@" + s.Provider
			}
			if s.Scenario == "before_offline_mac_changes" && s.Reads["p status"] >= 2 && !wasDisconnected() || s.Scenario == "login_mac_changes" && s.Writes["new:login"] > 0 {
				terminalMAC = "112233445566"
			}
			if s.Scenario == "binding_changes_portal_mac" && s.Writes["new:bind"] > 0 {
				terminalMAC = "112233445566"
			}
			if s.Scenario == "wrong_status_ip" {
				address = "10.20.30.99"
			}
			terminal = map[string]any{"account": account, "ip": address, "mac": terminalMAC, "session": "portal-session"}
		}
		return map[string]any{"source": source, "online": online, "terminal": terminal}
	}
	finishWrite := func(result map[string]any) int {
		if failed && s.FailureMode == "unknown_mutation" {
			result["outcome"], result["verified"] = "unknown", false
			return emit(result, "submission outcome is unknown", 1)
		}
		if failed && (s.FailureMode == "accepted_unverified" || s.FailureMode == "accepted_unverified_success") {
			result["verified"] = false
			if s.FailureMode != "accepted_unverified_success" {
				return emit(result, "operation accepted; final state is unverified", 1)
			}
		}
		return emit(result, "", 0)
	}
	switch command {
	case "interfaces":
		addresses := []string{source}
		if s.Scenario == "source_changes_after_session_start" {
			addresses = []string{"10.20.30.41"}
		}
		if s.Scenario == "multiple_ipv4" {
			addresses = append(addresses, "10.20.30.41")
		}
		rows := []any{map[string]any{"name": "校园 有线 网络", "index": 12, "up": s.Scenario != "interface_down", "ipv4": addresses}, map[string]any{"name": "independent WiFi", "index": 13, "up": true, "ipv4": []string{"192.168.43.123"}}}
		if s.Scenario == "duplicate_source" {
			rows = append(rows, map[string]any{"name": "duplicate", "index": 14, "up": true, "ipv4": []string{source}})
		}
		return emit(rows, "", 0)
	case "p status":
		s.Reads["p status"]++
		if s.Scenario == "pause_status" {
			save()
			if err := os.WriteFile(path+".ready", []byte("ready"), 0600); err != nil {
				panic(err)
			}
			deadline := time.Now().Add(20 * time.Second)
			for {
				if _, err := os.Stat(path + ".release"); err == nil {
					break
				}
				if time.Now().After(deadline) {
					return emit(nil, "offline pause was not released", 1)
				}
				time.Sleep(25 * time.Millisecond)
			}
		}
		if wasDisconnected() {
			s.PortalReads++
		}
		return emit(status(), "", 0)
	case "p login":
		s.Writes[entry.Write]++
		if s.Online {
			return emit(map[string]any{"outcome": "rejected", "verified": false, "ret_code": 2, "message": "终端IP已经在线"}, "portal rejected login: 终端IP已经在线", 1)
		}
		if s.Writes[entry.Write] == 1 && s.LoginDelayMS > 0 {
			time.Sleep(time.Duration(s.LoginDelayMS) * time.Millisecond)
		}
		const bindingMessage = "未绑定运营商账号,请正确绑定运营商账号再试！"
		loginFailure := func(outcome, message string, exitCode int) int {
			return emit(map[string]any{"outcome": outcome, "verified": false, "message": message}, "portal rejected login: "+message, exitCode)
		}
		switch s.Scenario {
		case "login_password_rejected":
			return loginFailure("rejected", "账号或密码错误", 1)
		case "login_binding_message_partial":
			return loginFailure("rejected", "未绑定运营商账号", 1)
		case "login_binding_unknown":
			return loginFailure("unknown", bindingMessage, 1)
		case "login_binding_unverified":
			return loginFailure("accepted", bindingMessage, 1)
		case "login_binding_exit2":
			return loginFailure("rejected", bindingMessage, 2)
		case "login_network_error":
			return emit(nil, "authentication connection timed out", 1)
		}
		operator := flags["--operator"]
		if operator == "cmcc" || operator == "njxy" {
			bound := s.Bindings[alias][operator]
			pending := s.Scenario == "binding_activation_delayed" && time.Now().UnixMilli()-s.BoundAt < 1000
			if bound.Account != cfg.Broadband.Account || !bound.PasswordSet || s.Scenario == "portal_binding_unsynchronized" || pending {
				return loginFailure("rejected", bindingMessage, 1)
			}
		}
		s.Online = true
		s.OnlineAccount = cfg.Accounts[alias].Account
		if operator != "" && operator != "campus" {
			s.OnlineAccount += "@" + operator
		}
		if s.Scenario == "login_wrong_identity" {
			s.OnlineAccount = "unrelated-campus@" + s.Provider
		}
		result := map[string]any{"outcome": "accepted", "verified": true, "status": status()}
		if s.Scenario == "login_missing_status" {
			delete(result, "status")
		}
		return finishWrite(result)
	case "probe":
		result := map[string]any{"source": source, "internet": s.Scenario != "probe_offline", "probe": "http://www.msftconnecttest.com/connecttest.txt"}
		switch s.Scenario {
		case "probe_dns_error":
			return emit(nil, "lookup connectivity host: DNS server returned failure", 1)
		case "probe_http_error":
			return emit(nil, "connectivity HTTP request timed out", 1)
		case "probe_secret_error":
			return emit(nil, "connectivity error CAMPUS_OLD_PASSWORD_73e6 / CAMPUS_NEW_PASSWORD_82a1 / BROADBAND_PASSWORD_91f4", 1)
		case "probe_error_data_offline":
			result["internet"] = false
			return emit(result, "connectivity HTTP response indicated offline", 1)
		case "probe_wrong_source_success":
			result["source"] = "10.20.30.99"
		case "probe_wrong_source_error":
			result["source"], result["internet"] = "10.20.30.99", false
			return emit(result, "connectivity request failed", 1)
		case "probe_invalid_data":
			result["internet"] = "false"
		case "probe_missing_error":
			return emit(nil, "", 1)
		case "probe_empty_error":
			return emit(nil, " ", 1)
		case "probe_usage_error":
			return emit(nil, "invalid probe arguments", 2)
		case "probe_mixed_streams":
			fmt.Fprintln(os.Stdout, "unexpected output")
			return emit(nil, "connectivity request failed", 1)
		case "probe_conflicting_result":
			return emit(result, "connectivity request failed", 1)
		}
		return emit(result, "", 0)
	case "zfw online":
		rows := []any{map[string]any{"session_id": "another-device-" + alias, "ip": "10.20.30.41", "mac": "112233445566"}}
		if s.Online && cfg.Accounts[alias].Account == baseAccount() || s.Residual[alias] {
			row := func(id, address string) any {
				return map[string]any{"session_id": id, "ip": source, "mac": address, "login_time": "2026-01-01 00:00:00", "use_time": "1", "up_flow": "1", "down_flow": "1", "host_name": nil, "terminal_type": "PC", "bras_id": "fixture", "user_id": 1}
			}
			switch s.Scenario {
			case "zero_session":
			case "multiple_sessions":
				rows = append(rows, row("self-session-"+alias, mac), row("duplicate-session", mac))
			case "wrong_mac":
				rows = append(rows, row("self-session-"+alias, "112233445566"))
			default:
				rows = append(rows, row("self-session-"+alias, "AA:BB:CC:DD:EE:FF"))
			}
		}
		return emit(rows, "", 0)
	case "zfw offline":
		if flags["--session"] != "self-session-"+alias || cfg.Accounts[alias].Account != baseAccount() {
			return emit(nil, "offline used another account or a portal session", 1)
		}
		s.Writes[entry.Write]++
		s.OfflineIDs = append(s.OfflineIDs, flags["--session"])
		s.Online = false
		if s.Scenario == "target_changes_after_offline" {
			s.Bindings["new"][s.Provider] = binding{Account: "concurrent-target-binding", PasswordSet: true}
		}
		if s.Scenario == "holder_changes_after_offline" {
			s.Bindings["old"][s.Provider] = binding{Account: "different-holder-binding", PasswordSet: true}
		}
		return finishWrite(map[string]any{"session_id": flags["--session"], "outcome": "accepted", "accepted": true, "verified": true})
	case "zfw operator":
		if entry.Write == "" {
			s.Reads[alias+":operator"]++
			if alias == "new" && s.Reads[alias+":operator"] == 2 && (s.Scenario == "target_changes_before_offline_empty" || s.Scenario == "target_changes_before_offline_ready" || s.Scenario == "target_changes_before_bind") {
				s.Bindings[alias][s.Provider] = binding{Account: "concurrent-target-binding", PasswordSet: true}
			}
			if alias == "old" && s.Reads[alias+":operator"] == 2 && s.Scenario == "holder_changes_before_offline" {
				s.Bindings[alias][s.Provider] = binding{Account: "different-holder-binding", PasswordSet: true}
			}
			return emit(s.Bindings[alias], "", 0)
		}
		provider, account := flags["--unbind"], ""
		if flags["--bind"] == "true" {
			provider, account = cfg.Broadband.Operator, cfg.Broadband.Account
		}
		if provider != s.Provider {
			return emit(nil, "workflow attempted to migrate another provider", 1)
		}
		s.Writes[entry.Write]++
		s.Bindings[alias][provider] = binding{Account: account, PasswordSet: account != ""}
		if flags["--bind"] == "true" {
			s.BoundAt = time.Now().UnixMilli()
			switch s.Scenario {
			case "binding_disconnects_terminal":
				s.Online = false
			case "binding_changes_portal_identity":
				s.OnlineAccount = "unrelated-campus@" + s.Provider
			}
		}
		return finishWrite(map[string]any{"operator": provider, "account": account, "outcome": "accepted", "verified": true, "message": "", "bindings": s.Bindings[alias]})
	default:
		return emit(nil, "unsupported fixture command", 1)
	}
}
