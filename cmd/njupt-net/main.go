package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/hicancan/njupt-net/v3/network"
)

const usage = `njupt-net — NJUPT campus network CLI

njupt-net interfaces
njupt-net version
njupt-net [GLOBAL FLAGS] probe
njupt-net [GLOBAL FLAGS] p COMMAND [COMMAND FLAGS]
njupt-net [GLOBAL FLAGS] zfw COMMAND [COMMAND FLAGS]

Global flags precede the command group:
  --interface NAME         use this interface's single IPv4 address
  --source IPv4            use this assigned IPv4 address
  --config config.json     credentials file
  --account ALIAS          configured account alias
  --self-url-file PATH     private zfw commands: use this p bridge URL instead of a password
  --timeout 15s            timeout per HTTP request
Exactly one of --interface or --source is required except for version, interfaces and --help.

p commands:
  config, status, error              inspect the selected terminal's portal
  login       [--operator campus|njxy|cmcc]
  logout                             disconnect the selected terminal
  self        [--type 0|1|2]         obtain the self-service entry URL
  password    --new-password-file PATH --captcha-output PATH
Every p command accepts --terminal pc|mobile|hipad|vipad (default: pc).
login, self and password require --account. Password reads the image answer
from stdin; the new password file contains one password with an optional newline.

zfw commands:
  verify                             verify account identity and close the session
  account, refresh, profile          account overview, refresh and profile
  online, history                    current connections and recent logins
  offline     --session ID            disconnect an account connection
  devices     [--page 1] [--size 10]  MAC bindings; size: 10, 25, 50 or 100
  unbind      --mac ADDRESS           remove a MAC binding
  consume     [--limit AMOUNT]        query or set; 999999 means no limit
  operator    [--bind | --unbind njxy|cmcc] query, bind or clear a broadband account
  mauth       [--change]              query or execute the current policy action
  recharge                           show recharge availability
  notice, help, agreement             public pages; no --account required
  bills       --kind online|monthly|operations [QUERY FLAGS]
  export      --kind online|monthly|operations --output PATH [--all] [QUERY FLAGS]
Each private zfw command logs in, executes once, and closes its management session.
With --self-url-file, --account still selects the expected identity; its password
is unused. The file contains one bridge URL with an optional trailing newline.

Bill query flags:
  --start YYYY-MM-DD --end YYYY-MM-DD  online/operations date range
  --year YYYY                         monthly billing year
  --page 1 --size 10 --sort FIELD --order ASC|DESC

Output is JSON. Errors go to stderr; usage errors exit 2, operation errors exit 1.
Output files must not already exist.
`

type options struct {
	config, account, iface, source string
	selfURLFile                    string
	timeout                        time.Duration
}

var version = "dev"

type versionInfo struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	cancel()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := commandFlags("njupt-net")
	var opt options
	fs.StringVar(&opt.config, "config", "config.json", "credentials file")
	fs.StringVar(&opt.account, "account", "", "configured account alias")
	fs.StringVar(&opt.selfURLFile, "self-url-file", "", "private zfw login through a short-lived p bridge URL")
	fs.StringVar(&opt.iface, "interface", "", "campus interface")
	fs.StringVar(&opt.source, "source", "", "campus source IPv4")
	fs.DurationVar(&opt.timeout, "timeout", 15*time.Second, "request timeout")
	if err := fs.Parse(args); err != nil {
		return finish(stdout, stderr, "njupt-net", nil, &argumentError{err})
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}
	group := remaining[0]
	if opt.selfURLFile != "" && group != "zfw" {
		return finish(stdout, stderr, group, nil, invalid("--self-url-file applies only to private zfw commands"))
	}
	if len(remaining) == 2 && (remaining[1] == "--help" || remaining[1] == "-h") && (group == "p" || group == "zfw") {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if group == "interfaces" || group == "probe" || group == "version" {
		if err := parse(commandFlags(group), remaining[1:]); err != nil {
			return finish(stdout, stderr, group, nil, err)
		}
		if group == "interfaces" {
			data, err := network.Interfaces()
			return finish(stdout, stderr, group, data, err)
		}
		if group == "version" {
			return finish(stdout, stderr, group, versionInfo{buildVersion(), runtime.Version(), runtime.GOOS, runtime.GOARCH}, nil)
		}
		link, err := openLink(opt)
		if err != nil {
			return finish(stdout, stderr, group, nil, err)
		}
		defer link.Close()
		data, err := link.Probe(ctx)
		return finish(stdout, stderr, group, data, err)
	}
	if group != "p" && group != "zfw" {
		return finish(stdout, stderr, group, nil, invalid("unknown command; use version, interfaces, probe, p or zfw"))
	}
	if len(remaining) < 2 {
		return finish(stdout, stderr, group, nil, invalid("a command is required; use --help"))
	}
	command := remaining[1]
	var data any
	var err error
	if group == "p" {
		data, err = pCommand(ctx, opt, command, remaining[2:], stderr)
	} else {
		data, err = zfwCommand(ctx, opt, command, remaining[2:])
	}
	return finish(stdout, stderr, group+" "+command, data, err)
}

type argumentError struct{ err error }

func (e *argumentError) Error() string { return e.err.Error() }
func (e *argumentError) Unwrap() error { return e.err }
func invalid(message string) error     { return &argumentError{errors.New(message)} }

func commandFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return &argumentError{err}
	}
	if fs.NArg() != 0 {
		return invalid("unexpected positional arguments")
	}
	return nil
}

func openLink(opt options) (*network.Link, error) {
	if (opt.iface == "") == (opt.source == "") {
		return nil, invalid("specify exactly one of --interface or --source")
	}
	source, err := network.SourceAddress(opt.iface, opt.source)
	if err != nil {
		return nil, &argumentError{err}
	}
	link, err := network.NewLink(source, opt.timeout)
	if err != nil {
		return nil, &argumentError{err}
	}
	return link, nil
}
