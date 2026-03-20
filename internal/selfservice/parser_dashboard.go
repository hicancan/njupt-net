package selfservice

import (
	"strings"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func parseOnlineSessions(rows []map[string]interface{}) []kernel.OnlineSession {
	sessions := make([]kernel.OnlineSession, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, kernel.OnlineSession{
			BRASID:       kernel.ToString(row["brasid"]),
			IP:           kernel.ToString(row["ip"]),
			LoginTime:    kernel.ToString(row["loginTime"]),
			MAC:          kernel.ToString(row["mac"]),
			SessionID:    kernel.ToString(row["sessionId"]),
			TerminalType: kernel.ToString(row["terminalType"]),
			UpFlow:       kernel.ToString(row["upFlow"]),
			DownFlow:     kernel.ToString(row["downFlow"]),
			UseTime:      kernel.ToString(row["useTime"]),
			UserID:       kernel.ToString(row["userId"]),
		})
	}
	return sessions
}

func parseLoginHistoryEntries(rows [][]interface{}) []kernel.LoginHistoryEntry {
	entries := make([]kernel.LoginHistoryEntry, 0, len(rows))
	for _, row := range rows {
		entry := kernel.LoginHistoryEntry{Raw: row}
		if len(row) > 0 {
			entry.LoginTime = kernel.ToString(row[0])
		}
		if len(row) > 1 {
			entry.LogoutTime = kernel.ToString(row[1])
		}
		if len(row) > 2 {
			entry.IP = kernel.ToString(row[2])
		}
		if len(row) > 3 {
			entry.MAC = kernel.ToString(row[3])
		}
		if len(row) > 9 {
			entry.TerminalFlag = kernel.ToString(row[9])
		}
		if len(row) > 10 {
			entry.TerminalType = kernel.ToString(row[10])
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
