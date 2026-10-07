// This executable models CLI responses for offline PowerShell workflow tests.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const source = "10.20.30.40"
const mac = "aabbccddeeff"

type binding struct {
	Account     string `json:"account"`
	PasswordSet bool   `json:"password_set"`
}

type call struct {
	Args    []string `json:"args"`
	Command string   `json:"command"`
	Account string   `json:"account"`
	Write   string   `json:"write,omitempty"`
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
	Writes        map[string]int                `json:"writes"`
	OfflineIDs    []string                      `json:"offline_ids"`
	PortalReads   int                           `json:"portal_reads"`
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
	path := os.Getenv("NJUPT_NET_FAKE_STATE")
	if path == "" {
		fmt.Fprintln(os.Stderr, "offline fixture requires a state file")
		os.Exit(2)
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
	args := os.Args[1:]
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
	c := call{Args: append([]string{}, args...), Command: command, Account: alias}
	write := ""
	switch {
	case command == "p login":
		write = alias + ":login"
	case command == "zfw offline":
		write = alias + ":offline"
	case command == "zfw operator" && flags["--unbind"] != "":
		write = alias + ":unbind"
	case command == "zfw operator" && flags["--bind"] == "true":
		write = alias + ":bind"
	}
	c.Write = write
	s.Calls = append(s.Calls, c)
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
	emit := func(result any, message string, exitCode int) {
		save()
		if failed && s.FailureMode == "wrong_envelope" {
			command = "unrelated command"
		}
		out := os.Stdout
		if exitCode != 0 {
			out = os.Stderr
		}
		envelope := map[string]any{"command": command, "data": result}
		if message != "" {
			envelope["error"] = map[string]string{"message": message}
		}
		if err := json.NewEncoder(out).Encode(envelope); err != nil {
			panic(err)
		}
		os.Exit(exitCode)
	}
	if failed && s.FailureMode == "malformed_json" {
		save()
		fmt.Fprintln(os.Stderr, "invalid fixture JSON")
		os.Exit(1)
	}
	if failed && s.FailureMode == "secret_error" {
		emit(nil, "injected error CAMPUS_OLD_PASSWORD_73e6 / CAMPUS_NEW_PASSWORD_82a1 / BROADBAND_PASSWORD_91f4", 1)
	}
	if failed && s.FailureMode != "accepted_unverified" && s.FailureMode != "accepted_unverified_success" && s.FailureMode != "wrong_envelope" {
		emit(nil, "injected CLI failure", 1)
	}
	var cfg config
	if flags["--config"] != "" {
		data, err = os.ReadFile(flags["--config"])
		if err != nil || json.Unmarshal(data, &cfg) != nil {
			emit(nil, "fixture could not load configuration", 1)
		}
	}
	terminal := func(online bool) any {
		if !online {
			return nil
		}
		return map[string]any{"account": s.OnlineAccount, "ip": source, "mac": mac, "session": "portal-session"}
	}
	status := func() any {
		online := s.Online
		if s.Writes["old:offline"] > 0 && !online && (s.Scenario == "offline_pending" || s.Scenario == "offline_delayed" && s.PortalReads <= 2) {
			online = true
		}
		return map[string]any{"source": source, "online": online, "terminal": terminal(online)}
	}
	verified := !(failed && (s.FailureMode == "accepted_unverified" || s.FailureMode == "accepted_unverified_success"))
	finishWrite := func(result any) {
		if !verified && s.FailureMode != "accepted_unverified_success" {
			emit(result, "operation accepted; final state is unverified", 1)
		}
		emit(result, "", 0)
	}
	switch command {
	case "interfaces":
		addresses := []string{source}
		if s.Scenario == "multiple_ipv4" {
			addresses = append(addresses, "10.20.30.41")
		}
		emit([]any{map[string]any{"name": "校园 有线 网络", "index": 12, "up": s.Scenario != "interface_down", "ipv4": addresses}}, "", 0)
	case "p status":
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
					emit(nil, "offline pause was not released", 1)
				}
				time.Sleep(25 * time.Millisecond)
			}
		}
		if s.Writes["old:offline"] > 0 {
			s.PortalReads++
		}
		emit(status(), "", 0)
	case "p login":
		s.Writes[write]++
		s.Online = true
		s.OnlineAccount = cfg.Accounts[alias].Account
		if flags["--operator"] != "campus" {
			s.OnlineAccount += "@" + flags["--operator"]
		}
		finishWrite(map[string]any{"outcome": "accepted", "verified": verified, "status": status()})
	case "probe":
		emit(map[string]any{"source": source, "internet": s.Scenario != "probe_offline", "probe": "http://www.msftconnecttest.com/connecttest.txt"}, "", 0)
	case "zfw verify":
		emit(map[string]any{"credentials_valid": true, "identity_verified": true, "login_method": "password", "account_alias": alias}, "", 0)
	case "zfw online":
		rows := []any{}
		if s.Online || s.Scenario == "offline_self_online" {
			row := func(id, ip, address string) any {
				return map[string]any{"session_id": id, "login_time": "2026-01-01 00:00:00", "ip": ip, "mac": address, "use_time": "1", "up_flow": "1", "down_flow": "1", "host_name": nil, "terminal_type": "PC", "bras_id": "fixture", "user_id": 1}
			}
			switch s.Scenario {
			case "zero_session":
			case "multiple_sessions":
				rows = append(rows, row("self-session", source, mac), row("self-second", source, mac))
			case "wrong_mac":
				rows = append(rows, row("self-session", source, "112233445566"))
			default:
				rows = append(rows, row("self-session", source, "AA:BB:CC:DD:EE:FF"), row("other-device", "10.20.30.41", "112233445566"))
			}
		}
		emit(rows, "", 0)
	case "zfw offline":
		if flags["--session"] != "self-session" {
			emit(nil, "offline used a portal session or another terminal", 1)
		}
		s.Writes[write]++
		s.OfflineIDs = append(s.OfflineIDs, flags["--session"])
		s.Online = false
		if s.Scenario == "target_changes_after_offline" {
			s.Bindings["new"][s.Provider] = binding{Account: "concurrent-target-binding", PasswordSet: true}
		}
		finishWrite(map[string]any{"session_id": "self-session", "outcome": "accepted", "accepted": true, "verified": verified})
	case "zfw operator":
		if flags["--unbind"] == "" && flags["--bind"] == "" {
			emit(s.Bindings[alias], "", 0)
		}
		provider := flags["--unbind"]
		account := ""
		if flags["--bind"] == "true" {
			provider = cfg.Broadband.Operator
			account = cfg.Broadband.Account
		}
		if provider != s.Provider {
			emit(nil, "workflow attempted to migrate another provider", 1)
		}
		s.Writes[write]++
		s.Bindings[alias][provider] = binding{Account: account, PasswordSet: account != ""}
		finishWrite(map[string]any{"operator": provider, "account": account, "outcome": "accepted", "verified": verified, "message": "", "bindings": s.Bindings[alias]})
	default:
		emit(nil, "unsupported fixture command", 1)
	}
}
