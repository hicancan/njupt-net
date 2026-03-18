package selfservice

import (
	"context"
	"fmt"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// RawGet executes a diagnostic GET against the configured Self base URL.
func (c *Client) RawGet(ctx context.Context, path string) (*kernel.OperationResult[map[string]any], error) {
	if err := c.ensureSession("raw.get"); err != nil {
		return nil, err
	}
	resp, err := c.session.Get(ctx, path, kernel.RequestOptions{})
	if err != nil {
		return nil, &kernel.OpError{Op: "raw.get", Message: "request failed", Err: err}
	}
	data := map[string]any{
		"method":   "GET",
		"path":     path,
		"status":   resp.StatusCode,
		"finalURL": resp.FinalURL,
	}
	return &kernel.OperationResult[map[string]any]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("raw GET completed for %s", path),
		Data:    &data,
		Raw:     rawCapture(resp),
	}, nil
}

// RawPost executes a diagnostic form POST against the configured Self base URL.
func (c *Client) RawPost(ctx context.Context, path string, form map[string]string) (*kernel.OperationResult[map[string]any], error) {
	if err := c.ensureSession("raw.post"); err != nil {
		return nil, err
	}
	resp, err := c.session.PostForm(ctx, path, kernel.RequestOptions{Form: form})
	if err != nil {
		return nil, &kernel.OpError{Op: "raw.post", Message: "request failed", Err: err}
	}
	data := map[string]any{
		"method":   "POST",
		"path":     path,
		"status":   resp.StatusCode,
		"finalURL": resp.FinalURL,
	}
	return &kernel.OperationResult[map[string]any]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: fmt.Sprintf("raw POST completed for %s", path),
		Data:    &data,
		Raw:     rawCapture(resp),
	}, nil
}
