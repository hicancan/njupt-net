package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const UserAgent = "njupt-net/4"

var sessionPathPattern = regexp.MustCompile(`(?i);jsessionid=[^/;?]*`)

func safeEndpointPath(path string) string {
	return sessionPathPattern.ReplaceAllString(path, ";jsessionid=<session>")
}

// HTTPStatusError preserves the observed status without including credentials
// from a query string, form body or session-qualified path.
type HTTPStatusError struct {
	Method     string
	Path       string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.StatusCode)
}

type readRequestKey struct{}

// RedirectError records a rejected destination without putting its query or
// session-bearing path in the error text.
type RedirectError struct {
	destination url.URL
}

func (e *RedirectError) Error() string { return "HTTP redirect rejected" }

func (e *RedirectError) Destination() *url.URL {
	u := e.destination
	return &u
}

// Read executes an observed read-only GET without following redirects.
// Link clients may reuse its connection.
func Read(ctx context.Context, client *http.Client, rawURL string, values url.Values) ([]byte, *url.URL, http.Header, error) {
	return request(context.WithValue(ctx, readRequestKey{}, true), client, http.MethodGet, rawURL, values, nil)
}

// Request executes one HTTP GET or form POST over Link's single-use connection
// transport. It rejects redirects, including POST-to-GET conversions.
func Request(ctx context.Context, client *http.Client, method, rawURL string, values url.Values) ([]byte, *url.URL, http.Header, error) {
	return request(context.WithValue(ctx, readRequestKey{}, false), client, method, rawURL, values, nil)
}

// Navigate submits once over Link's single-use transport, then follows only
// caller-confirmed read-only GET destinations. The initial entry is never a
// navigation target, and 307/308 never replay the submission.
func Navigate(ctx context.Context, client *http.Client, method, rawURL string, values url.Values, readOnly func(*url.URL) bool) ([]byte, *url.URL, http.Header, error) {
	return request(context.WithValue(ctx, readRequestKey{}, false), client, method, rawURL, values, readOnly)
}

func request(ctx context.Context, client *http.Client, method, rawURL string, values url.Values, readOnly func(*url.URL) bool) ([]byte, *url.URL, http.Header, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid endpoint URL")
	}
	var body io.Reader
	if method == http.MethodGet {
		q := u.Query()
		for key, vals := range values {
			q[key] = vals
		}
		u.RawQuery = q.Encode()
	} else {
		body = strings.NewReader(values.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create %s request: %w", method, err)
	}
	req.Header.Set("User-Agent", UserAgent)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	// Redirect policy belongs to the operation, not a mutable shared client.
	// Keep the client's transport, CookieJar, timeout and additional restrictions.
	operationClient := *client
	operationClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if client.CheckRedirect != nil {
			if err := client.CheckRedirect(next, via); err != nil {
				return err
			}
		}
		if err := checkRedirect(next, via); err != nil {
			return err
		}
		if readOnly == nil || next.Method != http.MethodGet || next.URL.Path == u.Path || !readOnly(next.URL) {
			return &RedirectError{destination: *next.URL}
		}
		*next = *next.WithContext(context.WithValue(next.Context(), readRequestKey{}, true))
		return nil
	}
	resp, err := operationClient.Do(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s %s: %w", method, safeEndpointPath(u.Path), safeClientError(err, resp))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024+1))
	if err != nil {
		return nil, resp.Request.URL, resp.Header, fmt.Errorf("read %s: %w", safeEndpointPath(u.Path), err)
	}
	if len(data) > 32*1024*1024 {
		return nil, resp.Request.URL, resp.Header, fmt.Errorf("%s response exceeds 32 MiB", safeEndpointPath(u.Path))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.Request.URL, resp.Header, &HTTPStatusError{Method: method, Path: safeEndpointPath(u.Path), StatusCode: resp.StatusCode}
	}
	return data, resp.Request.URL, resp.Header, nil
}

func safeClientError(err error, response *http.Response) error {
	for {
		wrapped, ok := err.(*url.Error)
		if !ok {
			break
		}
		err = wrapped.Err
	}
	// net/http formats a malformed Location verbatim into this untyped error.
	// Removing url.Error alone does not remove that signed redirect URL.
	if strings.HasPrefix(err.Error(), "failed to parse Location header ") {
		return errors.New("invalid HTTP redirect Location")
	}
	// Client.Do returns a response together with an error when CheckRedirect
	// rejects navigation. Its callback may also quote the complete target URL.
	var redirect *RedirectError
	if errors.As(err, &redirect) {
		return redirect
	}
	if response != nil {
		return errors.New("HTTP redirect rejected")
	}
	return err
}
