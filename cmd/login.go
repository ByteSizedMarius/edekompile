package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	edeka "github.com/ByteSizedMarius/edekompile/pkg"
)

// handleLogin performs first-time setup: it trades an OAuth bearer token
// (copied from https://login.edeka/app devtools) for long-lived API
// credentials and writes them to disk. Any subsequent invocation of the
// CLI can then authenticate from the resulting auth file.
func handleLogin(ctx context.Context, args []string, authFile string) error {
	if wantsHelp(args) {
		loginHelp()
		return nil
	}

	fs := newFlagSet("login")
	bearer := fs.String("bearer", "", "OAuth bearer token from https://login.edeka/app")
	if ok, err := parseFlags(fs, args, loginHelp); !ok {
		return err
	}
	if *bearer == "" {
		return fmt.Errorf("flag -bearer is required")
	}

	// Reuse the device config from an existing auth file to avoid accumulating
	// phantom device registrations on the account during re-login.
	var device *edeka.DeviceConfig
	if authFile != "" {
		var a edeka.AuthFile
		if err := a.LoadFromFilepath(authFile); err == nil {
			dc := a.DeviceConfig
			device = &dc
		}
	}

	ed, err := edeka.RefreshCredentialsFromBearerCtx(ctx, *bearer, device, nil)
	if err != nil {
		return err
	}

	if authFile != "" {
		if err := ed.SaveAuthTo(authFile); err != nil {
			return err
		}
		fmt.Printf("Saved credentials to %s\n", authFile)
		return nil
	}
	if err := ed.SaveAuth(); err != nil {
		return err
	}
	// Resolve to an absolute path so users running `cd`-chained shells can
	// see exactly where the file landed. Fall back to the bare filename if
	// Getwd fails (shouldn't in practice, but no reason to panic if it does).
	if cwd, wdErr := os.Getwd(); wdErr == nil {
		fmt.Printf("Saved credentials to %s\n", filepath.Join(cwd, edeka.DefaultAuthFileName))
	} else {
		fmt.Printf("Saved credentials to %s in the current directory\n", edeka.DefaultAuthFileName)
	}
	return nil
}

func loginHelp() {
	fmt.Print(strings.ReplaceAll(`Usage: {bin} [-auth <path>] login -bearer <token>

Exchanges an OAuth bearer token for API credentials and writes them to disk.
The bearer is single-use; run this once per device configuration. Use the
global -auth flag to choose where credentials are written (defaults to
edeka_auth.json in the current directory).

Flags:
  -bearer <token>  OAuth bearer token from https://login.edeka/app (required)

Examples:
  {bin} login -bearer eyJhbG...
  {bin} -auth /etc/edeka/auth.json login -bearer eyJhbG...
`, "{bin}", binaryName))
}
