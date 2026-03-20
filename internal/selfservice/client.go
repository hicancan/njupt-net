package selfservice

import (
	"bytes"
	"context"
	"strconv"
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

func timestampQuery() map[string]string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	return map[string]string{"t": ts, "_": ts}
}
