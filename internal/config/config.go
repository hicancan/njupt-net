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
	defaultConfigName    = "config.json"
)

// AccountConfig resolves profile-based login for Self and Portal.
type AccountConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BroadbandConfig keeps the configured mobile broadband credentials block.
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

// GuardScheduleConfig contains the supported day/night schedule model.
type GuardScheduleConfig struct {
	DayProfile   string `json:"dayProfile"`
	NightProfile string `json:"nightProfile"`
	NightStart   string `json:"nightStart"`
	NightEnd     string `json:"nightEnd"`
}

// GuardConfig contains runtime defaults for the supported Go guard.
type GuardConfig struct {
	StateDir                    string              `json:"stateDir"`
	ProbeIntervalSeconds        int                 `json:"probeIntervalSeconds"`
	BindingCheckIntervalSeconds int                 `json:"bindingCheckIntervalSeconds"`
	Timezone                    string              `json:"timezone"`
	Schedule                    GuardScheduleConfig `json:"schedule"`
}

// Config is the canonical config model for the terminal system.
type Config struct {
	Accounts map[string]AccountConfig `json:"accounts"`
	CMCC     BroadbandConfig          `json:"cmcc"`
	Output   string                   `json:"output"`
	Self     SelfConfig               `json:"self"`
	Portal   PortalConfig             `json:"portal"`
	Guard    GuardConfig              `json:"guard"`
}

// Load resolves config from the given path or the default config.json.
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
	candidates = append(candidates, defaultConfigName)

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
		Message: "no config file found; pass --config or create config.json",
		Err:     kernel.ErrInvalidConfig,
		ProblemDetails: kernel.ConfigProblemDetails{
			Field: "configPath",
			Hint:  "pass --config or create config.json",
		},
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
	if strings.TrimSpace(c.Guard.StateDir) == "" {
		c.Guard.StateDir = filepath.Join("dist", "guard")
	}
	if c.Guard.ProbeIntervalSeconds == 0 {
		c.Guard.ProbeIntervalSeconds = 3
	}
	if c.Guard.BindingCheckIntervalSeconds == 0 {
		c.Guard.BindingCheckIntervalSeconds = 180
	}
	if strings.TrimSpace(c.Guard.Timezone) == "" {
		c.Guard.Timezone = "Asia/Shanghai"
	}
	if strings.TrimSpace(c.Guard.Schedule.DayProfile) == "" {
		c.Guard.Schedule.DayProfile = "B"
	}
	if strings.TrimSpace(c.Guard.Schedule.NightProfile) == "" {
		c.Guard.Schedule.NightProfile = "W"
	}
	if strings.TrimSpace(c.Guard.Schedule.NightStart) == "" {
		c.Guard.Schedule.NightStart = "23:30"
	}
	if strings.TrimSpace(c.Guard.Schedule.NightEnd) == "" {
		c.Guard.Schedule.NightEnd = "07:00"
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
			return &kernel.OpError{Op: "config.validate", Message: fmt.Sprintf("account %q requires username and password", name), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "accounts." + name, Hint: "set both username and password"}}
		}
	}
	if strings.TrimSpace(c.Self.BaseURL) == "" {
		return &kernel.OpError{Op: "config.validate", Message: "self.baseURL is required", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "self.baseURL"}}
	}
	if strings.TrimSpace(c.Portal.BaseURL) == "" {
		return &kernel.OpError{Op: "config.validate", Message: "portal.baseURL is required", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "portal.baseURL"}}
	}
	if c.Guard.ProbeIntervalSeconds <= 0 {
		return &kernel.OpError{Op: "config.validate", Message: "guard.probeIntervalSeconds must be positive", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.probeIntervalSeconds", Value: strconv.Itoa(c.Guard.ProbeIntervalSeconds)}}
	}
	if c.Guard.BindingCheckIntervalSeconds <= 0 {
		return &kernel.OpError{Op: "config.validate", Message: "guard.bindingCheckIntervalSeconds must be positive", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "guard.bindingCheckIntervalSeconds", Value: strconv.Itoa(c.Guard.BindingCheckIntervalSeconds)}}
	}
	return nil
}

// ResolveAccount returns profile-based credentials or explicit overrides.
func (c *Config) ResolveAccount(profile, username, password string) (AccountConfig, error) {
	if strings.TrimSpace(username) != "" || strings.TrimSpace(password) != "" {
		if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
			return AccountConfig{}, &kernel.OpError{Op: "config.resolveAccount", Message: "explicit username/password must be provided together", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "username,password", Hint: "set both explicit values together"}}
		}
		return AccountConfig{Username: username, Password: password}, nil
	}
	if len(c.Accounts) == 0 {
		return AccountConfig{}, &kernel.OpError{Op: "config.resolveAccount", Message: "no configured accounts; provide --username and --password or add accounts to config", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "accounts", Hint: "configure accounts or pass explicit credentials"}}
	}
	if strings.TrimSpace(profile) == "" {
		if len(c.Accounts) == 1 {
			for _, account := range c.Accounts {
				return account, nil
			}
		}
		return AccountConfig{}, &kernel.OpError{Op: "config.resolveAccount", Message: "profile is required when config has multiple accounts", Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "profile", Hint: "choose one configured account profile"}}
	}
	account, ok := c.Accounts[profile]
	if !ok {
		return AccountConfig{}, &kernel.OpError{Op: "config.resolveAccount", Message: fmt.Sprintf("profile %q not found", profile), Err: kernel.ErrInvalidConfig, ProblemDetails: kernel.ConfigProblemDetails{Field: "profile", Value: profile}}
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

// ResolveBroadband returns the configured mobile broadband credentials.
func (c *Config) ResolveBroadband() (BroadbandConfig, error) {
	if strings.TrimSpace(c.CMCC.Account) == "" || strings.TrimSpace(c.CMCC.Password) == "" {
		return BroadbandConfig{}, &kernel.OpError{
			Op:      "config.resolveBroadband",
			Message: "cmcc account and password are required for guard runtime",
			Err:     kernel.ErrInvalidConfig,
			ProblemDetails: kernel.ConfigProblemDetails{
				Field: "cmcc",
				Hint:  "configure cmcc.account and cmcc.password for guard runtime",
			},
		}
	}
	return c.CMCC, nil
}
