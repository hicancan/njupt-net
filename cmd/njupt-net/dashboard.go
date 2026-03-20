package main

import (
	"io"

	"github.com/spf13/cobra"
)

func newDashboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Dashboard data and actions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newDashboardOnlineListCmd())
	cmd.AddCommand(newDashboardLoginHistoryCmd())
	cmd.AddCommand(newDashboardRefreshAccountCmd())
	cmd.AddCommand(newDashboardMauthCmd())
	cmd.AddCommand(newDashboardOfflineCmd())
	return cmd
}

func newDashboardOnlineListCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "online-list",
		Short: "Fetch current online sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			result, opErr := client.GetOnlineList(cmd.Context())
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newDashboardLoginHistoryCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "login-history",
		Short: "Fetch login history rows",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			result, opErr := client.GetLoginHistory(cmd.Context())
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newDashboardRefreshAccountCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "refresh-account-raw",
		Short: "Run the refreshaccount raw probe",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			result, opErr := client.RefreshAccountRaw(cmd.Context())
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newDashboardMauthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mauth",
		Short: "Read and toggle mauth state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newDashboardMauthGetCmd())
	cmd.AddCommand(newDashboardMauthToggleCmd())
	return cmd
}

func newDashboardMauthGetCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Read mauth state",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			result, opErr := client.GetMauthState(cmd.Context())
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newDashboardMauthToggleCmd() *cobra.Command {
	var flags authFlags
	var restore bool
	cmd := &cobra.Command{
		Use:   "toggle",
		Short: "Toggle mauth state with readback verification",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "dashboard mauth toggle"); err != nil {
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
			result, opErr := client.ToggleMauth(cmd.Context())
			if err := renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			}); err != nil {
				return err
			}
			if opErr != nil {
				return opErr
			}
			if restore {
				_, _ = client.ToggleMauth(cmd.Context())
			}
			return nil
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().BoolVar(&restore, "restore", false, "Toggle back after successful verification")
	return cmd
}

func newDashboardOfflineCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "offline <sessionid>",
		Short: "Force-offline request with bounded readback verification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "dashboard offline"); err != nil {
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
			result, opErr := client.ForceOffline(cmd.Context(), args[0])
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}
