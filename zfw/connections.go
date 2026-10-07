package zfw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

type Connection struct {
	SessionID    string  `json:"session_id"`
	LoginTime    string  `json:"login_time"`
	IP           string  `json:"ip"`
	MAC          string  `json:"mac"`
	UseTime      string  `json:"use_time"`
	UpFlow       string  `json:"up_flow"`
	DownFlow     string  `json:"down_flow"`
	HostName     *string `json:"host_name"`
	TerminalType string  `json:"terminal_type"`
	BRASID       string  `json:"bras_id"`
	UserID       int64   `json:"user_id"`
}

type connectionWire struct {
	SessionID    string  `json:"sessionId"`
	LoginTime    string  `json:"loginTime"`
	IP           string  `json:"ip"`
	MAC          string  `json:"mac"`
	UseTime      string  `json:"useTime"`
	UpFlow       string  `json:"upFlow"`
	DownFlow     string  `json:"downFlow"`
	HostName     *string `json:"hostName"`
	TerminalType string  `json:"terminalType"`
	BRASID       string  `json:"brasid"`
	UserID       int64   `json:"userId"`
}

func (value *connectionWire) UnmarshalJSON(data []byte) error {
	type plain connectionWire
	var decoded plain
	if err := decodeObject(data, &decoded, "sessionId", "loginTime", "ip", "mac", "useTime", "upFlow", "downFlow", "terminalType", "brasid", "userId"); err != nil {
		return err
	}
	*value = connectionWire(decoded)
	return nil
}

type LoginRecord struct {
	LoginTime     json.Number `json:"login_time"`
	LogoutTime    json.Number `json:"logout_time"`
	IP            string      `json:"ip"`
	MAC           string      `json:"mac"`
	Duration      json.Number `json:"duration"`
	Flow          json.Number `json:"flow"`
	BillingMethod json.Number `json:"billing_method"`
	Cost          json.Number `json:"cost"`
	HostName      *string     `json:"host_name"`
	TerminalType  string      `json:"terminal_type"`
	// These two server columns are absent from the rendered table. Their
	// values are preserved without assigning an unverified business meaning.
	ServerField10 string      `json:"server_field_10"`
	ServerField11 json.Number `json:"server_field_11"`
}

type loginRecordWire LoginRecord

func (r *loginRecordWire) UnmarshalJSON(data []byte) error {
	return decodeColumns(data, &r.LoginTime, &r.LogoutTime, &r.IP, &r.MAC, &r.Duration, &r.Flow, &r.BillingMethod, &r.Cost, &r.HostName, &r.TerminalType, &r.ServerField10, &r.ServerField11)
}

func (s *Session) Online(ctx context.Context) ([]Connection, error) {
	var wire []connectionWire
	if err := s.json(ctx, "dashboard/getOnlineList", nil, &wire); err != nil {
		return nil, err
	}
	if wire == nil {
		return nil, fmt.Errorf("Self dashboard/getOnlineList did not return an array")
	}
	connections := make([]Connection, len(wire))
	for index, connection := range wire {
		if connection.SessionID == "" || connection.IP == "" {
			return nil, fmt.Errorf("Self online connection is missing its session ID or IP")
		}
		connections[index] = Connection(connection)
	}
	return connections, nil
}

func (s *Session) History(ctx context.Context) ([]LoginRecord, error) {
	var wire []loginRecordWire
	if err := s.json(ctx, "dashboard/getLoginHistory", nil, &wire); err != nil {
		return nil, err
	}
	if wire == nil {
		return nil, fmt.Errorf("Self dashboard/getLoginHistory did not return an array")
	}
	records := make([]LoginRecord, len(wire))
	for index, record := range wire {
		records[index] = LoginRecord(record)
	}
	return records, nil
}

type OfflineResult struct {
	SessionID string  `json:"session_id"`
	Outcome   Outcome `json:"outcome"`
	Accepted  bool    `json:"accepted"`
	Verified  bool    `json:"verified"`
}

// Offline submits one request and observes the online list until that exact
// session disappears. A failed observation never resubmits the request.
func (s *Session) Offline(ctx context.Context, sessionID string) (*OfflineResult, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("offline requires an online session ID")
	}
	result := &OfflineResult{SessionID: sessionID, Outcome: Unknown}
	var response struct {
		Success *bool `json:"success"`
	}
	if err := s.json(ctx, "dashboard/tooffline", url.Values{"sessionid": {sessionID}}, &response); err != nil {
		return result, fmt.Errorf("terminal offline request failed; result is unknown: %w", err)
	}
	if response.Success == nil {
		return result, fmt.Errorf("Self tooffline response is missing its success flag")
	}
	result.Accepted = *response.Success
	if !result.Accepted {
		result.Outcome = Rejected
		return result, fmt.Errorf("Self rejected the terminal offline request")
	}
	result.Outcome = Accepted
	observation, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		connections, err := s.Online(observation)
		if err != nil {
			return result, fmt.Errorf("terminal offline request was accepted; final state is unverified: %w", err)
		}
		found := false
		for _, connection := range connections {
			if connection.SessionID == sessionID {
				found = true
				break
			}
		}
		if !found {
			result.Verified = true
			return result, nil
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-observation.Done():
			timer.Stop()
			return result, fmt.Errorf("terminal offline request was accepted; session is still listed: %w", observation.Err())
		case <-timer.C:
		}
	}
}
