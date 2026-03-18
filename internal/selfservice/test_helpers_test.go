package selfservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

type mockSessionClient struct {
	getFn      func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error)
	postFormFn func(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error)
}

func (m *mockSessionClient) Get(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
	if m.getFn != nil {
		return m.getFn(ctx, path, opts)
	}
	return nil, errors.New("mock get not implemented")
}

func (m *mockSessionClient) PostForm(ctx context.Context, path string, opts kernel.RequestOptions) (*kernel.SessionResponse, error) {
	if m.postFormFn != nil {
		return m.postFormFn(ctx, path, opts)
	}
	return nil, errors.New("mock post not implemented")
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return payload
}
