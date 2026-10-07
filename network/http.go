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

const UserAgent = "njupt-net/3"

func safeEndpointPath(path string) string {
	return regexp.MustCompile(`(?i);jsessionid=[^/;?]*`).ReplaceAllString(path, ";jsessionid=<session>")
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

// Request executes one HTTP GET or form POST. It never includes a query string or a request body in an error.
func Request(ctx context.Context, client *http.Client, method, rawURL string, values url.Values) ([]byte, *url.URL, http.Header, error) {
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
	resp, err := client.Do(req)
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
	if response != nil {
		return errors.New("HTTP redirect rejected")
	}
	return err
}
