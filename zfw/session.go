package zfw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hicancan/njupt-net/v3/network"
	"golang.org/x/net/html"
)

var ErrSessionExpired = errors.New("zfw management session is not authenticated")

// Outcome describes a submitted operation independently from its final-state
// verification. A lost response remains unknown and is never resubmitted.
type Outcome string

const (
	Accepted     Outcome = "accepted"
	Rejected     Outcome = "rejected"
	Unknown      Outcome = "unknown"
	NotSubmitted Outcome = "not_submitted"
)

// Session owns one management session and is used sequentially. It does not own
// the terminal's Internet session. Operation-specific tokens remain local.
type Session struct {
	client         *http.Client
	base           *url.URL
	identity       string
	authenticated  bool
	loginSubmitted bool
}

func New(link *network.Link) *Session {
	base, _ := url.Parse("http://zfw.njupt.edu.cn:8080/Self/")
	return &Session{client: link.Client(), base: base}
}
func (s *Session) request(ctx context.Context, method, path string, values url.Values) ([]byte, *url.URL, http.Header, error) {
	reference, err := url.Parse(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid Self endpoint")
	}
	u := s.base.ResolveReference(reference)
	if u.Scheme != s.base.Scheme || u.Host != s.base.Host || !strings.HasPrefix(u.Path, s.base.Path) || u.User != nil {
		return nil, nil, nil, fmt.Errorf("endpoint is outside Self")
	}
	return network.Request(ctx, s.client, method, u.String(), values)
}

func (s *Session) page(ctx context.Context, path string) (*html.Node, error) {
	if s.identity == "" {
		return nil, ErrSessionExpired
	}
	data, final, _, err := s.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return s.authenticatedPage(data, final)
}

func (s *Session) authenticatedPage(data []byte, final *url.URL) (*html.Node, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("Session returned an empty HTML response")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("Session returned non-UTF-8 HTML")
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse Session page: %w", err)
	}
	if selfLoginPath(final) || loginForm(doc) != nil {
		s.identity, s.authenticated, s.loginSubmitted = "", false, false
		return nil, ErrSessionExpired
	}
	return doc, nil
}

func (s *Session) json(ctx context.Context, path string, params url.Values, out any) error {
	if s.identity == "" {
		return ErrSessionExpired
	}
	query := cloneValues(params)
	query.Set("t", selfNonce())
	data, final, _, err := s.request(ctx, http.MethodGet, path, query)
	if err != nil {
		return err
	}
	if selfLoginPath(final) {
		s.identity, s.authenticated, s.loginSubmitted = "", false, false
		return ErrSessionExpired
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("Session %s returned an empty JSON response", path)
	}
	if trimmed[0] == '<' {
		doc, parseErr := html.Parse(bytes.NewReader(data))
		if parseErr == nil && loginForm(doc) != nil {
			s.identity, s.authenticated, s.loginSubmitted = "", false, false
			return ErrSessionExpired
		}
		return fmt.Errorf("Session %s returned HTML instead of JSON", path)
	}
	if !utf8.Valid(data) || !json.Valid(data) {
		return fmt.Errorf("Session %s returned invalid JSON", path)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("Session %s JSON does not match its contract: %w", path, err)
	}
	return nil
}

// Object contracts remain private to this site's protocol.
func decodeObject(data []byte, out any, required ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return fmt.Errorf("expected a JSON object")
	}
	for _, name := range required {
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("required field %s is absent or null", name)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	return nil
}

// The site's positional tables have fixed, observed column contracts.
func decodeColumns(data []byte, targets ...any) error {
	var columns []json.RawMessage
	if err := json.Unmarshal(data, &columns); err != nil || len(columns) != len(targets) {
		return fmt.Errorf("expected a row with %d columns", len(targets))
	}
	for index, target := range targets {
		if bytes.Equal(bytes.TrimSpace(columns[index]), []byte("null")) {
			switch target.(type) {
			case *string, *json.Number, *int, *int64:
				return fmt.Errorf("column %d must not be null", index)
			}
		}
		if _, numeric := target.(*json.Number); numeric && len(bytes.TrimSpace(columns[index])) > 0 && bytes.TrimSpace(columns[index])[0] == '"' {
			return fmt.Errorf("column %d must be a JSON number", index)
		}
		if err := json.Unmarshal(columns[index], target); err != nil {
			return fmt.Errorf("column %d has an unexpected type: %w", index, err)
		}
	}
	return nil
}

func (s *Session) Login(ctx context.Context, account, password string) (err error) {
	if account == "" || password == "" {
		return fmt.Errorf("Session login requires account and password")
	}
	if s.client == nil || s.client.Jar == nil {
		return fmt.Errorf("Session login requires a client with an independent CookieJar")
	}
	if s.authenticated || s.loginSubmitted {
		return fmt.Errorf("Self management session is already authenticated")
	}
	defer s.closeRejectedLogin(&err)
	s.identity = ""
	data, final, _, err := s.request(ctx, http.MethodGet, "login", nil)
	if err != nil {
		return err
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse Session login page: %w", err)
	}
	form := loginForm(doc)
	if form == nil {
		return fmt.Errorf("Session login page does not contain its verify form")
	}
	values := formValues(form)
	for _, name := range []string{"foo", "bar", "checkcode", "account", "password", "code"} {
		if _, exists := values[name]; !exists {
			return fmt.Errorf("Session login form is missing %s", name)
		}
	}
	if values.Get("checkcode") == "" {
		return fmt.Errorf("Session login form has an empty checkcode")
	}
	if !regexp.MustCompile(`\bvar\s+md5\s*=\s*false\s*;`).MatchString(scriptText(doc)) {
		return fmt.Errorf("Session password submission contract changed")
	}
	captcha := find(doc, func(n *html.Node) bool { return attr(n, "id") == "randomDiv" })
	if captcha == nil || !hasClass(captcha, "hide") {
		return fmt.Errorf("Session normal password login is unavailable: captcha is enabled")
	}
	image, _, headers, err := s.request(ctx, http.MethodGet, "login/randomCode", url.Values{"t": {selfNonce()}})
	if err != nil {
		return err
	}
	if !strings.HasPrefix(headers.Get("Content-Type"), "image/png") || !bytes.HasPrefix(image, []byte("\x89PNG\r\n\x1a\n")) {
		return fmt.Errorf("Session login/randomCode did not return the expected PNG")
	}
	values.Set("account", account)
	values.Set("password", password)
	values.Del("submit")
	action, err := final.Parse(attr(form, "action"))
	if err != nil {
		return fmt.Errorf("invalid Session login form action")
	}
	s.loginSubmitted = true
	data, final, _, err = s.request(ctx, http.MethodPost, action.String(), values)
	if err != nil {
		return err
	}
	return s.confirmLogin(data, final, account)
}

// LoginBridge consumes the caller-selected, short-lived p self-service URL.
// Its actual Self origin owns the Cookie session used by subsequent operations.
// Password authentication is never attempted when this entry fails.
func (s *Session) LoginBridge(ctx context.Context, expectedAccount, bridgeURL string) (err error) {
	if expectedAccount == "" {
		return fmt.Errorf("Self bridge login requires the expected account")
	}
	if s.client == nil || s.client.Jar == nil {
		return fmt.Errorf("Self bridge login requires an independent CookieJar")
	}
	if s.authenticated || s.loginSubmitted {
		return fmt.Errorf("Self management session is already authenticated")
	}
	target, err := parseBridgeURL(bridgeURL)
	if err != nil {
		return err
	}
	defer s.closeRejectedLogin(&err)
	s.identity = ""
	s.base = &url.URL{Scheme: target.Scheme, Host: target.Host, Path: "/Self/"}
	s.loginSubmitted = true
	data, final, _, err := s.request(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return err
	}
	return s.confirmLogin(data, final, expectedAccount)
}

func ValidateBridgeURL(raw string) error {
	_, err := parseBridgeURL(raw)
	return err
}

func parseBridgeURL(raw string) (*url.URL, error) {
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != "http" || (target.Host != "zfw.njupt.edu.cn:8080" && target.Host != "10.10.244.240:8080") || target.User != nil || target.Path != "/Self/login/eportalLogin" || target.Fragment != "" {
		return nil, fmt.Errorf("Self bridge URL must use a confirmed campus HTTP 8080 origin and the eportalLogin entry")
	}
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil || len(query) != 3 {
		return nil, fmt.Errorf("Self bridge URL requires exactly params, timestamp and sign")
	}
	for _, name := range []string{"params", "timestamp", "sign"} {
		if len(query[name]) != 1 || query.Get(name) == "" {
			return nil, fmt.Errorf("Self bridge URL requires one nonempty %s", name)
		}
	}
	return target, nil
}

func (s *Session) confirmLogin(data []byte, final *url.URL, account string) error {
	if final == nil || final.Scheme != s.base.Scheme || final.Host != s.base.Host || pathWithoutSession(final.Path) != "/Self/dashboard" {
		return fmt.Errorf("Session login was rejected; no authenticated dashboard was returned")
	}
	s.authenticated = true
	doc, err := s.authenticatedPage(data, final)
	if err != nil {
		return err
	}
	user, err := selfUser(doc)
	if err != nil {
		s.identity = ""
		return err
	}
	if user.Account != account {
		s.identity = ""
		return fmt.Errorf("Session authenticated a different account")
	}
	s.identity = account
	s.loginSubmitted = false
	return nil
}

func (s *Session) closeRejectedLogin(operationErr *error) {
	if *operationErr == nil || (!s.authenticated && !s.loginSubmitted) {
		return
	}
	if !s.authenticated && !s.hasManagementCookie() {
		s.loginSubmitted = false
		return
	}
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if closeErr := s.Logout(cleanup); closeErr != nil {
		*operationErr = errors.Join(*operationErr, fmt.Errorf("close rejected management session: %w", closeErr))
	}
}

func (s *Session) Logout(ctx context.Context) error {
	if !s.authenticated && !s.loginSubmitted {
		return ErrSessionExpired
	}
	data, final, _, err := s.request(ctx, http.MethodGet, "login/logout", nil)
	if err != nil {
		return err
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil || !selfLoginPath(final) || loginForm(doc) == nil {
		return fmt.Errorf("Session logout did not return the login page")
	}
	s.identity = ""
	s.authenticated = false
	s.loginSubmitted = false
	return nil
}

func selfLoginPath(u *url.URL) bool {
	if u == nil {
		return false
	}
	p := strings.TrimSuffix(pathWithoutSession(u.Path), "/")
	return p == "/Self/login" || p == "/Self/login/verify"
}

func pathWithoutSession(path string) string {
	return regexp.MustCompile(`;jsessionid=[^/;?]*`).ReplaceAllString(path, "")
}

func loginForm(doc *html.Node) *html.Node {
	return find(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "form" && pathWithoutSession(attr(n, "action")) == "/Self/login/verify"
	})
}

// Language changes this Cookie session's page preference. Account overview
// parsing follows the verified Chinese page contract; this does not change a
// persistent account preference or an external browser's language.
func (s *Session) Language(ctx context.Context, language string) error {
	if language != "zh_cn" && language != "English" {
		return fmt.Errorf("language must be zh_cn or English")
	}
	_, final, _, err := s.request(ctx, http.MethodGet, "login/changeLanguage?=t"+selfNonce(), url.Values{"language": {language}})
	if err != nil {
		return err
	}
	if selfLoginPath(final) && s.identity != "" {
		s.identity, s.authenticated, s.loginSubmitted = "", false, false
		return ErrSessionExpired
	}
	data, _, _, err := s.request(ctx, http.MethodGet, "login", nil)
	if err != nil {
		return err
	}
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("parse language result: %w", err)
	}
	button := find(doc, func(n *html.Node) bool { return attr(n, "id") == "language" })
	want := "English"
	if language == "English" {
		want = "中文"
	}
	if nodeText(button) != want {
		return fmt.Errorf("Self did not apply the requested language")
	}
	return nil
}
