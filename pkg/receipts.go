package edeka

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"time"
)

var ErrTemporaryCredentialsReceipts = errors.New("temporary credentials cannot be used to get receipts")

// receiptListData is the template input for the getAllReceiptsByApp SOAP call.
type receiptListData struct {
	Context mobileContext
	Limit   int
	Offset  int
}

// receiptDetailData is the template input for the getFormattedReceipt SOAP call.
type receiptDetailData struct {
	Context   mobileContext
	ReceiptID int
}

// ParseReceipts parses a list of raw receipts into their structured form.
func ParseReceipts(receipts []Receipt) ([]ReceiptParsed, error) {
	parsed := make([]ReceiptParsed, 0, len(receipts))
	for _, r := range receipts {
		p, err := r.Parse()
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, p)
	}
	return parsed, nil
}

func (e *Edeka) effectivePageSize() int {
	if e.pageSize > 0 {
		return e.pageSize
	}
	return DefaultReceiptPageSize
}

func (e *Edeka) requireFullCredentials() error {
	if e.temporaryCreds {
		return ErrTemporaryCredentialsReceipts
	}
	return nil
}

// ------------------------------------------------------------------------------
// Simple variants - use context.Background()
// ------------------------------------------------------------------------------

// GetReceipts retrieves a paginated list of receipts from the server.
// For control over context, use GetReceiptsCtx.
func (e *Edeka) GetReceipts(page int) ([]Receipt, error) {
	return e.GetReceiptsCtx(context.Background(), page)
}

// GetReceipt retrieves detailed information about a single receipt based on its ID.
// For control over context, use GetReceiptCtx.
func (e *Edeka) GetReceipt(receiptID int) (*ReceiptDetails, error) {
	return e.GetReceiptCtx(context.Background(), receiptID)
}

// GetAllReceipts retrieves all receipts using the iterator pattern.
// For control over context, use GetAllReceiptsCtx.
func (e *Edeka) GetAllReceipts() ([]Receipt, error) {
	return e.GetAllReceiptsCtx(context.Background())
}

// GetAllReceiptsUntil retrieves receipts until a specific receipt ID is found.
// For control over context, use GetAllReceiptsUntilCtx.
func (e *Edeka) GetAllReceiptsUntil(receiptID int) ([]Receipt, bool, error) {
	return e.GetAllReceiptsUntilCtx(context.Background(), receiptID)
}

// GetReceiptsWithIterator delegates receipt processing to the caller while handling pagination.
// For control over context, use GetReceiptsWithIteratorCtx.
func (e *Edeka) GetReceiptsWithIterator(iterator ReceiptIterator, callback func(page, count int)) error {
	return e.GetReceiptsWithIteratorCtx(context.Background(), iterator, callback)
}

// ------------------------------------------------------------------------------
// Ctx variants - explicit context
// ------------------------------------------------------------------------------

// GetAllReceiptsCtx retrieves all receipts using the iterator pattern.
// Returns nil on error. Callers needing access to partially-fetched pages
// should use GetReceiptsWithIteratorCtx directly and collect pages in the
// iterator callback.
func (e *Edeka) GetAllReceiptsCtx(ctx context.Context) ([]Receipt, error) {
	var rcs []Receipt
	err := e.GetReceiptsWithIteratorCtx(ctx,
		func(page []Receipt) (bool, error) {
			rcs = append(rcs, page...)
			return true, nil
		},
		nil,
	)
	if err != nil {
		return nil, err
	}
	return rcs, nil
}

// GetAllReceiptsUntilCtx retrieves receipts until a specific receipt ID is found.
// Returns the collected receipts (excluding the target) and whether the target
// ID was found. Returns (nil, false, err) on error; use
// GetReceiptsWithIteratorCtx for progressive collection.
func (e *Edeka) GetAllReceiptsUntilCtx(ctx context.Context, receiptID int) ([]Receipt, bool, error) {
	var rcs []Receipt
	var found bool
	err := e.GetReceiptsWithIteratorCtx(ctx,
		func(page []Receipt) (bool, error) {
			for _, rc := range page {
				if rc.ID == receiptID {
					found = true
					return false, nil
				}
				rcs = append(rcs, rc)
			}
			return true, nil
		},
		nil,
	)
	if err != nil {
		return nil, false, err
	}
	return rcs, found, nil
}

// GetReceiptsCtx retrieves a paginated list of receipts from the server.
// The page parameter is zero-based, meaning page=0 must be used for the first page.
// The app uses a page size of 50, which is what we will be using here as well.
func (e *Edeka) GetReceiptsCtx(ctx context.Context, page int) ([]Receipt, error) {
	if page < 0 {
		return nil, fmt.Errorf("page must be >= 0, got %d", page)
	}
	if err := e.requireFullCredentials(); err != nil {
		return nil, err
	}

	ps := e.effectivePageSize()
	return e.getReceipts(ctx, ps, page*ps)
}

// GetReceiptCtx retrieves detailed information about a single receipt based on its ID.
func (e *Edeka) GetReceiptCtx(ctx context.Context, receiptID int) (*ReceiptDetails, error) {
	if receiptID <= 0 {
		return nil, fmt.Errorf("receiptID must be > 0, got %d", receiptID)
	}
	if err := e.requireFullCredentials(); err != nil {
		return nil, err
	}

	return e.getFormattedReceipt(ctx, receiptID)
}

// ------------------------------------------------------------------------------
// Receipt-Iterator
// ------------------------------------------------------------------------------

// ReceiptIterator is a function type that processes receipts and determines whether to continue pagination.
type ReceiptIterator func([]Receipt) (continueIteration bool, err error)

// GetReceiptsWithIteratorCtx handles pagination logic common between GetAllReceipts and GetAllReceiptsUntil while delegating receipt processing.
// It may be used for custom receipt processing logic to allow for easier pagination.
// The inter-page delay is controlled via SetReceiptDelay (default 1500ms, zero disables).
// The optional callback runs after each page with (pageIndex, pageItemCount) in the caller goroutine.
func (e *Edeka) GetReceiptsWithIteratorCtx(ctx context.Context, iterator ReceiptIterator, callback func(page, count int)) error {
	if iterator == nil {
		return fmt.Errorf("iterator must not be nil")
	}
	if err := e.requireFullCredentials(); err != nil {
		return err
	}

	ps := e.effectivePageSize()
	page := 0
	for {
		rcsPage, err := e.getReceipts(ctx, ps, page*ps)
		if err != nil {
			return err
		}
		if len(rcsPage) == 0 {
			break
		}

		// Fire the progress callback before iterator runs so progress reporting
		// stays consistent: a page that was fetched should always be reported,
		// regardless of whether iterator later returns an error or signals stop.
		if callback != nil {
			callback(page, len(rcsPage))
		}

		continueIteration, err := iterator(rcsPage)
		if err != nil {
			return err
		}
		if !continueIteration {
			break
		}

		page++

		// Cancellable sleep between pages
		if delay := e.effectiveReceiptDelay(); delay > 0 {
			if err := sleepInterruptible(ctx, delay); err != nil {
				return err
			}
		}
	}

	return nil
}

// sleepInterruptible blocks for d or until ctx is cancelled, whichever comes
// first. Scoped to its own function so timer.Stop can be deferred without
// leaking across loop iterations in the pagination caller.
func sleepInterruptible(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		// Guard against Go's non-deterministic select choice: if ctx was
		// canceled at exactly the moment timer fired, the scheduler may
		// have picked this case. Check before returning success.
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ------------------------------------------------------------------------------
// Main Receipt Helpers
// ------------------------------------------------------------------------------

func (e *Edeka) getReceipts(ctx context.Context, limit, offset int) ([]Receipt, error) {
	reqData := receiptListData{
		Context: e.mobileContext(),
		Limit:   limit,
		Offset:  offset,
	}

	var response soapEnvelope[receiptListResponse]
	if err := e.callSOAP(ctx, e.endpoints.getReceipts, reqData, &response); err != nil {
		return nil, fmt.Errorf("getting receipts (offset %d, limit %d): %w", offset, limit, err)
	}
	return response.Body.Response.Data.Receipts, nil
}

func (e *Edeka) getFormattedReceipt(ctx context.Context, receiptID int) (*ReceiptDetails, error) {
	reqData := receiptDetailData{
		Context:   e.mobileContext(),
		ReceiptID: receiptID,
	}

	var response soapEnvelope[formattedReceiptResponse]
	if err := e.callSOAP(ctx, e.endpoints.getReceipt, reqData, &response); err != nil {
		return nil, fmt.Errorf("getting receipt %d: %w", receiptID, err)
	}
	receipt := &response.Body.Response.Data.Receipt

	// Parse the header XML which is embedded as a string
	var headerData ReceiptHeader
	if err := xml.Unmarshal([]byte(receipt.Header), &headerData); err != nil {
		return nil, fmt.Errorf("parsing receipt header %q: %w", truncateBody([]byte(receipt.Header)), err)
	}
	receipt.ParsedHeader = headerData

	return receipt, nil
}
