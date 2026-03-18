package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/hicancan/njupt-net-cli/internal/workflow"
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Broadband binding, consume protect, and MAC registry commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServiceBindingCmd())
	cmd.AddCommand(newServiceConsumeCmd())
	cmd.AddCommand(newServiceMacCmd())
	cmd.AddCommand(newServiceMigrateCmd())
	return cmd
}

func newServiceBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "binding",
		Short: "Read and update operator broadband binding fields",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServiceBindingGetCmd())
	cmd.AddCommand(newServiceBindingSetCmd())
	return cmd
}

func newServiceBindingGetCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Load FLDEXTRA binding fields",
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
			result, opErr := client.GetOperatorBinding(cmd.Context())
			if err := render(cmd, result, func(w io.Writer) error {
				if result.Data == nil {
					return printKV(w, result.Message)
				}
				return printKV(w,
					result.Message,
					"telecomAccount="+result.Data.TelecomAccount,
					"telecomPassword="+result.Data.TelecomPassword,
					"mobileAccount="+result.Data.MobileAccount,
					"mobilePassword="+result.Data.MobilePassword,
				)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newServiceBindingSetCmd() *cobra.Command {
	var flags authFlags
	var telecomAccount string
	var telecomPassword string
	var mobileAccount string
	var mobilePassword string
	var readback bool
	var restore bool
	var clearAll bool

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Write FLDEXTRA binding fields with readback verification",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "service binding set"); err != nil {
				return err
			}
			account, err := resolveAccount(cmd, flags)
			if err != nil {
				return err
			}
			target := map[string]string{}
			if clearAll {
				target["FLDEXTRA1"] = ""
				target["FLDEXTRA2"] = ""
				target["FLDEXTRA3"] = ""
				target["FLDEXTRA4"] = ""
			}
			if telecomAccount != "" {
				target["FLDEXTRA1"] = telecomAccount
			}
			if telecomPassword != "" {
				target["FLDEXTRA2"] = telecomPassword
			}
			if mobileAccount != "" {
				target["FLDEXTRA3"] = mobileAccount
			}
			if mobilePassword != "" {
				target["FLDEXTRA4"] = mobilePassword
			}
			if len(target) == 0 {
				return &usageError{message: "service binding set requires at least one target field or --clear-all"}
			}

			client, err := newSelfClient(cmd)
			if err != nil {
				return err
			}
			if _, err := client.Login(cmd.Context(), account.Username, account.Password); err != nil {
				return err
			}
			result, opErr := client.BindOperator(cmd.Context(), target, readback, restore)
			if err := render(cmd, result, func(w io.Writer) error {
				return printKV(w, result.Message)
			}); err != nil {
				return err
			}
			return opErr
		},
	}

	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringVar(&telecomAccount, "telecom-account", "", "Value for FLDEXTRA1")
	cmd.Flags().StringVar(&telecomPassword, "telecom-password", "", "Value for FLDEXTRA2")
	cmd.Flags().StringVar(&mobileAccount, "mobile-account", "", "Value for FLDEXTRA3")
	cmd.Flags().StringVar(&mobilePassword, "mobile-password", "", "Value for FLDEXTRA4")
	cmd.Flags().BoolVar(&clearAll, "clear-all", false, "Clear all four FLDEXTRA fields before optional overrides")
	cmd.Flags().BoolVar(&readback, "readback", true, "Verify post-submit readback")
	cmd.Flags().BoolVar(&restore, "restore", false, "Restore the pre-submit state after verification")
	return cmd
}

func newServiceConsumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "consume",
		Short: "Read and update consume-protect state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServiceConsumeGetCmd())
	cmd.AddCommand(newServiceConsumeSetCmd())
	return cmd
}

func newServiceConsumeGetCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Load consume-protect state",
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
			result, opErr := client.GetConsumeProtect(cmd.Context())
			if err := render(cmd, result, func(w io.Writer) error {
				if result.Data == nil {
					return printKV(w, result.Message)
				}
				return printKV(w,
					result.Message,
					"installmentFlag="+result.Data.InstallmentFlag,
					"currentLimit="+result.Data.CurrentLimit,
					"currentUsage="+result.Data.CurrentUsage,
					"balance="+result.Data.Balance,
				)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newServiceConsumeSetCmd() *cobra.Command {
	var flags authFlags
	var limit string
	var readback bool
	var restore bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update consume-protect limit with readback verification",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "service consume set"); err != nil {
				return err
			}
			if limit == "" {
				return &usageError{message: "service consume set requires --limit"}
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
			result, opErr := client.ChangeConsumeProtect(cmd.Context(), limit, readback, restore)
			if err := render(cmd, result, func(w io.Writer) error {
				return printKV(w, result.Message)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringVar(&limit, "limit", "", "Target consumeLimit/installmentFlag value")
	cmd.Flags().BoolVar(&readback, "readback", true, "Verify post-submit readback")
	cmd.Flags().BoolVar(&restore, "restore", false, "Restore the original state after verification")
	return cmd
}

func newServiceMacCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mac",
		Short: "MAC registry commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServiceMacListCmd())
	return cmd
}

func newServiceMacListCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Fetch the MAC registry JSON list",
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
			result, opErr := client.GetMacList(cmd.Context())
			if err := render(cmd, result, func(w io.Writer) error {
				if result.Data == nil {
					return printKV(w, result.Message)
				}
				return printKV(w, result.Message, "total="+itoa(result.Data.Total))
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newServiceMigrateCmd() *cobra.Command {
	var from authFlags
	var to authFlags
	var telecomAccount string
	var telecomPassword string
	var mobileAccount string
	var mobilePassword string

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Clear broadband fields on one account, then bind them on another",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "service migrate"); err != nil {
				return err
			}
			appCtx, err := rootOpts.load(cmd)
			if err != nil {
				return err
			}

			fromAccount, err := appCtx.Config.ResolveAccount(from.Profile, from.Username, from.Password)
			if err != nil {
				return err
			}
			toAccount, err := appCtx.Config.ResolveAccount(to.Profile, to.Username, to.Password)
			if err != nil {
				return err
			}

			target := map[string]string{}
			if telecomAccount != "" {
				target["FLDEXTRA1"] = telecomAccount
			}
			if telecomPassword != "" {
				target["FLDEXTRA2"] = telecomPassword
			}
			if mobileAccount != "" {
				target["FLDEXTRA3"] = mobileAccount
			}
			if mobilePassword != "" {
				target["FLDEXTRA4"] = mobilePassword
			}
			if len(target) == 0 {
				return &usageError{message: "service migrate requires at least one target FLDEXTRA field"}
			}

			result, opErr := workflow.MigrateBroadband(
				cmd.Context(),
				appCtx.Config.Self.BaseURL,
				fromAccount.Username,
				fromAccount.Password,
				toAccount.Username,
				toAccount.Password,
				target,
			)
			if err := render(cmd, result, func(w io.Writer) error {
				return printKV(w, result.Message)
			}); err != nil {
				return err
			}
			return opErr
		},
	}

	cmd.Flags().StringVar(&from.Profile, "from-profile", "", "Configured source profile from credentials.json")
	cmd.Flags().StringVar(&from.Username, "from-username", "", "Explicit source username")
	cmd.Flags().StringVar(&from.Password, "from-password", "", "Explicit source password")
	cmd.Flags().StringVar(&to.Profile, "to-profile", "", "Configured target profile from credentials.json")
	cmd.Flags().StringVar(&to.Username, "to-username", "", "Explicit target username")
	cmd.Flags().StringVar(&to.Password, "to-password", "", "Explicit target password")
	cmd.Flags().StringVar(&telecomAccount, "telecom-account", "", "Target FLDEXTRA1 value")
	cmd.Flags().StringVar(&telecomPassword, "telecom-password", "", "Target FLDEXTRA2 value")
	cmd.Flags().StringVar(&mobileAccount, "mobile-account", "", "Target FLDEXTRA3 value")
	cmd.Flags().StringVar(&mobilePassword, "mobile-password", "", "Target FLDEXTRA4 value")
	return cmd
}

type usageError struct {
	message string
}

func (e *usageError) Error() string {
	return e.message
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}
