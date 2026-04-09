package edeka

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// SOAPFaultError represents a SOAP fault returned by the API inside an HTTP 200 response.
type SOAPFaultError struct {
	Code   string
	String string
}

func (e *SOAPFaultError) Error() string {
	return fmt.Sprintf("SOAP fault: [%s] %s", e.Code, e.String)
}

// RateLimitError is returned when the API responds with HTTP 429.
type RateLimitError struct {
	RetryAfter    time.Duration
	RawRetryAfter string // original Retry-After header value, empty if absent
}

// Cap Retry-After at 24h. A server asking us to wait longer isn't useful,
// and unbounded values can overflow time.Duration.
const maxRetryAfterSeconds = 24 * 60 * 60

func newRateLimitError(header string) *RateLimitError {
	e := &RateLimitError{RawRetryAfter: header}
	if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
		if secs > maxRetryAfterSeconds {
			secs = maxRetryAfterSeconds
		}
		e.RetryAfter = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			if d > maxRetryAfterSeconds*time.Second {
				d = maxRetryAfterSeconds * time.Second
			}
			e.RetryAfter = d
		}
	}
	return e
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited - retry after %s", e.RetryAfter)
	}
	if e.RawRetryAfter != "" {
		return fmt.Sprintf("rate limited (unparsed Retry-After: %q)", e.RawRetryAfter)
	}
	return "rate limited"
}

// soapFaultEnvelope is the unmarshal target used to probe SOAP responses for
// a Fault element without knowing the full response shape.
type soapFaultEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		// Pointer so we can distinguish "no Fault element" (nil) from
		// "Fault element present but empty". A present-but-empty Fault
		// is still a server-side anomaly worth surfacing.
		Fault *struct {
			Code   string `xml:"faultcode"`
			String string `xml:"faultstring"`
		} `xml:"Fault"`
	} `xml:"Body"`
}
