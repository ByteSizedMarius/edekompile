package main

import (
	"context"
	"fmt"
	"strings"

	edeka "github.com/ByteSizedMarius/edekompile/pkg"
)

func handleOffers(ctx context.Context, args []string) (any, error) {
	if wantsHelp(args) {
		offersHelp()
		return nil, nil
	}
	fs := newFlagSet("offers")
	market := fs.String("market", "", "Market GLN")
	if ok, err := parseFlags(fs, args, offersHelp); !ok {
		return nil, err
	}
	if *market == "" {
		return nil, fmt.Errorf("flag -market is required")
	}

	return edeka.NewOffersClient(nil).GetAllOffersCtx(ctx, *market)
}

func offersHelp() {
	fmt.Print(strings.ReplaceAll(`Usage: {bin} offers [flags]

Fetches every offer for the given market.
The endpoint uses app-level OAuth and does not require user credentials.
Global auth flags (-auth, -token-id, -token-secret) are not needed and
will be rejected.

Flags:
  -market <gln>    Market GLN (13-digit identifier from 'markets search'; required)

Examples:
  {bin} offers -market 4314021182307
`, "{bin}", binaryName))
}
