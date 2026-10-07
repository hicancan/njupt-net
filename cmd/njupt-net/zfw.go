package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hicancan/njupt-net/v3/zfw"
)

func zfwCommand(ctx context.Context, opt options, command string, args []string) (result any, resultErr error) {
	fs := commandFlags("zfw " + command)
	var sessionID, mac, limit, output string
	var bind, change, all bool
	page, size := 1, 10
	var query zfw.BillQuery
	switch command {
	case "verify", "account", "refresh", "profile", "online", "history", "recharge", "notice", "help", "agreement":
	case "offline":
		fs.StringVar(&sessionID, "session", "", "online session ID")
	case "devices":
		fs.IntVar(&page, "page", 1, "page number")
		fs.IntVar(&size, "size", 10, "page size")
	case "unbind":
		fs.StringVar(&mac, "mac", "", "device MAC")
	case "consume":
		fs.StringVar(&limit, "limit", "", "new limit; omitted means query")
	case "operator":
		fs.BoolVar(&bind, "bind", false, "submit configured broadband account")
	case "mauth":
		fs.BoolVar(&change, "change", false, "execute the current mauth action")
	case "bills", "export":
		fs.StringVar(&query.Kind, "kind", "", "online, monthly or operations")
		fs.StringVar(&query.Start, "start", "", "start date")
		fs.StringVar(&query.End, "end", "", "end date")
		fs.IntVar(&query.Year, "year", 0, "billing year")
		fs.IntVar(&query.Page, "page", 1, "page number")
		fs.IntVar(&query.Size, "size", 10, "page size")
		fs.StringVar(&query.Sort, "sort", "", "server sort field")
		fs.StringVar(&query.Order, "order", "DESC", "ASC or DESC")
		if command == "export" {
			fs.StringVar(&output, "output", "", "XLS output path")
			fs.BoolVar(&all, "all", false, "export entire selected date/year range")
		}
	default:
		return nil, invalid("unknown zfw command")
	}
	if err := parse(fs, args); err != nil {
		return nil, err
	}
	if command == "offline" && sessionID == "" {
		return nil, invalid("offline requires --session")
	}
	if command == "unbind" {
		if err := zfw.ValidateMAC(mac); err != nil {
			return nil, &argumentError{err}
		}
	}
	if command == "export" {
		if err := outputAvailable(output); err != nil {
			return nil, err
		}
	}
	if command == "bills" || command == "export" {
		if err := query.Validate(); err != nil {
			return nil, &argumentError{err}
		}
	}
	if command == "devices" && (page < 1 || (size != 10 && size != 25 && size != 50 && size != 100)) {
		return nil, invalid("devices require page >= 1 and size 10, 25, 50 or 100")
	}
	if command == "consume" && limit != "" {
		if err := zfw.ValidateConsumeLimit(limit); err != nil {
			return nil, &argumentError{err}
		}
	}
	public := command == "notice" || command == "help" || command == "agreement"
	if public && opt.selfURLFile != "" {
		return nil, invalid("--self-url-file applies only to private zfw commands")
	}
	var bridgeURL string
	if opt.selfURLFile != "" {
		data, err := os.ReadFile(opt.selfURLFile)
		if err != nil {
			return nil, fmt.Errorf("read Self bridge URL file: %w", err)
		}
		bridgeURL = strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
		if err := zfw.ValidateBridgeURL(bridgeURL); err != nil {
			return nil, &argumentError{err}
		}
	}
	var cfg config
	var value credential
	if !public {
		var err error
		if bridgeURL == "" {
			cfg, value, err = configured(opt)
		} else {
			cfg, value, err = configuredIdentity(opt)
		}
		if err != nil {
			return nil, err
		}
		if command == "operator" && bind {
			broadband := cfg.BroadbandAccount
			if broadband.Operator != "njxy" && broadband.Operator != "cmcc" {
				return nil, fmt.Errorf("configured broadband operator must be njxy or cmcc")
			}
			if broadband.Account == "" || broadband.Password == "" {
				return nil, fmt.Errorf("configured broadband account and password are required")
			}
		}
	}
	link, err := openLink(opt)
	if err != nil {
		return nil, err
	}
	defer link.Close()
	self := zfw.New(link)
	if public {
		return self.PublicPage(ctx, command)
	}
	if bridgeURL == "" {
		err = self.Login(ctx, value.Account, value.Password)
	} else {
		err = self.LoginBridge(ctx, value.Account, bridgeURL)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opt.timeout)
		defer cancel()
		if err := self.Logout(closeCtx); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close zfw management session: %w", err))
		}
	}()
	switch command {
	case "verify":
		method := "password"
		var passwordValid *bool
		if bridgeURL == "" {
			valid := true
			passwordValid = &valid
		} else {
			method = "portal_bridge"
		}
		return struct {
			CredentialsValid *bool  `json:"credentials_valid,omitempty"`
			IdentityVerified bool   `json:"identity_verified"`
			LoginMethod      string `json:"login_method"`
			AccountAlias     string `json:"account_alias"`
		}{passwordValid, true, method, opt.account}, nil
	case "account":
		return self.Overview(ctx)
	case "refresh":
		if err := self.RefreshAccount(ctx); err != nil {
			return nil, err
		}
		return self.Overview(ctx)
	case "profile":
		return self.Profile(ctx)
	case "online":
		return self.Online(ctx)
	case "history":
		return self.History(ctx)
	case "offline":
		return self.Offline(ctx, sessionID)
	case "devices":
		return self.Devices(ctx, page, size)
	case "unbind":
		return self.Unbind(ctx, mac)
	case "consume":
		if limit == "" {
			return self.ConsumeProtect(ctx)
		}
		return self.SetConsumeProtect(ctx, limit)
	case "operator":
		if bind {
			broadband := cfg.BroadbandAccount
			return self.BindOperator(ctx, broadband.Operator, broadband.Account, broadband.Password)
		}
		return self.OperatorBinding(ctx)
	case "mauth":
		if change {
			return self.ChangeMauth(ctx)
		}
		return self.Mauth(ctx)
	case "recharge":
		return self.Recharge(ctx)
	case "bills":
		return self.Bills(ctx, query)
	case "export":
		data, err := self.ExportBills(ctx, query, all)
		if err != nil {
			return nil, err
		}
		if err := writeExclusive(output, data); err != nil {
			return nil, err
		}
		return fileResult{Output: output, Bytes: len(data), Format: "xls"}, nil
	}
	return nil, invalid("unknown zfw command")
}
