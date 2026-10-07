package zfw

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// Account preserves the school's displayed values and their units.
type Account struct {
	Account           string `json:"account"`
	Status            string `json:"status"`
	Plan              string `json:"plan"`
	BillingMethod     string `json:"billing_method"`
	BillingCycle      string `json:"billing_cycle"`
	Balance           string `json:"balance"`
	UsedTime          string `json:"used_time"`
	AvailableTime     string `json:"available_time"`
	ConsumeProtection string `json:"consume_protection"`
}

type DisplayField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Profile is a read-only collection of the fields displayed by this deployment.
type Profile struct {
	Fields   []DisplayField `json:"fields"`
	ReadOnly bool           `json:"read_only"`
}

func (s *Session) Overview(ctx context.Context) (*Account, error) {
	doc, err := s.page(ctx, "dashboard")
	if err != nil {
		return nil, err
	}
	cards := find(doc, func(n *html.Node) bool { return hasClass(n, "user-info1") })
	if cards == nil {
		return nil, fmt.Errorf("Self dashboard is missing its account cards")
	}
	panel := cards.Parent
	for panel != nil && !hasClass(panel, "panel-body") {
		panel = panel.Parent
	}
	if panel == nil {
		return nil, fmt.Errorf("Self dashboard account panel is missing")
	}
	fields := labelValues(panel)
	walk(cards, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "dl" {
			return
		}
		label := find(n, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "dd" })
		value := find(n, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "dt" })
		if label != nil && value != nil {
			fields = append(fields, DisplayField{fieldLabel(nodeText(label)), nodeText(value)})
		}
	})
	account := new(Account)
	targets := map[string]*string{
		"账号": &account.Account, "状态": &account.Status, "套餐": &account.Plan,
		"计费方式": &account.BillingMethod, "计费周期": &account.BillingCycle,
		"账户余额": &account.Balance, "已用时长": &account.UsedTime,
		"可用时长": &account.AvailableTime, "消费保护": &account.ConsumeProtection,
	}
	seen := make(map[string]bool, len(targets))
	for _, field := range fields {
		if target, exists := targets[field.Label]; exists {
			if seen[field.Label] {
				return nil, fmt.Errorf("Self dashboard repeats account field %s", field.Label)
			}
			*target, seen[field.Label] = field.Value, true
		}
	}
	for label := range targets {
		if !seen[label] {
			return nil, fmt.Errorf("Self dashboard is missing account field %s", label)
		}
	}
	user, err := selfUser(doc)
	if err != nil {
		return nil, err
	}
	if account.Account != user.Account || account.Account != s.identity {
		return nil, fmt.Errorf("Self dashboard account does not match the management session")
	}
	return account, nil
}

func (s *Session) Profile(ctx context.Context) (*Profile, error) {
	doc, err := s.page(ctx, "setting/personList")
	if err != nil {
		return nil, err
	}
	section := find(doc, func(n *html.Node) bool { return attr(n, "id") == "userInfo" })
	if section == nil {
		return nil, fmt.Errorf("Self profile is missing its userInfo section")
	}
	fields := labelValues(section)
	if len(fields) == 0 {
		return nil, fmt.Errorf("Self profile has no displayed business fields")
	}
	form := findForm(section, "/Self/setting/updateUserSecurity")
	if form == nil {
		return nil, fmt.Errorf("Self profile is missing its user-security form")
	}
	editable := find(form, func(n *html.Node) bool {
		if n.Type != html.ElementNode || attr(n, "name") == "" || hasAttr(n, "disabled") || hasAttr(n, "readonly") {
			return false
		}
		if n.Data == "select" || n.Data == "textarea" {
			return true
		}
		if n.Data != "input" {
			return false
		}
		switch strings.ToLower(attr(n, "type")) {
		case "hidden", "submit", "button", "reset", "image":
			return false
		}
		return true
	})
	if editable != nil {
		return nil, fmt.Errorf("Self profile now exposes editable fields; its update contract requires examination")
	}
	return &Profile{Fields: fields, ReadOnly: true}, nil
}

var refreshTokenPattern = regexp.MustCompile(`\$\.get\("/Self/dashboard/refreshaccount",\s*\{\s*csrftoken:\s*'([^']+)'`)

func (s *Session) RefreshAccount(ctx context.Context) error {
	doc, err := s.page(ctx, "dashboard")
	if err != nil {
		return err
	}
	token := refreshTokenPattern.FindStringSubmatch(scriptText(doc))
	if len(token) != 2 || token[1] == "" {
		return fmt.Errorf("Self dashboard is missing its refreshaccount CSRF token")
	}
	data, final, _, err := s.request(ctx, http.MethodGet, "dashboard/refreshaccount", url.Values{"csrftoken": {token[1]}, "t": {selfNonce()}})
	if err != nil {
		return err
	}
	if selfLoginPath(final) {
		s.identity, s.authenticated, s.loginSubmitted = "", false, false
		return ErrSessionExpired
	}
	if strings.TrimSpace(string(data)) != "" {
		return fmt.Errorf("Self refreshaccount returned an unexpected nonempty response")
	}
	return nil
}

func nextElement(n *html.Node) *html.Node {
	for n = n.NextSibling; n != nil; n = n.NextSibling {
		if n.Type == html.ElementNode {
			return n
		}
	}
	return nil
}

func fieldLabel(text string) string {
	return strings.TrimRight(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text), ":：")
}

func labelValues(doc *html.Node) []DisplayField {
	fields := []DisplayField{}
	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "label" || attr(n, "for") != "" {
			return
		}
		label := fieldLabel(nodeText(n))
		if label == "" {
			return
		}
		value := nextElement(n)
		if value == nil && n.Parent != nil && n.Parent.Data == "div" {
			value = nextElement(n.Parent)
		}
		if value != nil && value.Data != "label" {
			fields = append(fields, DisplayField{Label: label, Value: nodeText(value)})
		}
	})
	return fields
}
