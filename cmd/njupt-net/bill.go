package main

import (
	"io"
	"strconv"

	"github.com/spf13/cobra"
)

func newBillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bill",
		Short: "Self bill JSON list endpoints",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newBillOnlineLogCmd())
	cmd.AddCommand(newBillMonthPayCmd())
	cmd.AddCommand(newBillOperatorLogCmd())
	return cmd
}

func newBillOnlineLogCmd() *cobra.Command {
	var flags authFlags
	var startTime string
	var endTime string
	cmd := &cobra.Command{
		Use:   "online-log",
		Short: "Fetch getUserOnlineLog rows",
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
			result, opErr := client.GetUserOnlineLog(cmd.Context(), startTime, endTime)
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				if result.Data == nil {
					return printKV(w, result.Message)
				}
				return printKV(w, result.Message, "total="+strconv.Itoa(result.Data.Total))
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringVar(&startTime, "start-time", "", "Optional start time filter")
	cmd.Flags().StringVar(&endTime, "end-time", "", "Optional end time filter")
	return cmd
}

func newBillMonthPayCmd() *cobra.Command {
	var flags authFlags
	var startTime string
	var endTime string
	cmd := &cobra.Command{
		Use:   "month-pay",
		Short: "Fetch getMonthPay rows",
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
			result, opErr := client.GetMonthPay(cmd.Context(), startTime, endTime)
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				if result.Data == nil {
					return printKV(w, result.Message)
				}
				return printKV(w, result.Message, "total="+strconv.Itoa(result.Data.Total))
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringVar(&startTime, "start-time", "", "Optional start time filter")
	cmd.Flags().StringVar(&endTime, "end-time", "", "Optional end time filter")
	return cmd
}

func newBillOperatorLogCmd() *cobra.Command {
	var flags authFlags
	var startTime string
	var endTime string
	cmd := &cobra.Command{
		Use:   "operator-log",
		Short: "Fetch getOperatorLog rows",
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
			result, opErr := client.GetOperatorLog(cmd.Context(), startTime, endTime)
			return renderOperation(cmd, result, opErr, func(w io.Writer) error {
				if result.Data == nil {
					return printKV(w, result.Message)
				}
				return printKV(w, result.Message, "total="+strconv.Itoa(result.Data.Total))
			})
		},
	}
	bindAuthFlags(cmd, &flags)
	cmd.Flags().StringVar(&startTime, "start-time", "", "Optional start time filter")
	cmd.Flags().StringVar(&endTime, "end-time", "", "Optional end time filter")
	return cmd
}
