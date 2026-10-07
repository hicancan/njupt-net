package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type credential struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type broadbandAccount struct {
	Operator string `json:"operator"`
	Account  string `json:"account"`
	Password string `json:"password"`
}

type config struct {
	Accounts         map[string]credential `json:"accounts"`
	BroadbandAccount broadbandAccount      `json:"broadband_account"`
}

func loadConfig(path string) (config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var cfg *config
	if err := dec.Decode(&cfg); err != nil {
		return config{}, fmt.Errorf("invalid config: %w", err)
	}
	if cfg == nil {
		return config{}, fmt.Errorf("config must contain one JSON object")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return config{}, fmt.Errorf("config must contain one JSON object")
	}
	return *cfg, nil
}

func (c config) identity(alias string) (credential, error) {
	value, ok := c.Accounts[alias]
	if !ok {
		return credential{}, fmt.Errorf("account alias %q is not configured", alias)
	}
	if value.Account == "" {
		return credential{}, fmt.Errorf("account alias %q requires an account", alias)
	}
	return value, nil
}

func (c config) credential(alias string) (credential, error) {
	value, err := c.identity(alias)
	if err != nil {
		return value, err
	}
	if value.Password == "" {
		return credential{}, fmt.Errorf("account alias %q requires account and password", alias)
	}
	return value, nil
}

func configured(opt options) (config, credential, error) {
	cfg, _, err := configuredIdentity(opt)
	if err != nil {
		return cfg, credential{}, err
	}
	value, err := cfg.credential(opt.account)
	return cfg, value, err
}

func configuredIdentity(opt options) (config, credential, error) {
	if opt.account == "" {
		return config{}, credential{}, invalid("--account is required")
	}
	cfg, err := loadConfig(opt.config)
	if err != nil {
		return cfg, credential{}, err
	}
	value, err := cfg.identity(opt.account)
	return cfg, value, err
}
