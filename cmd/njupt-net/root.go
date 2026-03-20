package main

import (
	"context"
	"io"

	"github.com/hicancan/njupt-net-cli/internal/app"
	"github.com/hicancan/njupt-net-cli/internal/kernel"
	"github.com/hicancan/njupt-net-cli/internal/output"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	ConfigPath  string
	OutputMode  string
	AssumeYes   bool
	InsecureTLS bool
}

type commandEnv struct {
	appCtx   *app.Context
	appOpts  rootOptions
	renderer *output.Renderer
	rendered rootOptions
}

type commandEnvKey struct{}

func newCommandEnv() *commandEnv {
	return &commandEnv{}
}

func (env *commandEnv) rendererFor(cmd *cobra.Command, out io.Writer) (*output.Renderer, error) {
	if env == nil {
		return nil, &kernel.OpError{
			Op:      "cmd.context",
			Message: "command environment not initialized",
			Err:     kernel.ErrInvalidConfig,
			ProblemDetails: kernel.ConfigProblemDetails{
				Field: "commandEnv",
				Hint:  "construct commands through newRootCmd",
			},
		}
	}
	if env.appCtx != nil && env.appCtx.Renderer != nil {
		return env.appCtx.Renderer, nil
	}
	opts, err := currentRootOptions(cmd)
	if err != nil {
		return nil, err
	}
	if env.renderer != nil && env.rendered == opts {
		return env.renderer, nil
	}
	renderer, err := output.NewRenderer(out, opts.OutputMode)
	if err != nil {
		return nil, err
	}
	env.renderer = renderer
	env.rendered = opts
	return renderer, nil
}

func (env *commandEnv) appContext(cmd *cobra.Command, out io.Writer) (*app.Context, error) {
	if env == nil {
		return nil, &kernel.OpError{
			Op:      "cmd.context",
			Message: "command environment not initialized",
			Err:     kernel.ErrInvalidConfig,
			ProblemDetails: kernel.ConfigProblemDetails{
				Field: "commandEnv",
				Hint:  "construct commands through newRootCmd",
			},
		}
	}
	opts, err := currentRootOptions(cmd)
	if err != nil {
		return nil, err
	}
	if env.appCtx != nil && env.appOpts == opts {
		return env.appCtx, nil
	}
	appCtx, err := app.Load(app.Options{
		ConfigPath:  opts.ConfigPath,
		OutputMode:  opts.OutputMode,
		AssumeYes:   opts.AssumeYes,
		InsecureTLS: opts.InsecureTLS,
	}, out)
	if err != nil {
		return nil, err
	}
	env.appCtx = appCtx
	env.appOpts = opts
	return env.appCtx, nil
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:   "njupt-net",
		Short: "NJUPT network terminal system",
		Long:  "Kernel-first terminal system for NJUPT Self and Portal workflows.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.SetContext(context.WithValue(context.Background(), commandEnvKey{}, newCommandEnv()))

	cmd.PersistentFlags().StringVar(&opts.ConfigPath, "config", "", "Path to config.json")
	cmd.PersistentFlags().StringVar(&opts.OutputMode, "output", "", "Output mode: human|json")
	cmd.PersistentFlags().BoolVar(&opts.AssumeYes, "yes", false, "Allow side-effecting operations without confirmation")
	cmd.PersistentFlags().BoolVar(&opts.InsecureTLS, "insecure-tls", false, "Allow insecure TLS certificates for Portal requests")

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

func getCommandEnv(cmd *cobra.Command) (*commandEnv, error) {
	ctx := cmd.Context()
	if ctx == nil && cmd != nil && cmd.Root() != nil {
		ctx = cmd.Root().Context()
	}
	var value any
	if ctx != nil {
		value = ctx.Value(commandEnvKey{})
	}
	env, ok := value.(*commandEnv)
	if ok && env != nil {
		return env, nil
	}
	return nil, &kernel.OpError{
		Op:      "cmd.context",
		Message: "command environment not initialized",
		Err:     kernel.ErrInvalidConfig,
		ProblemDetails: kernel.ConfigProblemDetails{
			Field: "commandEnv",
			Hint:  "use the root command context provided by newRootCmd",
		},
	}
}

func currentRootOptions(cmd *cobra.Command) (rootOptions, error) {
	root := cmd
	if root == nil {
		return rootOptions{}, &kernel.OpError{
			Op:      "cmd.context",
			Message: "command is nil",
			Err:     kernel.ErrInvalidConfig,
			ProblemDetails: kernel.ConfigProblemDetails{
				Field: "command",
			},
		}
	}
	if root.Root() != nil {
		root = root.Root()
	}
	opts := rootOptions{}
	opts.ConfigPath, _ = root.Flags().GetString("config")
	opts.OutputMode, _ = root.Flags().GetString("output")
	opts.AssumeYes, _ = root.Flags().GetBool("yes")
	opts.InsecureTLS, _ = root.Flags().GetBool("insecure-tls")
	return opts, nil
}
