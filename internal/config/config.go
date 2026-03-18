package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

const (
	defaultSelfBaseURL   = "http://10.10.244.240:8080"
	defaultPortalBaseURL = "https://10.10.244.11:802/eportal/portal"
)

// AccountConfig resolves profile-based login for Self and Portal.
type AccountConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BroadbandConfig keeps the current experimental broadband credentials block.
type BroadbandConfig struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

// SelfConfig contains Self transport defaults.
type SelfConfig struct {
	BaseURL        string `json:"baseURL"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

// PortalConfig contains Portal transport defaults.
type PortalConfig struct {
	BaseURL          string   `json:"baseURL"`
	FallbackBaseURLs []string `json:"fallbackBaseURLs,omitempty"`
	ISP              string   `json:"isp"`
	TimeoutSeconds   int      `json:"timeoutSeconds"`
	InsecureTLS      bool     `json:"insecureTLS"`
}

// Config is the canonical config model for the terminal system.
type Config struct {
	Accounts map[string]AccountConfig `json:"accounts"`
	CMCC     BroadbandConfig          `json:"cmcc"`
	Output   string                   `json:"output"`
	Self     SelfConfig               `json:"self"`
	Portal   PortalConfig             `json:"portal"`
}

// Load resolves config from the given path or the default credentials.json.
func Load(path string) (*Config, error) {
	cfgPath, err := resolvePath(path)
	if err != nil {
		return nil, err
	}

	payload, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, fmt.Errorf("parse config json: %w", err)
	}

	cfg.applyDefaults()
	cfg.applyEnv()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func resolvePath(path string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(path) != "" {
		candidates = append(candidates, path)
	}
	if env := strings.TrimSpace(os.Getenv("NJUPT_NET_CONFIG")); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, "credentials.json")

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		resolved, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(resolved); err == nil {
			return resolved, nil
		}
	}

	return "", &kernel.OpError{
		Op:      "config.load",
		Message: "no config file found; pass --config or create credentials.json",
		Err:     kernel.ErrInvalidConfig,
	}
}

func (c *Config) applyDefaults() {
	if c.Output == "" {
		c.Output = "human"
	}
	if c.Self.BaseURL == "" {
		c.Self.BaseURL = defaultSelfBaseURL
	}
	if c.Self.TimeoutSeconds == 0 {
		c.Self.TimeoutSeconds = 10
	}
	if c.Portal.BaseURL == "" {
		c.Portal.BaseURL = defaultPortalBaseURL
	}
	if c.Portal.TimeoutSeconds == 0 {
		c.Portal.TimeoutSeconds = 8
	}
	if c.Portal.ISP == "" {
		c.Portal.ISP = "mobile"
	}
	if len(c.Portal.FallbackBaseURLs) == 0 {
		c.Portal.FallbackBaseURLs = []string{"https://p.njupt.edu.cn:802/eportal/portal"}
	}
}

func (c *Config) applyEnv() {
	if v := strings.TrimSpace(os.Getenv("NJUPT_NET_OUTPUT")); v != "" {
		c.Output = v
	}
	if v := strings.TrimSpace(os.Getenv("NJUPT_NET_SELF_BASE_URL")); v != "" {
		c.Self.BaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("NJUPT_NET_PORTAL_BASE_URL")); v != "" {
		c.Portal.BaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("NJUPT_NET_PORTAL_ISP")); v != "" {
		c.Portal.ISP = v
	}
	if v := strings.TrimSpace(os.Getenv("NJUPT_NET_INSECURE_TLS")); v != "" {
		c.Portal.InsecureTLS = parseBool(v, c.Portal.InsecureTLS)
	}
	if v := strings.TrimSpace(os.Getenv("NJUPT_NET_SELF_TIMEOUT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Self.TimeoutSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("NJUPT_NET_PORTAL_TIMEOUT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Portal.TimeoutSeconds = n
		}
	}
}

// Validate ensures the runtime has enough information to operate deterministically.
func (c *Config) Validate() error {
	for name, account := range c.Accounts {
		if strings.TrimSpace(account.Username) == "" || strings.TrimSpace(account.Password) == "" {
			return &kernel.OpError{Op: "config.validate", Message: fmt.Sprintf("account %q requires username and password", name), Err: kernel.ErrInvalidConfig}
		}
	}
	if strings.TrimSpace(c.Self.BaseURL) == "" {
		return &kernel.OpError{Op: "config.validate", Message: "self.baseURL is required", Err: kernel.ErrInvalidConfig}
	}
	if strings.TrimSpace(c.Portal.BaseURL) == "" {
		return &kernel.OpError{Op: "config.validate", Message: "portal.baseURL is required", Err: kernel.ErrInvalidConfig}
	}
	return nil
}

// ResolveAccount returns profile-based credentials or explicit overrides.
func (c *Config) ResolveAccount(profile, username, password string) (AccountConfig, error) {
	if strings.TrimSpace(username) != "" || strings.TrimSpace(password) != "" {
		if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
			return AccountConfig{}, &kernel.OpError{Op: "config.resolveAccount", Message: "explicit username/password must be provided together", Err: kernel.ErrInvalidConfig}
		}
		return AccountConfig{Username: username, Password: password}, nil
	}
	if len(c.Accounts) == 0 {
		return AccountConfig{}, &kernel.OpError{Op: "config.resolveAccount", Message: "no configured accounts; provide --username and --password or add accounts to config", Err: kernel.ErrInvalidConfig}
	}
	if strings.TrimSpace(profile) == "" {
		if len(c.Accounts) == 1 {
			for _, account := range c.Accounts {
				return account, nil
			}
		}
		return AccountConfig{}, &kernel.OpError{Op: "config.resolveAccount", Message: "profile is required when config has multiple accounts", Err: kernel.ErrInvalidConfig}
	}
	account, ok := c.Accounts[profile]
	if !ok {
		return AccountConfig{}, &kernel.OpError{Op: "config.resolveAccount", Message: fmt.Sprintf("profile %q not found", profile), Err: kernel.ErrInvalidConfig}
	}
	return account, nil
}

func parseBool(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
