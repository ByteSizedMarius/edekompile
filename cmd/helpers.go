package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	edeka "github.com/ByteSizedMarius/edekompile/pkg"
)

// binaryName is the name the binary was invoked as. Used in help text so
// examples match whatever the user actually typed.
var binaryName = filepath.Base(os.Args[0])

// isHelpToken reports whether a single arg is one of the help sentinels.
// Covers every spelling Go's flag package treats as help (-h, -help, --help)
// plus the bare "help" subcommand word.
func isHelpToken(a string) bool {
	switch a {
	case "help", "-h", "-help", "--help":
		return true
	}
	return false
}

// wantsHelp checks if args request help (empty args or first arg is a help token).
func wantsHelp(args []string) bool {
	if len(args) == 0 {
		return true
	}
	return isHelpToken(args[0])
}

// authFn is a lazy auth provider passed to handlers. Handlers call it only
// after arg validation, so missing-flag errors exit without the cost of
// loading the auth file or verifying credentials. The implementation
// os.Exit's on auth failure (same contract as authenticateOrDie).
type authFn func() *edeka.Edeka

// checkUnexpectedArgs returns an error if there are leftover positional arguments
func checkUnexpectedArgs(fs *flag.FlagSet) error {
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument(s): %s", strings.Join(fs.Args(), " "))
	}
	return nil
}

// newFlagSet returns a FlagSet configured the way every subcommand needs it:
// errors are returned (not os.Exit'd) and the flag package's own error
// output is discarded so our custom help/usage stays the single source.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags runs fs.Parse on args, translates -h/-help/--help and a leading
// bare "help" token into a help() call, and rejects leftover positional
// arguments. Returns ok=false when the caller should stop (either help was
// printed or a parse error occurred). When err is nil but ok is false, help
// was printed and the handler should return nil.
func parseFlags(fs *flag.FlagSet, args []string, help func()) (ok bool, err error) {
	// Catch bare "help" before flag.Parse - it would otherwise bail out at
	// checkUnexpectedArgs with an "unexpected argument: help" error. Every
	// leaf handler funnels through here, so centralizing the check
	// removes the guards that used to sit at the top of each one.
	if len(args) > 0 && args[0] == "help" {
		help()
		return false, nil
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			help()
			return false, nil
		}
		return false, fmt.Errorf("parsing flags: %w", err)
	}
	if err := checkUnexpectedArgs(fs); err != nil {
		return false, err
	}
	return true, nil
}
