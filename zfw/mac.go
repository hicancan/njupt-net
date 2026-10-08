package zfw

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type MACBinding struct {
	Online        int     `json:"online"`
	MAC           string  `json:"mac"`
	TerminalType  *string `json:"terminal_type"`
	LastLoginTime *string `json:"last_login_time"`
	LastIP        *string `json:"last_ip"`
}

type macBindingWire MACBinding

func (b *macBindingWire) UnmarshalJSON(data []byte) error {
	var online string
	if err := decodeColumns(data, &online, &b.MAC, &b.TerminalType, &b.LastLoginTime, &b.LastIP); err != nil {
		return err
	}
	switch online {
	case "0":
		b.Online = 0
	case "1":
		b.Online = 1
	default:
		return fmt.Errorf("Self MAC binding online column must be the string 0 or 1")
	}
	return nil
}

type MACPage struct {
	Total int          `json:"total"`
	Rows  []MACBinding `json:"rows"`
}

func (s *Session) Devices(ctx context.Context, page, size int) (*MACPage, error) {
	if page < 1 || (size != 10 && size != 25 && size != 50 && size != 100) {
		return nil, fmt.Errorf("device page must be positive and size must be 10, 25, 50 or 100")
	}
	return s.macList(ctx, page, size)
}

func (s *Session) macList(ctx context.Context, page, size int) (*MACPage, error) {
	var data struct {
		Total *int             `json:"total"`
		Rows  []macBindingWire `json:"rows"`
	}
	if err := s.json(ctx, "service/getMacList", url.Values{
		"pageNumber": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(size)},
		"searchText": {""}, "sortName": {"2"}, "sortOrder": {"DESC"},
	}, &data); err != nil {
		return nil, err
	}
	if data.Total == nil || *data.Total < 0 || data.Rows == nil || *data.Total < len(data.Rows) || len(data.Rows) > size {
		return nil, fmt.Errorf("Self getMacList response requires total and rows")
	}
	rows := make([]MACBinding, len(data.Rows))
	for index, row := range data.Rows {
		if err := ValidateMAC(row.MAC); err != nil {
			return nil, fmt.Errorf("Self MAC binding row has an invalid MAC identifier")
		}
		rows[index] = MACBinding(row)
	}
	return &MACPage{Total: *data.Total, Rows: rows}, nil
}

type UnbindResult struct {
	MAC      string  `json:"mac"`
	Message  string  `json:"message"`
	Outcome  Outcome `json:"outcome"`
	Verified bool    `json:"verified"`
}

var unbindTokenPattern = regexp.MustCompile(`"&ajaxCsrfToken="\s*\+\s*'([^']+)'`)
var macPattern = regexp.MustCompile(`^[0-9a-fA-F]{12}$`)

func ValidateMAC(mac string) error {
	if !macPattern.MatchString(mac) {
		return fmt.Errorf("unbind MAC must be exactly twelve hexadecimal digits")
	}
	return nil
}

func (s *Session) Unbind(ctx context.Context, mac string) (*UnbindResult, error) {
	if err := ValidateMAC(mac); err != nil {
		return nil, err
	}
	result := &UnbindResult{MAC: mac, Outcome: NotSubmitted}
	doc, err := s.page(ctx, "service/myMac")
	if err != nil {
		return result, err
	}
	token := unbindTokenPattern.FindStringSubmatch(scriptText(doc))
	if len(token) != 2 || token[1] == "" {
		return result, fmt.Errorf("Self device page is missing its unbind CSRF token")
	}
	present, err := s.hasMACBinding(ctx, mac)
	if err != nil {
		return result, fmt.Errorf("MAC unbind precondition failed; no request was submitted: %w", err)
	}
	if !present {
		result.Verified = true
		return result, nil
	}
	result.Outcome = Unknown
	data, final, _, err := s.navigate(ctx, http.MethodGet, "service/unbindmac", url.Values{"mac": {mac}, "ajaxCsrfToken": {token[1]}}, "/Self/service/myMac")
	if err != nil {
		return result, fmt.Errorf("MAC unbind request failed; result is unknown: %w", err)
	}
	doc, err = s.authenticatedPage(data, final)
	if err != nil {
		return result, err
	}
	message, state, err := formFeedback(doc)
	if err != nil || message == "" {
		return result, fmt.Errorf("Self unbind did not return its operation message")
	}
	result.Message = message
	switch state {
	case "true":
		result.Outcome = Accepted
	case "false":
		result.Outcome = Rejected
		return result, fmt.Errorf("Self rejected the MAC unbind submission")
	default:
		return result, fmt.Errorf("Self MAC unbind result has no recognized acceptance state")
	}
	present, err = s.hasMACBinding(ctx, mac)
	if err != nil {
		return result, fmt.Errorf("MAC unbind was submitted; binding removal is unverified: %w", err)
	}
	result.Verified = !present
	if present {
		return result, fmt.Errorf("Self accepted the unbind submission but the MAC binding is still listed")
	}
	return result, nil
}

// Account bindings can span pages. Absence is established only after every
// expected row has been read, with a consistent total throughout the query.
func (s *Session) hasMACBinding(ctx context.Context, mac string) (bool, error) {
	const size = 100
	total := -1
	seen := map[string]bool{}
	for page, read := 1, 0; ; page++ {
		bindings, err := s.macList(ctx, page, size)
		if err != nil {
			return false, err
		}
		if total == -1 {
			total = bindings.Total
		} else if total != bindings.Total {
			return false, fmt.Errorf("Self MAC binding total changed during pagination")
		}
		expected := min(size, total-read)
		if len(bindings.Rows) != expected {
			return false, fmt.Errorf("Self MAC binding page does not contain its expected rows")
		}
		for _, binding := range bindings.Rows {
			key := strings.ToLower(binding.MAC)
			if seen[key] {
				return false, fmt.Errorf("Self MAC binding pages repeat the same MAC identifier")
			}
			seen[key] = true
			if strings.EqualFold(binding.MAC, mac) {
				return true, nil
			}
		}
		read += len(bindings.Rows)
		if read == total {
			return false, nil
		}
	}
}
