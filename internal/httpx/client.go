package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// Options configure the transport session in a deterministic way.
type Options struct {
	BaseURL     string
	Timeout     time.Duration
	InsecureTLS bool
	UserAgent   string
}

// SessionClient is the default kernel.SessionClient implementation.
type SessionClient struct {
	baseURL string
	http    *http.Client
}

// NewSessionClient creates a cookie-aware, redirect-inspecting HTTP session.
func NewSessionClient(opts Options) (*SessionClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.InsecureTLS},
		},
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			_ = req
			_ = via
			return http.ErrUseLastResponse
		},
	}

	return &SessionClient{
		baseURL: strings.TrimSpace(opts.BaseURL),
		http:    client,
	}, nil
}

// NewDefaultSessionClient is kept as a compatibility wrapper for older code paths.
func NewDefaultSessionClient(baseURL string) (*SessionClient, error) {
	return NewSessionClient(Options{
		BaseURL:     baseURL,
		Timeout:     30 * time.Second,
		InsecureTLS: true,
	})
}

// ResetCookies drops the current cookie jar so follow-up requests observe a fresh session.
func (c *SessionClient) ResetCookies() error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("create cookie jar: %w", err)
	}
	c.http.Jar = jar
	return nil
}

func (c *SessionClient) Get(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
	reqURL, err := c.buildURL(path, opts.Query)
	if err != nil {
		return nil, fmt.Errorf("build get url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create get request: %w", err)
	}
	applyHeaders(req, opts.Headers)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do get request: %w", err)
	}
	defer resp.Body.Close()

	return adaptResponse(resp)
}

func (c *SessionClient) PostForm(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
	reqURL, err := c.buildURL(path, opts.Query)
	if err != nil {
		return nil, fmt.Errorf("build post url: %w", err)
	}

	form := url.Values{}
	for k, v := range opts.Form {
		form.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create post request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyHeaders(req, opts.Headers)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do post request: %w", err)
	}
	defer resp.Body.Close()

	return adaptResponse(resp)
}

func (c *SessionClient) PostJSON(ctx context.Context, path string, opts kernel.RequestOptions, payload []byte) (*kernel.SessionResponse, error) {
	reqURL, err := c.buildURL(path, opts.Query)
	if err != nil {
		return nil, fmt.Errorf("build post url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create post request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyHeaders(req, opts.Headers)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do post request: %w", err)
	}
	defer resp.Body.Close()

	return adaptResponse(resp)
}

func (c *SessionClient) buildURL(path string, query map[string]string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}

	var target *url.URL
	if parsed, err := url.Parse(path); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		target = parsed
	} else {
		if strings.TrimSpace(c.baseURL) == "" {
			return "", fmt.Errorf("relative path requires a baseURL")
		}
		base, err := url.Parse(c.baseURL)
		if err != nil {
			return "", fmt.Errorf("parse baseURL: %w", err)
		}
		rel, err := url.Parse(path)
		if err != nil {
			return "", fmt.Errorf("parse relative path: %w", err)
		}
		target = base.ResolveReference(rel)
	}

	q := target.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	target.RawQuery = q.Encode()
	return target.String(), nil
}

func applyHeaders(req *http.Request, headers map[string]string) {
	if headers == nil {
		return
	}
	for k, v := range headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
}

func adaptResponse(resp *http.Response) (*kernel.SessionResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	finalURL := ""
	if location := strings.TrimSpace(resp.Header.Get("Location")); location != "" {
		if parsed, err := url.Parse(location); err == nil {
			if resp.Request != nil && resp.Request.URL != nil {
				finalURL = resp.Request.URL.ResolveReference(parsed).String()
			} else {
				finalURL = parsed.String()
			}
		} else {
			finalURL = location
		}
	} else if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	headers := make(map[string][]string, len(resp.Header))
	for k, values := range resp.Header {
		copied := make([]string, len(values))
		copy(copied, values)
		headers[k] = copied
	}

	return &kernel.SessionResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
		FinalURL:   finalURL,
	}, nil
}
