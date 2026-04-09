package edeka

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// OfferDate wraps time.Time with JSON marshaling that uses time.DateOnly
// (ISO YYYY-MM-DD) on the wire and renders as German DD.MM.YYYY via
// String(). Narrowing parsing to a single layout makes API drift visible
// as an error instead of silently absorbing a new format. The zero value
// marshals to the empty string and IsEmpty() returns true.
type OfferDate struct {
	time.Time
}

func (d *OfferDate) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		// Reset to zero so reusing the receiver doesn't carry forward a
		// previously-parsed value when the server later emits an empty date.
		d.Time = time.Time{}
		return nil
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return fmt.Errorf("failed to parse offer date %q (expected layout %q): %w", s, time.DateOnly, err)
	}
	d.Time = t
	return nil
}

func (d OfferDate) MarshalJSON() ([]byte, error) {
	if d.Time.IsZero() {
		return json.Marshal("")
	}
	return json.Marshal(d.Time.Format(time.DateOnly))
}

func (d OfferDate) String() string {
	if d.Time.IsZero() {
		return ""
	}
	return d.Time.Format("02.01.2006")
}

// IsEmpty reports whether the date is the zero value (absent or unparseable from the API).
func (d OfferDate) IsEmpty() bool {
	return d.Time.IsZero()
}

// OfferAvailability describes the day-of-week window (From..To, inclusive)
// during which an offer applies inside its broader ValidFrom/ValidTill period.
// From == To means a single day.
//
// Populated client-side by GetOffers from German day-range prefixes in
// Offer.Title (e.g. "Donnerstag bis Samstag:", "Samstags-Knüller:"). Nil
// when no prefix matched. Becomes stale if Title is mutated post-fetch.
type OfferAvailability struct {
	From time.Weekday
	To   time.Weekday
}

// String renders the availability as a short German label: "Nur Sa" for a
// single day, otherwise a "Mo-Sa"-style range.
func (a OfferAvailability) String() string {
	if a.From == a.To {
		return "Nur " + germanWeekdayShort[a.From]
	}
	return germanWeekdayShort[a.From] + "-" + germanWeekdayShort[a.To]
}

var germanWeekdays = map[string]time.Weekday{
	"montag":     time.Monday,
	"dienstag":   time.Tuesday,
	"mittwoch":   time.Wednesday,
	"donnerstag": time.Thursday,
	"freitag":    time.Friday,
	"samstag":    time.Saturday,
	"sonntag":    time.Sunday,
}

var germanWeekdayShort = map[time.Weekday]string{
	time.Monday:    "Mo",
	time.Tuesday:   "Di",
	time.Wednesday: "Mi",
	time.Thursday:  "Do",
	time.Friday:    "Fr",
	time.Saturday:  "Sa",
	time.Sunday:    "So",
}

// germanDayPattern is the shared alternation group reused by every
// availability regex that captures a day name.
const germanDayPattern = `montag|dienstag|mittwoch|donnerstag|freitag|samstag|sonntag`

// availabilityRule pairs a regex with a function that maps captures to an
// OfferAvailability. Rules are tried in order; the first match wins.
//
// To add a new prefix pattern, append one entry to availabilityRules:
//
//  1. Write a regex anchored at ^, allow a leading asterisk via \*?, mark
//     it case-insensitive with (?i), and terminate with a mandatory :\s*
//     so trailing whitespace is swallowed.
//  2. Capture day-name arguments with (germanDayPattern).
//  3. Provide a toAvail that builds OfferAvailability from caps.
//     caps[0] is the full match; caps[1..] are the submatches.
//
// Example - adding "Wochenend-Angebot:" → (Sa, So):
//
//	{
//	    re: regexp.MustCompile(`(?i)^\*?\s*wochenend-angebot\s*:\s*`),
//	    toAvail: func(_ []string) OfferAvailability {
//	        return OfferAvailability{From: time.Saturday, To: time.Sunday}
//	    },
//	},
type availabilityRule struct {
	re      *regexp.Regexp
	toAvail func(caps []string) OfferAvailability
}

var availabilityRules = []availabilityRule{
	// "Donnerstag bis Samstag:" → (day1, day2). Trailing s? tolerates
	// plural forms like "Donnerstags bis Samstags:".
	{
		re: regexp.MustCompile(`(?i)^\*?\s*(` + germanDayPattern + `)s?\s+bis\s+(` + germanDayPattern + `)s?\s*:\s*`),
		toAvail: func(caps []string) OfferAvailability {
			return OfferAvailability{
				From: germanWeekdays[strings.ToLower(caps[1])],
				To:   germanWeekdays[strings.ToLower(caps[2])],
			}
		},
	},
	// "Ab Montag erhältlich:" → (day, Saturday). Edeka's offer week ends
	// Saturday, so "ab X" means X..Sa. When X == Sa this collapses to
	// (Sa, Sa) which String() renders as "Nur Sa".
	{
		re: regexp.MustCompile(`(?i)^\*?\s*ab\s+(` + germanDayPattern + `)\s+erh[aä]ltlich\s*:\s*`),
		toAvail: func(caps []string) OfferAvailability {
			return OfferAvailability{
				From: germanWeekdays[strings.ToLower(caps[1])],
				To:   time.Saturday,
			}
		},
	},
	// "NUR AM DONNERSTAG:" → single day.
	{
		re: regexp.MustCompile(`(?i)^\*?\s*nur\s+am\s+(` + germanDayPattern + `)\s*:\s*`),
		toAvail: func(caps []string) OfferAvailability {
			d := germanWeekdays[strings.ToLower(caps[1])]
			return OfferAvailability{From: d, To: d}
		},
	},
	// "Samstags-Knüller:" and variants - tolerates leading *, trailing
	// footnote markers (², *, digit), and the real-world typo "Sasmstags".
	{
		re: regexp.MustCompile(`(?i)^\*?\s*sas?mstags-kn[uü]ller[²*\d]?\s*:\s*`),
		toAvail: func(_ []string) OfferAvailability {
			return OfferAvailability{From: time.Saturday, To: time.Saturday}
		},
	},
}

// parseTitleAvailability strips a German day-range prefix from title and
// returns the cleaned text plus the parsed window. No match returns the
// original title and nil.
func parseTitleAvailability(title string) (string, *OfferAvailability) {
	for _, rule := range availabilityRules {
		m := rule.re.FindStringSubmatch(title)
		if m == nil {
			continue
		}
		// m[0] is the full match; every regex is anchored at ^ so it always
		// starts at index 0, making len(m[0]) the right prefix-strip length.
		avail := rule.toAvail(m)
		return strings.TrimSpace(title[len(m[0]):]), &avail
	}
	return title, nil
}

// applyAvailabilityToOffers rewrites each offer's Title and populates its
// Availability via parseTitleAvailability. Called by getOffers after JSON
// unmarshal; split out from the fetch path so tests can exercise it in
// isolation without standing up an httptest server. Also collapses runs
// of whitespace in the title (the API occasionally emits doubles like
// "Mango  genussreif").
func applyAvailabilityToOffers(offers []Offer) {
	for i := range offers {
		title := strings.Join(strings.Fields(offers[i].Title), " ")
		title, avail := parseTitleAvailability(title)
		offers[i].Title = title
		offers[i].Availability = avail
	}
}

// OfferResponse is the top-level JSON response from the v2 offers endpoint.
type OfferResponse struct {
	Offers []Offer `json:"offers"`
	Offset *int    `json:"offset"`
	Limit  *int    `json:"limit"`
	// National is true when the offers are Edeka's nationwide marketing
	// (shown identically across all stores). False means the offers are
	// specific to the queried market's local promos.
	National   *bool     `json:"national"`
	TotalCount *int      `json:"totalCount"`
	Disclaimer string    `json:"disclaimer"`
	ValidFrom  OfferDate `json:"validFrom"`
	ValidTill  OfferDate `json:"validTill"`
}

// PriceType tags an offer's price-rendering intent. Values map to the
// three display modes the app produces when laying out an offer card.
type PriceType string

const (
	// PriceTypeShow: render the numeric price as-is.
	PriceTypeShow PriceType = "SHOW"
	// PriceTypeHide: suppress the numeric price entirely.
	PriceTypeHide PriceType = "HIDE"
	// PriceTypeLabel: show PriceLabel text instead of the numeric price.
	PriceTypeLabel PriceType = "LABEL"
)

// Offer represents a single offer/discount from an Edeka store.
type Offer struct {
	ID                    int                `json:"id"`
	Title                 string             `json:"title"`
	Descriptions          []string           `json:"descriptions"`
	Image                 OfferImage         `json:"image"`
	Price                 OfferPrice         `json:"price"`
	PriceType             PriceType          `json:"priceType"`
	PriceLabel            string             `json:"priceLabel"`
	Discount              string             `json:"discount"`
	Category              OfferCategory      `json:"category"`
	ValidFrom             OfferDate          `json:"validFrom"`
	ValidTill             OfferDate          `json:"validTill"`
	// National is true when this offer is part of Edeka's nationwide
	// marketing (shown across all stores). False marks it as specific
	// to the queried market's local promos.
	National              *bool              `json:"national"`
	Disclaimer            string             `json:"disclaimer"`
	Badges                []OfferBadge       `json:"badges"`
	Criteria              []OfferCriteria    `json:"criteria"`
	Ingredients           []OfferIngredient  `json:"ingredients"`
	Genussplus            string             `json:"genussplus"`
	LowestPrice           *float64           `json:"lowestPrice"`
	PointsDeutschlandcard *int               `json:"pointsDeutschlandcard"`
	PbPointsMultiplier    *int               `json:"pbPointsMultiplier"`
	PbAdditionalPoints    *int               `json:"pbAdditionalPoints"`
	PbPercentageDiscount  *int               `json:"pbPercentageDiscount"`
	PbPriceDiscount       *float64           `json:"pbPriceDiscount"`
	Availability          *OfferAvailability `json:"availability,omitempty"`
}

type OfferPrice struct {
	Value    string `json:"value"` // locale-formatted string (e.g. "1,99"), not float64
	Currency string `json:"currency"`
	Format   string `json:"format"`
}

// formatPriceValue strips a trailing all-zero fractional part so that
// "1.0", "1,0", "1.00", "1,00" all render as "1", while values with any
// non-zero fractional digit ("1,50", "1.99") pass through unchanged.
// Preserves whichever decimal separator the API emitted.
func formatPriceValue(s string) string {
	idx := strings.IndexAny(s, ".,")
	if idx < 0 {
		return s
	}
	frac := s[idx+1:]
	if frac == "" {
		return s[:idx]
	}
	for _, c := range frac {
		if c != '0' {
			return s
		}
	}
	return s[:idx]
}

type OfferImage struct {
	ImageParameter *bool  `json:"imageParameter"`
	ImageURL       string `json:"imageUrl"`
}

type OfferCategory struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type OfferBadge struct {
	Image OfferImage `json:"image"`
	Type  string     `json:"type"`
}

type OfferCriteria struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type OfferIngredient struct {
	Name string `json:"name"`
}

func (or OfferResponse) String() string {
	var sb strings.Builder

	// Header block: summary + metadata. Pagination fields are pointers because
	// the API omits them for national-wide result sets.
	shown := len(or.Offers)
	switch {
	case or.TotalCount == nil:
		fmt.Fprintf(&sb, "Offers: %d\n", shown)
	case shown >= *or.TotalCount:
		// Full set already in hand (single fetch or GetAllOffers); skip the
		// pagination breakdown and just report the count.
		fmt.Fprintf(&sb, "Offers: %d\n", shown)
	default:
		fmt.Fprintf(&sb, "Offers: %d shown, %d total", shown, *or.TotalCount)
		if or.Offset != nil && or.Limit != nil {
			fmt.Fprintf(&sb, " (offset %d, limit %d)", *or.Offset, *or.Limit)
		}
		sb.WriteByte('\n')
		if or.Offset != nil {
			nextOffset := *or.Offset + shown
			if nextOffset < *or.TotalCount {
				fmt.Fprintf(&sb, "More available: %d remaining. Request offset %d for the next batch.\n", *or.TotalCount-nextOffset, nextOffset)
			}
		}
	}
	if or.National != nil {
		scope := "market-specific"
		if *or.National {
			scope = "nationwide"
		}
		fmt.Fprintf(&sb, "Scope: %s\n", scope)
	}
	if !or.ValidFrom.IsEmpty() || !or.ValidTill.IsEmpty() {
		fmt.Fprintf(&sb, "Valid: %s - %s\n", or.ValidFrom, or.ValidTill)
	}
	if or.Disclaimer != "" {
		fmt.Fprintf(&sb, "Disclaimer: %s\n", or.Disclaimer)
	}

	for _, o := range or.Offers {
		sb.WriteString(o.String())
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (o Offer) String() string {
	var sb strings.Builder
	sb.WriteString("--------------------------------------\n")
	sb.WriteString(o.Title)
	if o.Availability != nil {
		fmt.Fprintf(&sb, " (%s)", o.Availability)
	}
	sb.WriteString("\n  ")
	sb.WriteString(strings.Join(o.Descriptions, "; "))
	fmt.Fprintf(&sb, "\n  Price: %s€", formatPriceValue(o.Price.Value))
	if o.Discount != "" {
		fmt.Fprintf(&sb, " (%s)", o.Discount)
	}
	return sb.String()
}
