package main

import (
	"io"

	"github.com/spf13/cobra"
)

func newSettingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setting",
		Short: "Self setting and guarded person-state commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSettingPersonCmd())
	return cmd
}

func newSettingPersonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "person",
		Short: "Guarded person-list operations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSettingPersonGetCmd())
	cmd.AddCommand(newSettingPersonUpdateCmd())
	return cmd
}

func newSettingPersonGetCmd() *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Load guarded person-list state",
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
			result, opErr := client.GetPerson(cmd.Context())
			if err := render(cmd, result, func(w io.Writer) error {
				if result.Data == nil {
					return printKV(w, result.Message)
				}
				return printKV(w, result.Message, "csrftoken="+result.Data.CSRFTOKEN)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	return cmd
}

func newSettingPersonUpdateCmd() *cobra.Command {
	var flags authFlags
	var fieldPairs []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Submit guarded updateUserSecurity payload",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireYes(cmd, "setting person update"); err != nil {
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
			form, err := parseFormPairs(fieldPairs)
			if err != nil {
				return err
			}
			result, opErr := client.UpdateUserSecurity(cmd.Context(), form, dryRun)
			if err := render(cmd, result, func(w io.Writer) error {
				return printKV(w, result.Message)
			}); err != nil {
				return err
			}
			return opErr
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringArrayVarP(&fieldPairs, "field", "f", nil, "Form field pair in key=value format; can be repeated")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Only fetch guarded state and report blocked semantics")
	return cmd
}
