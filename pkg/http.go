package edeka

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
	"unicode/utf8"
)

// ------------------------------------------------------------------------------
// Client + Request Config
// ------------------------------------------------------------------------------

// HTTPDoer is the interface for executing HTTP requests.
// *http.Client satisfies this interface.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

const maxResponseSize = 10 << 20 // 10 MB

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

func resolveClient(c HTTPDoer) HTTPDoer {
	if c != nil {
		return c
	}
	return defaultHTTPClient
}

// requestConfig bundles everything needed for a single SOAP request:
// the template/action/URL from the mapper, config for headers, template
// data for XML body, optional credential override, and HTTP client.
type requestConfig struct {
	templateActionMapper
	config       Config
	templateData any
	credentials  *Credentials
	client       HTTPDoer
}

func addHeaders(req *http.Request, config Config, soapAction string) {
	req.Header.Set("User-Agent", fmt.Sprintf(uaKSOAP, config.PlatformVersion, config.Brand, config.AppVersion))
	req.Header.Set("SOAPAction", soapAction)
	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
	req.Header.Set("X-VPVersion", fmt.Sprintf("App version : %s SDK version : %s", config.AppVersion, sdkVersion))
	req.Header.Set("X-Application-Type", config.MobileApplication)
	req.Header.Set("Connection", "Keep-Alive")
}

// ------------------------------------------------------------------------------
// Response Validation
// ------------------------------------------------------------------------------

const maxErrorBodyLen = 512

// truncateBody returns body as a string, truncated near maxErrorBodyLen bytes
// for use in error messages. German responses routinely contain multi-byte
// runes (ä, ö, ü, €) - a naive byte slice at maxErrorBodyLen can land
// mid-rune and produce invalid UTF-8 that breaks downstream loggers or JSON
// encoders. Walk back up to 3 bytes to the nearest rune start.
func truncateBody(body []byte) string {
	if len(body) <= maxErrorBodyLen {
		return string(body)
	}
	end := maxErrorBodyLen
	// Max UTF-8 rune width is 4 bytes, so the cut is at most 3 bytes past
	// a valid start. Never walk past the whole cap - if no rune start is
	// found, fall back to the raw cut (will yield a U+FFFD, not a panic).
	for i := 0; i < 3 && end > 0 && !utf8.RuneStart(body[end]); i++ {
		end--
	}
	return string(body[:end]) + "..."
}

// checkHTTPStatus validates the HTTP status line shared by every endpoint:
// 429 becomes a RateLimitError; any other non-200 carries a truncated body
// slice in the diagnostic message. Returns nil for 200.
func checkHTTPStatus(statusCode int, retryAfter string, body []byte) error {
	if statusCode == http.StatusTooManyRequests {
		return newRateLimitError(retryAfter)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d: %s", statusCode, truncateBody(body))
	}
	return nil
}

// checkSOAPResponse runs checkHTTPStatus first, then probes an HTTP 200 body
// for an embedded SOAP Fault element. The fault probe lives here (not in the
// generic status check) because REST callers have no Envelope/Body shape.
func checkSOAPResponse(body []byte, statusCode int, retryAfter string) error {
	if err := checkHTTPStatus(statusCode, retryAfter, body); err != nil {
		return err
	}

	// Probe for a SOAP Fault. Success responses share the Envelope/Body shape
	// but have no Fault element, so Body.Fault stays nil. On unmarshal failure
	// we treat the body as non-SOAP and let the caller's own parser produce a
	// domain-specific error.
	var fault soapFaultEnvelope
	if err := xml.Unmarshal(body, &fault); err != nil {
		return nil
	}
	if fault.Body.Fault == nil {
		return nil
	}
	return &SOAPFaultError{
		Code:   fault.Body.Fault.Code,
		String: fault.Body.Fault.String,
	}
}

// ------------------------------------------------------------------------------
// SOAP Request Plumbing
// ------------------------------------------------------------------------------

// makeTemplatedRequest handles the common pattern of making SOAP requests to the edeka/valuephone API.
func makeTemplatedRequest(ctx context.Context, rc requestConfig) (body []byte, err error) {
	if ctx == nil {
		return nil, fmt.Errorf("%s: context must not be nil", rc.name)
	}

	var payload bytes.Buffer
	if err = rc.template.Execute(&payload, rc.templateData); err != nil {
		return nil, fmt.Errorf("%s: executing template: %w", rc.name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rc.baseURL, &payload)
	if err != nil {
		return nil, fmt.Errorf("%s: creating request: %w", rc.name, err)
	}

	if rc.credentials != nil {
		req.SetBasicAuth(rc.credentials.Username, rc.credentials.Password)
	}

	addHeaders(req, rc.config, rc.action)

	client := resolveClient(rc.client)

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: sending request: %w", rc.name, err)
	}
	defer closeWithWrap(res.Body, &err)

	body, err = readLimited(res.Body, maxResponseSize)
	if err != nil {
		return nil, fmt.Errorf("%s: reading response: %w", rc.name, err)
	}

	if err := checkSOAPResponse(body, res.StatusCode, res.Header.Get("Retry-After")); err != nil {
		return nil, fmt.Errorf("%s: %w", rc.name, err)
	}

	return body, nil
}

// callSOAP fires a SOAP request on behalf of *Edeka and, if out is non-nil,
// unmarshals the raw response body into it. Callers pass a pre-allocated
// *soapEnvelope[T] to read into, then unwrap through envelope.Body.Response.Data.
// Pass nil for out when the response payload is irrelevant (checkin-style calls).
func (e *Edeka) callSOAP(ctx context.Context, mapper templateActionMapper, data any, out any) error {
	return doSOAPCall(ctx, e.client, e.config, &e.creds, mapper, data, out)
}

// callSOAPWithCreds is callSOAP with an explicit credential override. Used by
// checkin/checkRegistration during Reauthenticate where the creds-to-validate
// are not yet committed to e.creds.
func (e *Edeka) callSOAPWithCreds(ctx context.Context, mapper templateActionMapper, data any, creds Credentials, out any) error {
	return doSOAPCall(ctx, e.client, e.config, &creds, mapper, data, out)
}

// doSOAPCall is the underlying primitive. Used by the *Edeka helpers above and
// by the free-function registerDevice path where no *Edeka exists yet.
// creds may be nil to skip Basic auth (anonymous registration).
func doSOAPCall(ctx context.Context, client HTTPDoer, config Config, creds *Credentials, mapper templateActionMapper, data any, out any) error {
	rc := requestConfig{
		templateActionMapper: mapper,
		config:               config,
		templateData:         data,
		credentials:          creds,
		client:               client,
	}
	body, err := makeTemplatedRequest(ctx, rc)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := xml.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: parsing response: %w", mapper.name, err)
	}
	return nil
}

// ------------------------------------------------------------------------------
// IO Helpers
// ------------------------------------------------------------------------------

// readLimited reads up to limit bytes from r. If the input would exceed limit,
// it returns an explicit overflow error instead of silently truncating - the
// plain io.LimitReader + io.ReadAll combo happily hands back a cut body that
// then fails downstream parsing with a cryptic "unexpected EOF".
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	lr := &io.LimitedReader{R: r, N: limit + 1}
	body, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if lr.N <= 0 {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return body, nil
}

// closeWithWrap closes f and, only when the primary *e is already non-nil,
// joins the close error onto it. After a successful read+parse, a close
// failure on an HTTP response body is typically connection-teardown jitter
// with no actionable signal - surfacing it would turn a fully-consumed
// response into a caller-visible failure. When the primary operation has
// already failed, the close error adds diagnostic context.
func closeWithWrap(f io.Closer, e *error) {
	if closeErr := f.Close(); closeErr != nil && *e != nil {
		*e = errors.Join(*e, closeErr)
	}
}
