package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newRawCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raw",
		Short: "Low-level Self GET/POST probes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newRawGetCmd())
	cmd.AddCommand(newRawPostCmd())
	return cmd
}

func newRawGetCmd() *cobra.Command {
	var flags authFlags
	var login bool
	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Send a raw GET request and print status/body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newSelfClient(cmd)
			if err != nil {
				return err
			}
			if login {
				account, err := resolveAccount(cmd, flags)
				if err != nil {
					return err
				}
				if _, err := client.Login(cmd.Context(), account.Username, account.Password); err != nil {
					return err
				}
			}
			result, opErr := client.RawGet(cmd.Context(), args[0])
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				if result.Raw == nil {
					return printKV(w, result.Message)
				}
				_, err := fmt.Fprintf(w, "Status: %d\nFinalURL: %s\n%s\n", result.Raw.Status, result.Raw.FinalURL, result.Raw.Body)
				return err
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().BoolVar(&login, "login", false, "Perform authoritative Self login before sending the raw request")
	return cmd
}

func newRawPostCmd() *cobra.Command {
	var flags authFlags
	var formPairs []string
	var login bool
	cmd := &cobra.Command{
		Use:   "post <path>",
		Short: "Send a raw form POST request and print status/body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newSelfClient(cmd)
			if err != nil {
				return err
			}
			if login {
				account, err := resolveAccount(cmd, flags)
				if err != nil {
					return err
				}
				if _, err := client.Login(cmd.Context(), account.Username, account.Password); err != nil {
					return err
				}
			}
			form, err := parseFormPairs(formPairs)
			if err != nil {
				return err
			}
			result, opErr := client.RawPost(cmd.Context(), args[0], form)
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				if result.Raw == nil {
					return printKV(w, result.Message)
				}
				_, err := fmt.Fprintf(w, "Status: %d\nFinalURL: %s\n%s\n", result.Raw.Status, result.Raw.FinalURL, result.Raw.Body)
				return err
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringArrayVarP(&formPairs, "form", "f", nil, "Form field pair in key=value format; can be repeated")
	cmd.Flags().BoolVar(&login, "login", false, "Perform authoritative Self login before sending the raw request")
	return cmd
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
