package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/hicancan/njupt-net/v4/network"
	"github.com/hicancan/njupt-net/v4/p"
	"github.com/hicancan/njupt-net/v4/zfw"
)

// commandContext owns the link and website sessions for one CLI invocation.
// Commands select operations; this object only owns their shared lifetimes.
type commandContext struct {
	opt      options
	stream   bool
	link     *network.Link
	cfg      *config
	portals  map[portalKey]*p.Portal
	sessions map[string]*zfw.Session
	public   *zfw.Session
	order    []string
	closed   bool
}

type portalKey struct {
	terminal string
	port     int
}

func newCommandContext(opt options) *commandContext {
	return &commandContext{opt: opt, portals: make(map[portalKey]*p.Portal), sessions: make(map[string]*zfw.Session)}
}

func (c *commandContext) openLink() (*network.Link, error) {
	if c.closed {
		return nil, fmt.Errorf("command context is closed")
	}
	if c.link == nil {
		link, err := openLink(c.opt)
		if err != nil {
			return nil, err
		}
		c.link = link
	}
	return c.link, nil
}

func (c *commandContext) configuration() (config, error) {
	if c.cfg == nil {
		cfg, err := loadConfig(c.opt.config)
		if err != nil {
			return config{}, err
		}
		c.cfg = &cfg
	}
	return *c.cfg, nil
}

func (c *commandContext) configured(alias string, password bool) (config, credential, error) {
	if alias == "" {
		return config{}, credential{}, invalid("--account is required")
	}
	cfg, err := c.configuration()
	if err != nil {
		return cfg, credential{}, err
	}
	var value credential
	if password {
		value, err = cfg.credential(alias)
	} else {
		value, err = cfg.identity(alias)
	}
	return cfg, value, err
}

func (c *commandContext) portal(terminal string, port int) (*p.Portal, error) {
	key := portalKey{terminal: terminal, port: port}
	if portal := c.portals[key]; portal != nil {
		return portal, nil
	}
	link, err := c.openLink()
	if err != nil {
		return nil, err
	}
	portal, err := p.NewAt(link, terminal, port)
	if err != nil {
		return nil, err
	}
	c.portals[key] = portal
	return portal, nil
}

func (c *commandContext) self(ctx context.Context, alias string, value credential, bridgeURL string) (*zfw.Session, error) {
	if session := c.sessions[alias]; session != nil {
		return session, nil
	}
	link, err := c.openLink()
	if err != nil {
		return nil, err
	}
	session := zfw.New(link)
	if bridgeURL == "" {
		err = session.Login(ctx, value.Account, value.Password)
	} else {
		err = session.LoginBridge(ctx, value.Account, bridgeURL)
	}
	if err != nil {
		return nil, err
	}
	c.sessions[alias] = session
	c.order = append(c.order, alias)
	return session, nil
}

func (c *commandContext) forgetExpired(alias string, err error) {
	if errors.Is(err, zfw.ErrSessionExpired) {
		delete(c.sessions, alias)
	}
}

func (c *commandContext) close(ctx context.Context) error {
	if c.closed {
		return nil
	}
	c.closed = true
	var result error
	for index := len(c.order) - 1; index >= 0; index-- {
		alias := c.order[index]
		session := c.sessions[alias]
		if session == nil {
			continue
		}
		delete(c.sessions, alias)
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.opt.timeout)
		err := session.Logout(closeCtx)
		cancel()
		if err != nil {
			result = errors.Join(result, fmt.Errorf("close zfw management session %q: %w", alias, err))
		}
	}
	if c.link != nil {
		c.link.Close()
	}
	return result
}
