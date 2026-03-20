package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hicancan/njupt-net-cli/internal/app"
	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/kernel"
	"github.com/hicancan/njupt-net-cli/internal/portal"
	"github.com/hicancan/njupt-net-cli/internal/selfservice"
)

type authFlags struct {
	Profile  string
	Username string
	Password string
}

func bindAuthFlags(cmd *cobra.Command, flags *authFlags) {
	cmd.Flags().StringVar(&flags.Profile, "profile", "", "Configured account profile from config.json")
	cmd.Flags().StringVar(&flags.Username, "username", "", "Explicit username override")
	cmd.Flags().StringVar(&flags.Password, "password", "", "Explicit password override")
}

func appContext(cmd *cobra.Command) (*app.Context, error) {
	env, err := getCommandEnv(cmd)
	if err != nil {
		return nil, err
	}
	return env.appContext(cmd, cmd.OutOrStdout())
}

func resolveAccount(cmd *cobra.Command, flags authFlags) (*config.AccountConfig, error) {
	appCtx, err := appContext(cmd)
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
	appCtx, err := appContext(cmd)
	if err != nil {
		return nil, err
	}
	return appCtx.NewSelfClient()
}

func newPortalClient(cmd *cobra.Command) (*portal.Client, error) {
	appCtx, err := appContext(cmd)
	if err != nil {
		return nil, err
	}
	return appCtx.NewPortalClient()
}

func render(cmd *cobra.Command, payload any, human func(io.Writer) error) error {
	env, err := getCommandEnv(cmd)
	if err != nil {
		return err
	}
	if payload == nil {
		return nil
	}
	renderer, err := env.rendererFor(cmd, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	return renderer.Print(payload, human)
}

func requireYes(cmd *cobra.Command, action string) error {
	opts, err := currentRootOptions(cmd)
	if err != nil {
		return err
	}
	if opts.AssumeYes {
		return nil
	}
	return (&app.Context{AssumeYes: false}).MustConfirm(action)
}

func renderOperation[T any](cmd *cobra.Command, result *kernel.OperationResult[T], opErr error, human func(io.Writer) error) error {
	if result != nil {
		result.Problems = kernel.MergeProblems(result.Problems, opErr)
	}
	if err := render(cmd, result, human); err != nil {
		return err
	}
	return opErr
}

func printKV(w io.Writer, pairs ...string) error {
	for _, pair := range pairs {
		if _, err := fmt.Fprintln(w, pair); err != nil {
			return err
		}
	}
	return nil
}

type usageError struct {
	message string
}

func (e *usageError) Error() string {
	return e.message
}

func parseFormPairs(pairs []string) (map[string]string, error) {
	form := map[string]string{}
	for _, pair := range pairs {
		trimmed := strings.TrimSpace(pair)
		if trimmed == "" {
			continue
		}
		idx := strings.Index(trimmed, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("raw post: invalid --form value %q, expected key=value", pair)
		}
		key := strings.TrimSpace(trimmed[:idx])
		value := trimmed[idx+1:]
		if key == "" {
			return nil, fmt.Errorf("raw post: empty form key in %q", pair)
		}
		form[key] = value
	}
	return form, nil
}
