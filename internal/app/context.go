package app

import (
	"fmt"
	"io"
	"time"

	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/httpx"
	"github.com/hicancan/njupt-net-cli/internal/kernel"
	"github.com/hicancan/njupt-net-cli/internal/output"
)

// Options are the root CLI/application settings.
type Options struct {
	ConfigPath  string
	OutputMode  string
	AssumeYes   bool
	InsecureTLS bool
}

// Context is the explicit outer-layer runtime container.
type Context struct {
	Config      *config.Config
	Renderer    *output.Renderer
	AssumeYes   bool
	InsecureTLS bool
}

// Load builds the explicit app context from config and CLI flags.
func Load(opts Options, out io.Writer) (*Context, error) {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	renderer, err := output.NewRenderer(out, chooseOutputMode(opts.OutputMode, cfg.Output))
	if err != nil {
		return nil, err
	}

	return &Context{
		Config:      cfg,
		Renderer:    renderer,
		AssumeYes:   opts.AssumeYes,
		InsecureTLS: opts.InsecureTLS || cfg.Portal.InsecureTLS,
	}, nil
}

// NewSelfSession returns a fresh session client for Self flows.
func (c *Context) NewSelfSession() (kernel.SessionClient, error) {
	return httpx.NewSessionClient(httpx.Options{
		BaseURL:     c.Config.Self.BaseURL,
		Timeout:     time.Duration(c.Config.Self.TimeoutSeconds) * time.Second,
		InsecureTLS: c.InsecureTLS,
	})
}

// NewPortalSession returns a fresh session client for Portal flows.
func (c *Context) NewPortalSession() (kernel.SessionClient, error) {
	return httpx.NewSessionClient(httpx.Options{
		BaseURL:     c.Config.Portal.BaseURL,
		Timeout:     time.Duration(c.Config.Portal.TimeoutSeconds) * time.Second,
		InsecureTLS: c.InsecureTLS,
	})
}

// MustConfirm returns an error when a side-effecting command requires --yes.
func (c *Context) MustConfirm(action string) error {
	if c.AssumeYes {
		return nil
	}
	return &kernel.OpError{
		Op:      "app.confirm",
		Message: fmt.Sprintf("%s requires --yes", action),
		Err:     kernel.ErrGuardedCapability,
	}
}

func chooseOutputMode(flagValue, configValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return configValue
}
