package main

import (
	"io"

	"github.com/spf13/cobra"
)

func newPortalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "portal",
		Short: "Portal 802 primary flow and guarded 801 fallback",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPortalLoginCmd())
	cmd.AddCommand(newPortalLogoutCmd())
	cmd.AddCommand(newPortalLogin801Cmd())
	cmd.AddCommand(newPortalLogout801Cmd())
	return cmd
}

func newPortalLoginCmd() *cobra.Command {
	var flags authFlags
	var ip string
	var isp string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Run the confirmed Portal 802 login path",
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := resolveAccount(cmd, flags)
			if err != nil {
				return err
			}
			appCtx, err := appContext(cmd)
			if err != nil {
				return err
			}
			if ip == "" {
				return &usageError{message: "portal login requires --ip"}
			}
			if isp == "" {
				isp = appCtx.Config.Portal.ISP
			}
			client, err := newPortalClient(cmd)
			if err != nil {
				return err
			}
			result, opErr := client.Login802(cmd.Context(), account.Username, account.Password, ip, isp)
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				if result.Data == nil {
					return printKV(w, result.Message)
				}
				return printKV(w,
					result.Message,
					"retCode="+result.Data.RetCode,
					"msg="+result.Data.Msg,
					"endpoint="+result.Data.Endpoint,
				)
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringVar(&ip, "ip", "", "Current WLAN IPv4")
	cmd.Flags().StringVar(&isp, "isp", "", "ISP suffix route: telecom|unicom|mobile")
	return cmd
}

func newPortalLogoutCmd() *cobra.Command {
	var ip string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Send the Portal 802 logout request",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "portal logout"); err != nil {
				return err
			}
			if ip == "" {
				return &usageError{message: "portal logout requires --ip"}
			}
			client, err := newPortalClient(cmd)
			if err != nil {
				return err
			}
			result, opErr := client.Logout802(cmd.Context(), ip)
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			})
		},
	}
	cmd.Flags().StringVar(&ip, "ip", "", "Current WLAN IPv4")
	return cmd
}

func newPortalLogin801Cmd() *cobra.Command {
	var flags authFlags
	var ip string
	var ipv6 string
	cmd := &cobra.Command{
		Use:   "login-801",
		Short: "Run the guarded raw Portal 801 fallback login",
		RunE: func(cmd *cobra.Command, args []string) error {
			account, err := resolveAccount(cmd, flags)
			if err != nil {
				return err
			}
			if ip == "" {
				return &usageError{message: "portal login-801 requires --ip"}
			}
			client, err := newPortalClient(cmd)
			if err != nil {
				return err
			}
			result, opErr := client.Login801(cmd.Context(), account.Username, account.Password, ip, ipv6)
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringVar(&ip, "ip", "", "Current WLAN IPv4")
	cmd.Flags().StringVar(&ipv6, "ipv6", "", "Current WLAN IPv6")
	return cmd
}

func newPortalLogout801Cmd() *cobra.Command {
	var ip string
	cmd := &cobra.Command{
		Use:   "logout-801",
		Short: "Run the guarded raw Portal 801 fallback logout",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "portal logout-801"); err != nil {
				return err
			}
			if ip == "" {
				return &usageError{message: "portal logout-801 requires --ip"}
			}
			client, err := newPortalClient(cmd)
			if err != nil {
				return err
			}
			result, opErr := client.Logout801(cmd.Context(), ip)
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				return printKV(w, result.Message)
			})
		},
	}
	cmd.Flags().StringVar(&ip, "ip", "", "Current WLAN IPv4")
	return cmd
}
