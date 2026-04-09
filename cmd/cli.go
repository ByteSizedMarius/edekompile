// Exit codes:
//
//	0   - Success or help displayed
//	1   - Error (invalid args, API failure, etc.)
//	124 - Timeout exceeded
//	130 - Interrupted (Ctrl+C)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	edeka "github.com/ByteSizedMarius/edekompile/pkg"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	authFile := flag.String("auth", "", "Path to the auth file")
	tokenID := flag.String("token-id", "", "API token ID")
	tokenSecret := flag.String("token-secret", "", "API token secret")
	jsonOutput := flag.Bool("json", false, "Output in JSON format")
	timeout := flag.Duration("timeout", 0, "Max time the command may run (e.g. 30s, 5m). No limit by default.")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Usage = func() { printMainHelp(os.Stderr) }
	flag.Parse()

	if *versionFlag {
		fmt.Println(version)
		return
	}

	if flag.NArg() == 0 {
		printMainHelp(os.Stdout)
		os.Exit(0)
	}

	ctx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()
	if *timeout > 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, *timeout)
		defer cancelTimeout()
	}

	var data any
	var err error

	// authFn defers auth until a handler actually needs it, so missing-flag
	// and unknown-subcommand errors can exit without paying for auth.
	// os.Exit semantics match the previous authenticateOrDie contract.
	authFn := func() *edeka.Edeka {
		return authenticateOrDie(ctx, *authFile, *tokenID, *tokenSecret)
	}

	switch flag.Arg(0) {
	case "login":
		if *tokenID != "" || *tokenSecret != "" {
			fmt.Fprintln(os.Stderr, "Error: login exchanges a bearer; credential flags (-token-id, -token-secret) are not applicable.")
			os.Exit(1)
		}
		err = handleLogin(ctx, flag.Args()[1:], *authFile)
	case "receipts":
		data, err = handleReceipts(ctx, authFn, flag.Args()[1:])
	case "markets":
		data, err = handleMarkets(ctx, authFn, flag.Args()[1:])
	case "offers":
		if *authFile != "" || *tokenID != "" || *tokenSecret != "" {
			fmt.Fprintln(os.Stderr, "Error: offers uses app-level auth; -auth, -token-id, and -token-secret are not used by this command.")
			os.Exit(1)
		}
		data, err = handleOffers(ctx, flag.Args()[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", flag.Arg(0))
		printMainHelp(os.Stderr)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		switch {
		case errors.Is(err, context.Canceled):
			os.Exit(130)
		case errors.Is(err, context.DeadlineExceeded):
			os.Exit(124)
		default:
			os.Exit(1)
		}
	}

	if data == nil {
		return
	}

	if *jsonOutput {
		bt, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: marshaling output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(bt))
	} else {
		switch d := data.(type) {
		case []edeka.ReceiptParsed:
			for _, r := range d {
				fmt.Println(r)
			}
		case []edeka.Market:
			for _, m := range d {
				fmt.Println(m)
			}
		case *edeka.OfferResponse:
			fmt.Println(d)
		default:
			fmt.Println(d)
		}
	}
}

func authenticateOrDie(ctx context.Context, authFile, tokenID, tokenSecret string) *edeka.Edeka {
	ed, err := authenticate(ctx, authFile, tokenID, tokenSecret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Authentication failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "If you have credentials, use -auth flag with the file or place edeka_auth.json in the current directory.")
		fmt.Fprintln(os.Stderr, "First-time setup: obtain a bearer token, then run:")
		fmt.Fprintf(os.Stderr, "  %s login -bearer <token>\n", binaryName)
		fmt.Fprintln(os.Stderr, "Small CLI that helps in obtaining the bearer: https://github.com/ByteSizedMarius/edeka-auth-helper")
		os.Exit(1)
	}
	return ed
}

func authenticate(ctx context.Context, authFile, tokenID, tokenSecret string) (*edeka.Edeka, error) {
	if (tokenID == "") != (tokenSecret == "") {
		return nil, fmt.Errorf("both -token-id and -token-secret must be provided together")
	}
	haveTokens := tokenID != "" && tokenSecret != ""
	switch {
	case haveTokens && authFile != "":
		// Reuse the DeviceConfig from the auth file so we don't register a
		// fresh random device on the account each run - same concern
		// RefreshCredentialsFromBearer flags in its docstring.
		var a edeka.AuthFile
		if err := a.LoadFromFilepath(authFile); err != nil {
			return nil, err
		}
		device := a.DeviceConfig
		return edeka.LoginWithCredentialsCtx(ctx, tokenID, tokenSecret, &device, nil)
	case haveTokens:
		return edeka.LoginWithCredentialsCtx(ctx, tokenID, tokenSecret, nil, nil)
	case authFile != "":
		return edeka.LoginFromCustomAuthFileCtx(ctx, authFile, nil)
	default:
		return edeka.LoginFromAuthFileCtx(ctx, nil)
	}
}

func printMainHelp(w io.Writer) {
	fmt.Fprint(w, strings.ReplaceAll(`Usage: {bin} [flags] <command> [flags]

Flags:
  -auth <path>          Auth file path (default: edeka_auth.json in CWD)
  -token-id <id>        API token ID
  -token-secret <sec>   API token secret
  -json                 Output as JSON
  -timeout <duration>   Max time the command may run (e.g. 30s, 5m). No limit by default.
  -version              Print version and exit

Commands:
  login           Exchange a manually retrieved bearer token for API credentials (first-time setup)
                  A small CLI tool for obtaining a bearer: https://github.com/ByteSizedMarius/edeka-auth-helper
  receipts        List and get receipt details (requires auth)
  markets         Search for Edeka stores (requires auth)
  offers          Get current offers for a market (no auth needed)

Examples:
  {bin} login -bearer <token>
  {bin} receipts list
  {bin} receipts get -id 12345
  {bin} markets search -query Berlin -limit 5
  {bin} offers -market <gln>    # use the GLN from 'markets search' output

Run '{bin} <command>' for subcommand help.
`, "{bin}", binaryName))
}
