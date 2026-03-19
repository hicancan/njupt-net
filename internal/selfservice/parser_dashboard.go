package selfservice

import (
	"strings"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func parseOnlineSessions(rows []map[string]interface{}) []kernel.OnlineSession {
	sessions := make([]kernel.OnlineSession, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, kernel.OnlineSession{
			BRASID:       toString(row["brasid"]),
			IP:           toString(row["ip"]),
			LoginTime:    toString(row["loginTime"]),
			MAC:          toString(row["mac"]),
			SessionID:    toString(row["sessionId"]),
			TerminalType: toString(row["terminalType"]),
			UpFlow:       toString(row["upFlow"]),
			DownFlow:     toString(row["downFlow"]),
			UseTime:      toString(row["useTime"]),
			UserID:       toString(row["userId"]),
		})
	}
	return sessions
}

func parseLoginHistoryEntries(rows [][]interface{}) []kernel.LoginHistoryEntry {
	entries := make([]kernel.LoginHistoryEntry, 0, len(rows))
	for _, row := range rows {
		entry := kernel.LoginHistoryEntry{Raw: row}
		if len(row) > 0 {
			entry.LoginTime = toString(row[0])
		}
		if len(row) > 1 {
			entry.LogoutTime = toString(row[1])
		}
		if len(row) > 2 {
			entry.IP = toString(row[2])
		}
		if len(row) > 3 {
			entry.MAC = toString(row[3])
		}
		if len(row) > 9 {
			entry.TerminalFlag = toString(row[9])
		}
		if len(row) > 10 {
			entry.TerminalType = toString(row[10])
		}
		entries = append(entries, entry)
	}
	return entries
}

func parseMauthState(body string) kernel.MauthState {
	switch {
	case strings.Contains(body, "默认"):
		return kernel.MauthOn
	case strings.Contains(body, "关闭"):
		return kernel.MauthOff
	default:
		return kernel.MauthUnknown
	}
}
