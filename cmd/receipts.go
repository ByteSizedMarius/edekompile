package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	edeka "github.com/ByteSizedMarius/edekompile/pkg"
)

func handleReceipts(ctx context.Context, getAuth authFn, args []string) (any, error) {
	if wantsHelp(args) {
		receiptsHelp()
		return nil, nil
	}

	switch args[0] {
	case "list":
		return handleReceiptsList(ctx, getAuth, args[1:])
	case "get":
		return handleReceiptsGet(ctx, getAuth, args[1:])
	case "all":
		return handleReceiptsAll(ctx, getAuth, args[1:])
	default:
		return nil, fmt.Errorf("unknown receipts subcommand: %s (valid: list, get, all)", args[0])
	}
}

func handleReceiptsList(ctx context.Context, getAuth authFn, args []string) (any, error) {
	fs := newFlagSet("receipts list")
	page := fs.Int("page", 0, "Page number (zero-based)")
	if ok, err := parseFlags(fs, args, receiptsHelp); !ok {
		return nil, err
	}

	ed := getAuth()
	receipts, err := ed.GetReceiptsCtx(ctx, *page)
	if err != nil {
		return nil, err
	}

	parsed, err := edeka.ParseReceipts(receipts)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "\nPage %d: %d receipts.", *page, len(parsed))
	if len(receipts) >= edeka.DefaultReceiptPageSize {
		fmt.Fprintf(os.Stderr, " More may be available:\n  %s receipts list -page %d\n", binaryName, *page+1)
	} else {
		fmt.Fprintln(os.Stderr)
	}

	return parsed, nil
}

func handleReceiptsGet(ctx context.Context, getAuth authFn, args []string) (any, error) {
	fs := newFlagSet("receipts get")
	id := fs.Int("id", 0, "Receipt ID")
	if ok, err := parseFlags(fs, args, receiptsHelp); !ok {
		return nil, err
	}
	if *id == 0 {
		return nil, fmt.Errorf("flag -id is required")
	}

	ed := getAuth()
	detail, err := ed.GetReceiptCtx(ctx, *id)
	if err != nil {
		return nil, err
	}

	return detail.Parse()
}

func handleReceiptsAll(ctx context.Context, getAuth authFn, args []string) (any, error) {
	fs := newFlagSet("receipts all")
	if ok, err := parseFlags(fs, args, receiptsHelp); !ok {
		return nil, err
	}

	ed := getAuth()
	receipts, err := ed.GetAllReceiptsCtx(ctx)
	if err != nil {
		return nil, err
	}

	return edeka.ParseReceipts(receipts)
}

func receiptsHelp() {
	fmt.Print(strings.ReplaceAll(`Usage: {bin} receipts <subcommand> [flags]

Subcommands:
  list            List receipts (paginated)
  get             Get a single receipt by ID
  all             Get all receipts

Flags for 'list':
  -page <n>       Page number, zero-based (default: 0)

Flags for 'get':
  -id <id>        Receipt ID (required)

Examples:
  {bin} receipts list
  {bin} receipts list -page 2
  {bin} receipts get -id 12345
  {bin} receipts all
`, "{bin}", binaryName))
}
