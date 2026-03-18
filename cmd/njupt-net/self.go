package main

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
	"github.com/hicancan/njupt-net-cli/internal/workflow"
)

func newSelfCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "self",
		Short: "Self service authentication and diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSelfLoginCmd())
	cmd.AddCommand(newSelfLogoutCmd())
	cmd.AddCommand(newSelfStatusCmd())
	cmd.AddCommand(newSelfDoctorCmd())
	return cmd
}

func newSelfLoginCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Run the authoritative Self login chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := resolveAccount(cmd, flags)
			if err != nil {
				return err
			}
			client, err := newSelfClient(cmd)
			if err != nil {
				return err
			}
			result, opErr := client.Login(cmd.Context(), account.Username, account.Password)
			if err := render(cmd, result, func(w io.Writer) error {
				return printKV(w, result.Message)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newSelfLogoutCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Login with the provided credentials, then verify logout",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "self logout"); err != nil {
				return err
			}
			account, err := resolveAccount(cmd, flags)
			if err != nil {
				return err
			}
			client, err := newSelfClient(cmd)
			if err != nil {
				return err
			}
			if _, err := client.Login(cmd.Context(), account.Username, account.Password); err != nil {
				return err
			}
			result, opErr := client.Logout(cmd.Context())
			if err := render(cmd, result, func(w io.Writer) error {
				return printKV(w, result.Message)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newSelfStatusCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Login and verify protected-page readability",
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := resolveAccount(cmd, flags)
			if err != nil {
				return err
			}
			client, err := newSelfClient(cmd)
			if err != nil {
				return err
			}
			loginResult, loginErr := client.Login(cmd.Context(), account.Username, account.Password)
			if loginErr != nil {
				if err := render(cmd, loginResult, func(w io.Writer) error {
					return printKV(w, loginResult.Message)
				}); err != nil {
					return err
				}
				return loginErr
			}
			statusResult, statusErr := client.Status(cmd.Context())
			if err := render(cmd, statusResult, func(w io.Writer) error {
				if statusResult.Data == nil {
					return printKV(w, statusResult.Message)
				}
				return printKV(w,
					statusResult.Message,
					"loggedIn="+boolString(statusResult.Data.LoggedIn),
					"dashboardReadable="+boolString(statusResult.Data.DashboardReadable),
					"serviceReadable="+boolString(statusResult.Data.ServiceReadable),
				)
			}); err != nil {
				return err
			}
			if statusErr != nil {
				return statusErr
			}
			if loginResult != nil && !loginResult.Success {
				return &kernel.OpError{Op: "self.status", Message: "login chain failed", Err: kernel.ErrAuth}
			}
			return nil
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newSelfDoctorCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run a typed Self diagnosis chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := resolveAccount(cmd, flags)
			if err != nil {
				return err
			}
			client, err := newSelfClient(cmd)
			if err != nil {
				return err
			}
			result, opErr := workflow.SelfDoctor(cmd.Context(), client, account.Username, account.Password)
			if err := render(cmd, result, func(w io.Writer) error {
				lines := []string{result.Message}
				if result.Data != nil && result.Data.Status != nil && result.Data.Status.Data != nil {
					lines = append(lines, "loggedIn="+boolString(result.Data.Status.Data.LoggedIn))
				}
				return printKV(w, lines...)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
