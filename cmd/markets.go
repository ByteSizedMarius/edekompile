package main

import (
	"context"
	"fmt"
	"strings"
)

func handleMarkets(ctx context.Context, getAuth authFn, args []string) (any, error) {
	if wantsHelp(args) {
		marketsHelp()
		return nil, nil
	}

	switch args[0] {
	case "search":
		return handleMarketsSearch(ctx, getAuth, args[1:])
	default:
		return nil, fmt.Errorf("unknown markets subcommand: %s (valid: search)", args[0])
	}
}

func handleMarketsSearch(ctx context.Context, getAuth authFn, args []string) (any, error) {
	fs := newFlagSet("markets search")
	query := fs.String("query", "", "City name or zip code")
	limit := fs.Int("limit", 5, "Maximum results")
	offset := fs.Int("offset", 0, "Result offset")
	if ok, err := parseFlags(fs, args, marketsHelp); !ok {
		return nil, err
	}
	if *query == "" {
		return nil, fmt.Errorf("flag -query is required")
	}

	ed := getAuth()
	return ed.FindMarketsCtx(ctx, *query, *limit, *offset)
}

func marketsHelp() {
	fmt.Print(strings.ReplaceAll(`Usage: {bin} markets <subcommand> [flags]

Subcommands:
  search          Search for Edeka stores

Flags for 'search':
  -query <text>   City name or zip code (required)
  -limit <n>      Maximum results (default: 5)
  -offset <n>     Result offset (default: 0)

Each result includes a GLN - the identifier needed by 'offers -market <gln>'.

Examples:
  {bin} markets search -query Berlin -limit 5
  {bin} markets search -query 10117 -limit 5
`, "{bin}", binaryName))
}
