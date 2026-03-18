package selfservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

const (
	loginPath              = "/Self/login/?302=LI"
	randomCodePath         = "/Self/login/randomCode"
	verifyPath             = "/Self/login/verify"
	logoutPath             = "/Self/login/logout"
	dashboardPath          = "/Self/dashboard"
	servicePath            = "/Self/service"
	operatorIDPath         = "/Self/service/operatorId"
	bindOperatorPath       = "/Self/service/bind-operator"
	consumeProtectPath     = "/Self/service/consumeProtect"
	changeConsumePath      = "/Self/service/changeConsumeProtect"
	macListPath            = "/Self/service/getMacList"
	personListPath         = "/Self/setting/personList"
	updateUserSecurityPath = "/Self/setting/updateUserSecurity"
)

// Client implements the Self protocol against the abstract transport.
type Client struct {
	session kernel.SessionClient
}

func NewClient(session kernel.SessionClient) *Client {
	return &Client{session: session}
}

func (c *Client) ensureSession(op string) error {
	if c == nil || c.session == nil {
		return &kernel.OpError{Op: op, Message: "session client is nil", Err: kernel.ErrAuth}
	}
	return nil
}

func (c *Client) readDocument(ctx context.Context, path string, opts kernel.RequestOptions, op string) (*goquery.Document, *kernel.SessionResponse, error) {
	resp, err := c.session.Get(ctx, path, opts)
	if err != nil {
		return nil, nil, &kernel.OpError{Op: op, Message: "request failed", Err: err}
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, resp, &kernel.OpError{Op: op, Message: "parse html failed", Err: err}
	}
	return doc, resp, nil
}

func rawCapture(resp *kernel.SessionResponse) *kernel.RawCapture {
	if resp == nil {
		return nil
	}
	return &kernel.RawCapture{
		Status:   resp.StatusCode,
		Headers:  resp.Headers,
		Body:     string(resp.Body),
		FinalURL: resp.FinalURL,
	}
}

func looksLikeLoginPage(body []byte) bool {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return false
	}
	if doc.Find("input[name='checkcode']").Length() > 0 {
		return true
	}
	if doc.Find("input[name='account']").Length() > 0 && doc.Find("input[name='password']").Length() > 0 {
		return true
	}
	if action, ok := doc.Find("form").First().Attr("action"); ok && strings.Contains(strings.ToLower(action), "/self/login") {
		return true
	}
	return false
}

func extractInputValue(doc *goquery.Document, name string) string {
	value, _ := doc.Find("input[name='" + name + "']").First().Attr("value")
	return strings.TrimSpace(value)
}

func extractText(doc *goquery.Document, selectors ...string) string {
	for _, selector := range selectors {
		text := normalizeText(doc.Find(selector).First().Text())
		if text != "" {
			return text
		}
	}
	return ""
}

func normalizeText(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func timestampQuery() map[string]string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	return map[string]string{"t": ts, "_": ts}
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case json.Number:
		return val.String()
	default:
		return strings.TrimSpace(fmt.Sprint(val))
	}
}

func boolFromJSON(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		lowered := strings.ToLower(strings.TrimSpace(val))
		return lowered == "true" || lowered == "1"
	default:
		return false
	}
}

func parseJSON(body []byte, out interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(out)
}
