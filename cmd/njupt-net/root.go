package main

import (
	"github.com/hicancan/njupt-net-cli/internal/app"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	ConfigPath  string
	OutputMode  string
	AssumeYes   bool
	InsecureTLS bool

	appCtx *app.Context
}

var rootOpts = &rootOptions{}

func (o *rootOptions) load(cmd *cobra.Command) (*app.Context, error) {
	if o.appCtx != nil {
		return o.appCtx, nil
	}
	ctx, err := app.Load(app.Options{
		ConfigPath:  o.ConfigPath,
		OutputMode:  o.OutputMode,
		AssumeYes:   o.AssumeYes,
		InsecureTLS: o.InsecureTLS,
	}, cmd.OutOrStdout())
	if err != nil {
		return nil, err
	}
	o.appCtx = ctx
	return ctx, nil
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "njupt-net",
		Short: "NJUPT network terminal system",
		Long:  "Kernel-first terminal system for NJUPT Self and Portal workflows.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&rootOpts.ConfigPath, "config", "", "Path to credentials.json or compatible config")
	cmd.PersistentFlags().StringVar(&rootOpts.OutputMode, "output", "", "Output mode: human|json")
	cmd.PersistentFlags().BoolVar(&rootOpts.AssumeYes, "yes", false, "Allow side-effecting operations without confirmation")
	cmd.PersistentFlags().BoolVar(&rootOpts.InsecureTLS, "insecure-tls", false, "Allow insecure TLS certificates for Portal requests")

	cmd.AddCommand(newSelfCmd())
	cmd.AddCommand(newDashboardCmd())
	cmd.AddCommand(newServiceCmd())
	cmd.AddCommand(newSettingCmd())
	cmd.AddCommand(newBillCmd())
	cmd.AddCommand(newPortalCmd())
	cmd.AddCommand(newRawCmd())
	cmd.AddCommand(newGuardCmd())

	return cmd
}
