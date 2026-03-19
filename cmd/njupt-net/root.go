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
	opts     rootOptions
	appCtx   *app.Context
	renderer *output.Renderer
}

type commandEnvKey struct{}

func newCommandEnv(opts rootOptions) *commandEnv {
	return &commandEnv{opts: opts}
}

func (env *commandEnv) rendererFor(out io.Writer) (*output.Renderer, error) {
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
	if env.renderer != nil {
		return env.renderer, nil
	}
	renderer, err := output.NewRenderer(out, env.opts.OutputMode)
	if err != nil {
		return nil, err
	}
	env.renderer = renderer
	return renderer, nil
}

func (env *commandEnv) appContext(out io.Writer) (*app.Context, error) {
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
	if env.appCtx != nil {
		return env.appCtx, nil
	}
	appCtx, err := app.Load(app.Options{
		ConfigPath:  env.opts.ConfigPath,
		OutputMode:  env.opts.OutputMode,
		AssumeYes:   env.opts.AssumeYes,
		InsecureTLS: env.opts.InsecureTLS,
	}, out)
	if err != nil {
		return nil, err
	}
	env.appCtx = appCtx
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
	cmd.SetContext(context.WithValue(context.Background(), commandEnvKey{}, newCommandEnv(*opts)))

	cmd.PersistentFlags().StringVar(&opts.ConfigPath, "config", "", "Path to credentials.json or compatible config")
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
		env.opts.ConfigPath, _ = cmd.Root().Flags().GetString("config")
		env.opts.OutputMode, _ = cmd.Root().Flags().GetString("output")
		env.opts.AssumeYes, _ = cmd.Root().Flags().GetBool("yes")
		env.opts.InsecureTLS, _ = cmd.Root().Flags().GetBool("insecure-tls")
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
