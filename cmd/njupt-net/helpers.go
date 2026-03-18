package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/portal"
	"github.com/hicancan/njupt-net-cli/internal/selfservice"
)

type authFlags struct {
	Profile  string
	Username string
	Password string
}

func bindAuthFlags(cmd *cobra.Command, flags *authFlags) {
	cmd.Flags().StringVar(&flags.Profile, "profile", "", "Configured account profile from credentials.json")
	cmd.Flags().StringVar(&flags.Username, "username", "", "Explicit username override")
	cmd.Flags().StringVar(&flags.Password, "password", "", "Explicit password override")
}

func resolveAccount(cmd *cobra.Command, flags authFlags) (*config.AccountConfig, error) {
	appCtx, err := rootOpts.load(cmd)
	if err != nil {
		return nil, err
	}
	account, err := appCtx.Config.ResolveAccount(flags.Profile, flags.Username, flags.Password)
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func newSelfClient(cmd *cobra.Command) (*selfservice.Client, error) {
	appCtx, err := rootOpts.load(cmd)
	if err != nil {
		return nil, err
	}
	session, err := appCtx.NewSelfSession()
	if err != nil {
		return nil, err
	}
	return selfservice.NewClient(session), nil
}

func newPortalClient(cmd *cobra.Command) (*portal.Client, error) {
	appCtx, err := rootOpts.load(cmd)
	if err != nil {
		return nil, err
	}
	session, err := appCtx.NewPortalSession()
	if err != nil {
		return nil, err
	}
	return portal.NewClient(session, appCtx.Config.Portal.BaseURL, firstFallback(appCtx.Config.Portal.FallbackBaseURLs)), nil
}

func render(cmd *cobra.Command, payload any, human func(io.Writer) error) error {
	appCtx, err := rootOpts.load(cmd)
	if err != nil {
		return err
	}
	if payload == nil {
		return nil
	}
	return appCtx.Renderer.Print(payload, human)
}

func requireYes(cmd *cobra.Command, action string) error {
	appCtx, err := rootOpts.load(cmd)
	if err != nil {
		return err
	}
	return appCtx.MustConfirm(action)
}

func firstFallback(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func printKV(w io.Writer, pairs ...string) error {
	for _, pair := range pairs {
		if _, err := fmt.Fprintln(w, pair); err != nil {
			return err
		}
	}
	return nil
}
