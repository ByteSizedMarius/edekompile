package edeka

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// receiptBaseRaw contains the fields shared between Receipt and ReceiptDetails.
type receiptBaseRaw struct {
	ID                int             `xml:"id"`
	Sum               float64         `xml:"sum"`
	SavedAmount       float64         `xml:"savedAmount"`
	PaidAmount        float64         `xml:"paidAmount"`
	Currency          Currency        `xml:"currency"`
	ShopName          string          `xml:"shopName"`
	ShopNativeID      int             `xml:"shopNativeId"`
	ShopLogoImageID   int             `xml:"shopLogoImageId"`
	ShopStreet        string          `xml:"shopStreet"`
	ShopCity          string          `xml:"shopCity"`
	ShopZipCode       string          `xml:"shopZipCode"`
	RetailerCountryID int             `xml:"retailerCountryId"`
	RetailerName      string          `xml:"retailerName"`
	IsPDFAvailable    bool            `xml:"isPdfAvailable"`
	ReceiptTenders    []ReceiptTender `xml:"receiptTenders>receiptTender"`
}

// Receipt represents a single receipt from the list endpoint. Items are not included.
type Receipt struct {
	receiptBaseRaw
	PhoneName   string `xml:"phoneName"`
	DateAndTime string `xml:"dateAndTime"`
}

// ReceiptDetails represents the detailed receipt information including line items.
type ReceiptDetails struct {
	receiptBaseRaw
	CancelTransactionID int  `xml:"receiptCancelTransactionId"`
	Canceled            bool `xml:"canceled"`
	// Header is the raw XML subtree (as a string) that the API embeds inside
	// the outer response. getFormattedReceipt unmarshals it into ParsedHeader
	// after the top-level parse; the raw string is kept as pass-through so
	// callers that need the original XML (diagnostics, alternative parsing)
	// can still reach it.
	Header       string        `xml:"header"`
	ParsedHeader ReceiptHeader `xml:"-"`
	Footer       string        `xml:"footer"`
	LineItems    []LineItem    `xml:"lineItems>item"`
}

// ------------------------------------------------------------------------------
// Parsers
// ------------------------------------------------------------------------------

// toBase converts the shared raw fields into a fully-populated receiptBase.
// phoneName and dateAndTime are passed separately because they come from
// different sources on each parent (Receipt.PhoneName vs
// ReceiptDetails.ParsedHeader.Phone; Receipt.DateAndTime vs
// ReceiptDetails.ParsedHeader.Date).
func (r receiptBaseRaw) toBase(phoneName string, dateAndTime time.Time) receiptBase {
	return receiptBase{
		ID: r.ID,
		Shop: ParsedShop{
			ShopNativeID:    r.ShopNativeID,
			ShopLogoImageID: r.ShopLogoImageID,
			Shop: Shop{
				StoreName: r.ShopName,
				Street:    r.ShopStreet,
				ZipCode:   r.ShopZipCode,
				City:      r.ShopCity,
			},
		},
		Retailer: Retailer{
			CountryID: r.RetailerCountryID,
			Name:      r.RetailerName,
		},
		PhoneName:      phoneName,
		DateAndTime:    dateAndTime,
		IsPDFAvailable: r.IsPDFAvailable,
	}
}

func (r receiptBaseRaw) toPaymentInfo() PaymentInformation {
	return PaymentInformation{
		Sum:                r.Sum,
		AmountPaidUsingApp: r.PaidAmount,
		Saved:              r.SavedAmount,
		Currency:           r.Currency,
		Tenders:            r.ReceiptTenders,
	}
}

func (r Receipt) Parse() (ReceiptParsed, error) {
	date, err := time.ParseInLocation("2006-01-02T15:04:05", r.DateAndTime, berlinTZ)
	if err != nil {
		return ReceiptParsed{}, fmt.Errorf("parsing receipt date %q: %w", r.DateAndTime, err)
	}
	return ReceiptParsed{
		receiptBase:        r.receiptBaseRaw.toBase(r.PhoneName, date),
		PaymentInformation: r.receiptBaseRaw.toPaymentInfo(),
	}, nil
}

func (rd ReceiptDetails) Parse() (ReceiptDetailsParsed, error) {
	items, err := parseReceiptItems(rd.LineItems)
	if err != nil {
		return ReceiptDetailsParsed{}, fmt.Errorf("parsing receipt items: %w", err)
	}

	date, err := time.ParseInLocation("Mon Jan 02 15:04:05 MST 2006", rd.ParsedHeader.Date, berlinTZ)
	if err != nil {
		return ReceiptDetailsParsed{}, fmt.Errorf("parsing receipt detail date %q: %w", rd.ParsedHeader.Date, err)
	}

	rp := ReceiptDetailsParsed{
		receiptBase: rd.receiptBaseRaw.toBase(rd.ParsedHeader.Phone, date),
		Items:       items,
		PaymentInformation: PaymentInformationDetails{
			PaymentInformation: rd.receiptBaseRaw.toPaymentInfo(),
			CouponsUsed:        rd.ParsedHeader.CouponsUsed,
			TenderCouponsUsed:  rd.ParsedHeader.TenderCouponsUsed,
			PaidWithValuephone: rd.ParsedHeader.PaidWithValuephone,
		},
		Canceled: rd.Canceled,
		Footer:   strings.TrimSuffix(rd.Footer, "\n"),
	}

	if rd.CancelTransactionID != 0 {
		rp.CancelTransactionID = &rd.CancelTransactionID
	}

	return rp, nil
}

// ------------------------------------------------------------------------------
// Helper Structs
// ------------------------------------------------------------------------------

// Currency represents the currency information in a receipt
type Currency struct {
	Symbol       string `xml:"symbol"`
	PositionLeft bool   `xml:"positionLeft"`
}

func (c Currency) String() string {
	return fmt.Sprintf("Symbol: %s", c.Symbol)
}

// ReceiptTender represents a payment tender in a receipt
type ReceiptTender struct {
	Amount     float64 `xml:"amount"`
	TenderType string  `xml:"tenderType"`
}

func (rt ReceiptTender) String() string {
	return fmt.Sprintf("%.2f %s", rt.Amount, rt.TenderType)
}

// Shop represents the store information in the header
type Shop struct {
	StoreName string `xml:"storeName"`
	Street    string `xml:"street"`
	ZipCode   string `xml:"zipCode"`
	City      string `xml:"city"`
}

func (s Shop) String() string {
	return fmt.Sprintf("%s, %s, %s %s", s.StoreName, s.Street, s.ZipCode, s.City)
}

// ReceiptHeader represents the parsed header XML content
type ReceiptHeader struct {
	RetailShop         Shop    `xml:"retailerShop"`
	ID                 string  `xml:"id"`
	Date               string  `xml:"date"`
	Total              float64 `xml:"total"`
	SavedAmount        float64 `xml:"savedAmount"`
	PaidAmount         float64 `xml:"paidAmount"`
	Currency           string  `xml:"currency"`
	Phone              string  `xml:"phone"`
	TransactionID      string  `xml:"retailerTransactionId"`
	IsCancellation     bool    `xml:"isCancelationReceipt"` // API uses single-L spelling
	CouponsUsed        bool    `xml:"couponsUsed"`
	TenderCouponsUsed  bool    `xml:"tenderCouponsUsed"`
	PaidWithValuephone bool    `xml:"paidWithValuephone"`
}

// LineItem represents a single item in the receipt
type LineItem struct {
	LineType  LineType `xml:"lineType"`
	LeftSide  string   `xml:"leftSide"`
	RightSide string   `xml:"rightSide"`
}

// LineType tags a receipt line's role. The server emits three values; the
// Android app additionally classifies lines as PAWN/POINTS client-side
// based on LeftSide content ("PFAND" / "Basis-Punkte"), which is derived
// rather than transmitted by the API.
type LineType string

const (
	// LineTypeNormal is a regular receipt line: items, quantity rows, and
	// PFAND deposit lines all share this type. Sub-classification happens
	// on LeftSide/RightSide content.
	LineTypeNormal LineType = "NORMAL"
	// LineTypeDiscount marks a discount line (promotional reductions).
	// The price on these is typically negative.
	LineTypeDiscount LineType = "DISCOUNT"
	// LineTypeSeparator marks a visual separator line with no semantic
	// payload - always skipped during parsing.
	LineTypeSeparator LineType = "SEPARATOR"
)

// ------------------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------------------

// itemParseState carries the running state of parseReceiptItems across its
// per-line loop. Quantity lines arrive before their target item, so the
// pending qty/price and mergeNext flag survive between iterations; Pfand
// lines attach to the previously-committed item.
type itemParseState struct {
	pendingQty   *float64
	pendingPrice *float64
	mergeNext    bool
	previousItem *ReceiptItem
	items        []*ReceiptItem
}

func parseReceiptItems(lineItems []LineItem) ([]ReceiptItem, error) {
	var state itemParseState
	for _, item := range lineItems {
		if err := state.consume(item); err != nil {
			return nil, err
		}
	}
	return mergeIdenticalItems(state.items), nil
}

// consume dispatches a single LineItem through the parse state machine.
func (s *itemParseState) consume(item LineItem) error {
	// Primary dispatch: LineType. Known values from the decompiled
	// app are NORMAL / DISCOUNT / SEPARATOR. Empty string is tolerated
	// for tests and legacy callers - treated as NORMAL. DISCOUNT lines
	// share the item-row layout (LeftSide = text, RightSide = signed
	// price), so the same logic handles both.
	switch item.LineType {
	case LineTypeSeparator:
		return nil
	case LineTypeNormal, LineTypeDiscount, "":
		// fall through
	default:
		return fmt.Errorf("unknown LineType %q (leftSide=%q, rightSide=%q)",
			item.LineType, item.LeftSide, item.RightSide)
	}

	if item.LeftSide == "" && item.RightSide == "" {
		return nil
	}

	// Left-only line: candidate quantity prefix for the next item.
	if item.LeftSide != "" && item.RightSide == "" {
		s.recordQuantityLine(item.LeftSide)
		return nil
	}

	// PFAND (deposit) attaches to the previous item.
	if item.LeftSide == "PFAND" && item.RightSide != "" {
		return s.attachPfand(item.RightSide)
	}

	return s.commitItem(item)
}

// recordQuantityLine parses a "2 x 1,50€"-style left-side and stashes the
// result for the next item to consume. Failed parses are silently ignored
// so an unrecognized left-only line doesn't clobber a valid pending merge.
func (s *itemParseState) recordQuantityLine(leftSide string) {
	if q, p := tryParseQuantityPrice(leftSide); q != nil && p != nil {
		s.pendingQty, s.pendingPrice = q, p
		s.mergeNext = true
	}
}

// attachPfand applies a deposit amount to the previously-committed item.
// No previous item (PFAND as first line) means the deposit has nothing to
// attach to, so it's dropped. The app derives the "PAWN" classification
// client-side from LeftSide content; the API emits these as NORMAL lines.
func (s *itemParseState) attachPfand(rightSide string) error {
	if s.previousItem == nil {
		return nil
	}
	pfand, err := parseEuroAmount(rightSide)
	if err != nil {
		return fmt.Errorf("parsing pfand amount %q: %w", rightSide, err)
	}
	s.previousItem.Pfand = &pfand
	return nil
}

// commitItem builds a ReceiptItem from the current line, folding in any
// pending quantity/unit-price that a prior quantity line stashed.
func (s *itemParseState) commitItem(item LineItem) error {
	i := &ReceiptItem{}
	if s.mergeNext {
		i.Merged = true
		i.Quantity = s.pendingQty
		i.UnitPrice = s.pendingPrice
		s.mergeNext = false
	}
	parsedPrice, err := parseEuroAmount(item.RightSide)
	if err != nil {
		return fmt.Errorf("parsing price %q for item %q: %w", item.RightSide, item.LeftSide, err)
	}
	i.rawPrice = normalizeEuroString(item.RightSide)
	i.Name = item.LeftSide
	i.Price = parsedPrice
	s.previousItem = i
	s.items = append(s.items, i)
	return nil
}

// mergeIdenticalItems combines items with the same name and price into a single
// entry with a quantity. Items that were already merged by the API (via quantity lines)
// are left as-is. Insertion order is preserved for deterministic output.
// Items with the same name are grouped together; original interleaving between
// different items is maintained, but duplicates collapse to their first occurrence's position.
//
// Mutates the input pointers in place: merge-targets gain mergedManually,
// Quantity, UnitPrice, Pfand, Merged, and a recomputed Price. Safe here
// because parseReceiptItems allocates and owns every *ReceiptItem it passes
// in - the mutation never leaks to a foreign caller.
func mergeIdenticalItems(items []*ReceiptItem) []ReceiptItem {
	itemMap := make(map[string][]*ReceiptItem)
	var keys []string

	for _, item := range items {
		existing, ok := itemMap[item.Name]
		if !ok {
			keys = append(keys, item.Name)
			itemMap[item.Name] = []*ReceiptItem{item}
			continue
		}
		if target := findMergeTarget(existing, item); target != nil {
			mergeInto(target, item)
			continue
		}
		itemMap[item.Name] = append(existing, item)
	}

	return collectMergedItems(keys, itemMap)
}

// findMergeTarget returns a candidate in existing that can absorb item, or nil.
//
// API-merged items (item.Merged) carry a rawPrice that represents the *total*,
// not the per-unit price. Collapsing them against another entry via rawPrice
// would be semantically unreliable (matches could be coincidental equality of
// totals), so they're kept as separate entries.
func findMergeTarget(existing []*ReceiptItem, item *ReceiptItem) *ReceiptItem {
	if item.Merged {
		return nil
	}
	for _, candidate := range existing {
		if candidate.Merged {
			continue
		}
		// Normalized string comparison - both sides pass through
		// normalizeEuroString on capture, so equal raw inputs (modulo
		// whitespace/suffix) collapse without needing a float tolerance.
		if candidate.rawPrice == item.rawPrice {
			return candidate
		}
	}
	return nil
}

// mergeInto absorbs source into target: bumps quantity, captures unit price on
// first merge, and accumulates pfand. Pfand values are copied rather than
// pointer-aliased so later mutation of either item can't silently desync.
func mergeInto(target, source *ReceiptItem) {
	target.mergedManually = true
	if target.UnitPrice == nil {
		up := target.Price
		target.UnitPrice = &up
	}
	if target.Quantity == nil {
		q := 2.0
		target.Quantity = &q
	} else {
		qt := *target.Quantity + 1
		target.Quantity = &qt
	}
	if source.Pfand != nil {
		newPfand := *source.Pfand
		if target.Pfand != nil {
			newPfand += *target.Pfand
		}
		target.Pfand = &newPfand
	}
}

// collectMergedItems flattens the per-name buckets in insertion order,
// recomputing the total price for each item that was merged in-place.
func collectMergedItems(keys []string, itemMap map[string][]*ReceiptItem) []ReceiptItem {
	var result []ReceiptItem
	for _, key := range keys {
		for _, item := range itemMap[key] {
			if item.mergedManually {
				item.Merged = true
				item.Price = math.Round((*item.Quantity)*(*item.UnitPrice)*100) / 100
			}
			result = append(result, *item)
		}
	}
	return result
}

func tryParseQuantityPrice(s string) (*float64, *float64) {
	parts := strings.SplitN(s, " x ", 2)
	if len(parts) != 2 {
		return nil, nil
	}

	q, err := parseGermanDecimal(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, nil
	}

	priceStr := strings.TrimSpace(parts[1])
	priceStr = strings.TrimSuffix(priceStr, "€")
	p, err := parseGermanDecimal(priceStr)
	if err != nil {
		return nil, nil
	}

	return &q, &p
}

// normalizeEuroString applies the trim/suffix-strip pass that downstream
// parsing and rawPrice comparison both rely on. Sharing the helper keeps
// stored rawPrice values bit-comparable with freshly normalized input,
// so whitespace or suffix variations can't fragment otherwise-mergeable
// line items.
func normalizeEuroString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "€")
	return strings.TrimSpace(s)
}

func parseEuroAmount(s string) (float64, error) {
	return parseGermanDecimal(normalizeEuroString(s))
}

// parseGermanDecimal parses "1.234,56" (DE locale) into 1234.56. Dots are
// thousand separators, comma is the decimal point. Edeka returns all prices
// in this format.
func parseGermanDecimal(s string) (float64, error) {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}
