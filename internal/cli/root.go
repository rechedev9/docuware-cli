// Package cli implements the dw command line.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rechedev9/docuware-cli/internal/config"
	"github.com/rechedev9/docuware-cli/internal/docuware"
)

// Exit codes. They are part of the interface agents rely on.
const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitAuth     = 3
	exitNotFound = 4
)

const metadataTTL = time.Hour

type app struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	getenv         func(string) string
	version        string

	jsonOut bool
	profile string
	noCache bool

	store  *config.Store
	client *docuware.Client
	sess   session
}

// Run executes dw with args and returns the process exit code.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string, version string) int {
	a := &app{stdin: stdin, stdout: stdout, stderr: stderr, getenv: getenv, version: version}
	root := a.rootCmd()
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		return a.reportError(err)
	}
	return exitOK
}

// Main is the entry point used by cmd/dw.
func Main(version string) {
	os.Exit(Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv, version))
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "dw",
		Short: "Work with DocuWare from the terminal",
		Long: `dw searches, reads and downloads documents in DocuWare (Cloud or on-premises,
version 7.10+) through the Platform REST API.

Typical flow:
  dw login --url acme --user peggy.jenkins
  dw cabinets
  dw fields Invoices
  dw search Invoices COMPANY=Peters* INVOICE_DATE=2024-01-01..2024-12-31
  dw text Invoices 42

Add --json to any command for machine-readable output.
Exit codes: 0 ok, 1 error, 2 usage, 3 auth, 4 not found.`,
		SilenceErrors:     true,
		SilenceUsage:      true,
		Version:           a.version,
		CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
	}
	pf := root.PersistentFlags()
	pf.BoolVar(&a.jsonOut, "json", false, "print machine-readable JSON")
	pf.StringVarP(&a.profile, "profile", "p", "", "profile to use (default: current profile, or $DW_PROFILE)")
	pf.BoolVar(&a.noCache, "no-cache", false, "ignore cached cabinet and dialog metadata")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageErr("%s", err.Error()) })

	root.AddCommand(
		a.loginCmd(), a.logoutCmd(), a.statusCmd(),
		a.cabinetsCmd(), a.dialogsCmd(), a.fieldsCmd(), a.valuesCmd(),
		a.searchCmd(), a.getCmd(), a.textCmd(), a.downloadCmd(),
		a.apiCmd(), a.skillCmd(),
	)
	return root
}

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usageErr(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

func exactArgs(n int, names string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != n {
			return usageErr("expected %s", names)
		}
		return nil
	}
}

func minArgs(n int, names string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < n {
			return usageErr("expected %s", names)
		}
		return nil
	}
}

func (a *app) reportError(err error) int {
	code, hint := classify(err)
	if code == exitUsage && strings.HasPrefix(err.Error(), "unknown command") {
		hint = "run `dw --help` to list commands"
	}
	fmt.Fprintf(a.stderr, "error: %v\n", err)
	if hint != "" {
		fmt.Fprintf(a.stderr, "hint: %s\n", hint)
	}
	return code
}

func classify(err error) (int, string) {
	var (
		ue  *usageError
		qe  *docuware.QueryError
		nf  *docuware.NotFoundError
		ae  *docuware.AuthError
		api *docuware.APIError
	)
	switch {
	case errors.As(err, &ue), strings.HasPrefix(err.Error(), "unknown command"):
		return exitUsage, "run `dw <command> --help` for usage"
	case errors.As(err, &qe):
		return exitUsage, "run `dw fields <cabinet>` to see field names and types"
	case errors.Is(err, docuware.ErrNoCredentials):
		return exitAuth, "run `dw login --url <server> --user <name>` in a terminal, or set DW_URL with DW_USERNAME/DW_PASSWORD (or DW_CLIENT_ID/DW_CLIENT_SECRET)"
	case errors.As(err, &ae):
		return exitAuth, "check the username/password or the client id/secret; accounts that sign in through SSO (Microsoft, ADFS, ...) cannot use the API, so use a DocuWare user with a DocuWare password or an OAuth app"
	case errors.As(err, &nf):
		return exitNotFound, ""
	case errors.Is(err, docuware.ErrNoText):
		return exitNotFound, "use `dw download` to get the file itself"
	case errors.As(err, &api):
		switch api.Status {
		case 401:
			return exitAuth, "the token was rejected; run `dw login` again"
		case 403:
			return exitAuth, "this DocuWare user has no permission for that resource"
		case 404:
			return exitNotFound, ""
		case 422:
			return exitError, "DocuWare rejected the query; check values with `dw fields <cabinet>` and narrow the conditions (DocuWare Cloud refuses searches with more than 10000 hits)"
		case 429:
			return exitError, "DocuWare Cloud allows about 60 calls per minute on some endpoints; wait a minute and retry"
		}
	}
	return exitError, ""
}
