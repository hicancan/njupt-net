package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hicancan/njupt-net/v4/p"
)

func pCommand(ctx context.Context, owner *commandContext, alias, command string, args []string, prompt io.Writer) (any, error) {
	fs := commandFlags("p " + command)
	var terminal, operator, newPasswordFile, captchaOutput, errorCode, answer string
	var selfType, port int
	var verifyWithSelf bool
	fs.StringVar(&terminal, "terminal", "pc", "terminal type")
	fs.IntVar(&port, "port", p.DefaultPort, "portal API port: 801, 802, 803 or 804")
	switch command {
	case "login":
		fs.StringVar(&operator, "operator", "campus", "campus, njxy or cmcc")
		fs.BoolVar(&verifyWithSelf, "verify-with-self", false, "verify masked account ownership via Self")
	case "self":
		fs.IntVar(&selfType, "type", 1, "0 login, 1 online bridge, 2 password page")
	case "error-prompt":
		fs.StringVar(&errorCode, "code", "", "portal error code to translate")
	case "captcha":
		fs.StringVar(&captchaOutput, "output", "", "captcha image output")
	case "captcha-check":
		fs.StringVar(&answer, "answer", "", "answer for the current portal session image")
	case "password":
		fs.StringVar(&newPasswordFile, "new-password-file", "", "file containing the new password")
		fs.StringVar(&captchaOutput, "captcha-output", "", "captcha image output")
	case "config", "status", "logout", "error", "notice":
	default:
		return nil, invalid("unknown p command")
	}
	if err := parse(fs, args); err != nil {
		return nil, err
	}
	if terminal != "pc" && terminal != "mobile" && terminal != "hipad" && terminal != "vipad" {
		return nil, invalid("terminal must be pc, mobile, hipad or vipad")
	}
	if port < 801 || port > 804 {
		return nil, invalid("portal port must be 801, 802, 803 or 804")
	}
	if command == "login" && operator != "campus" && operator != "njxy" && operator != "cmcc" {
		return nil, invalid("operator must be campus, njxy or cmcc")
	}
	if command == "self" && (selfType < 0 || selfType > 2) {
		return nil, invalid("self type must be 0, 1 or 2")
	}
	if command == "error-prompt" && strings.TrimSpace(errorCode) == "" {
		return nil, invalid("error-prompt requires --code")
	}
	if command == "captcha-check" && (!owner.stream || strings.TrimSpace(answer) == "") {
		return nil, invalid("captcha-check requires --answer and a captcha in the same CLI session")
	}
	if command == "captcha" {
		if err := outputAvailable(captchaOutput); err != nil {
			return nil, err
		}
	}
	var newPassword string
	if command == "password" {
		if owner.stream {
			return nil, invalid("p password requires interactive stdin and is unavailable in a session")
		}
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
		_, value, err = owner.configured(alias, true)
		if err != nil {
			return nil, err
		}
	}
	portal, err := owner.portal(terminal, port)
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
	case "error-prompt":
		return portal.ErrorPrompt(ctx, errorCode)
	case "notice":
		return portal.Notice(ctx)
	case "captcha":
		image, err := portal.Captcha(ctx)
		if err != nil {
			return nil, err
		}
		if err := writeExclusive(captchaOutput, image); err != nil {
			return nil, err
		}
		return fileResult{Output: captchaOutput, Bytes: len(image), Format: "image"}, nil
	case "captcha-check":
		return portal.CheckCaptcha(ctx, answer)
	case "logout":
		return portal.Logout(ctx)
	case "login":
		if verifyWithSelf {
			return loginWithSelfReadback(ctx, owner, portal, alias, value, operator)
		}
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
