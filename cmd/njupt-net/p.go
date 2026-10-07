package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hicancan/njupt-net/v3/p"
)

func pCommand(ctx context.Context, opt options, command string, args []string, prompt io.Writer) (any, error) {
	fs := commandFlags("p " + command)
	var terminal, operator, newPasswordFile, captchaOutput string
	var selfType int
	fs.StringVar(&terminal, "terminal", "pc", "terminal type")
	switch command {
	case "login":
		fs.StringVar(&operator, "operator", "campus", "campus, njxy or cmcc")
	case "self":
		fs.IntVar(&selfType, "type", 1, "0 login, 1 online bridge, 2 password page")
	case "password":
		fs.StringVar(&newPasswordFile, "new-password-file", "", "file containing the new password")
		fs.StringVar(&captchaOutput, "captcha-output", "", "captcha image output")
	case "config", "status", "logout", "error":
	default:
		return nil, invalid("unknown p command")
	}
	if err := parse(fs, args); err != nil {
		return nil, err
	}
	if terminal != "pc" && terminal != "mobile" && terminal != "hipad" && terminal != "vipad" {
		return nil, invalid("terminal must be pc, mobile, hipad or vipad")
	}
	if command == "login" && operator != "campus" && operator != "njxy" && operator != "cmcc" {
		return nil, invalid("operator must be campus, njxy or cmcc")
	}
	if command == "self" && (selfType < 0 || selfType > 2) {
		return nil, invalid("self type must be 0, 1 or 2")
	}
	var newPassword string
	if command == "password" {
		if newPasswordFile == "" {
			return nil, invalid("password requires --new-password-file")
		}
		if err := outputAvailable(captchaOutput); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(newPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("read new password file: %w", err)
		}
		newPassword = strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
		if newPassword == "" {
			return nil, invalid("new password file is empty")
		}
		if strings.ContainsAny(newPassword, "\r\n") {
			return nil, invalid("new password file must contain one line")
		}
	}
	var value credential
	if command == "login" || command == "self" || command == "password" {
		var err error
		_, value, err = configured(opt)
		if err != nil {
			return nil, err
		}
	}
	link, err := openLink(opt)
	if err != nil {
		return nil, err
	}
	defer link.Close()
	portal, err := p.New(link, terminal)
	if err != nil {
		return nil, err
	}
	switch command {
	case "config":
		return portal.Configure(ctx)
	case "status":
		return portal.Status(ctx)
	case "error":
		return portal.ErrorInfo(ctx)
	case "logout":
		return portal.Logout(ctx)
	case "login":
		return portal.Login(ctx, value.Account, value.Password, operator)
	case "self":
		return portal.SelfURL(ctx, value.Account, value.Password, selfType)
	case "password":
		image, err := portal.Captcha(ctx)
		if err != nil {
			return nil, err
		}
		if err := writeExclusive(captchaOutput, image); err != nil {
			return nil, err
		}
		fmt.Fprintf(prompt, "Read %s and enter the captcha: ", captchaOutput)
		code, err := readCaptcha(ctx, os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read captcha: %w", err)
		}
		code = strings.TrimSpace(code)
		if code == "" {
			return nil, invalid("captcha is required")
		}
		return portal.ChangePassword(ctx, value.Account, value.Password, newPassword, code)
	}
	return nil, invalid("unknown p command")
}
