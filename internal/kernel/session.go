package kernel

import "context"

// RequestOptions normalizes transport-layer inputs across Self and Portal.
type RequestOptions struct {
	Headers map[string]string
	Query   map[string]string
	Form    map[string]string
}

// SessionResponse is the transport-normalized response envelope.
type SessionResponse struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       []byte              `json:"-"`
	FinalURL   string              `json:"finalURL,omitempty"`
}

// SessionClient is the transport contract consumed by protocol packages.
type SessionClient interface {
	Get(ctx context.Context, path string, opts RequestOptions) (*SessionResponse, error)
	PostForm(ctx context.Context, path string, opts RequestOptions) (*SessionResponse, error)
}
