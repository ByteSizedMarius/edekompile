package edeka

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OAuth client credentials for the offers API. These are embedded in the
// public APK and are not secret - they authenticate the app, not the user.
const (
	offersClientID     = "edeka-app-android-client"
	offersClientSecret = "wFoBG7VhAIn48kx9SDyVhaN9FttvPZi2"
)

// Cap offers bearer lifetime at 7 days. Defensive bound on pathologically
// large expires_in values from the server - the real token lifetime sits
// around an hour, so anything past a week is almost certainly nonsense we
// shouldn't cache across restarts.
const maxOffersExpirySeconds = 7 * 24 * 60 * 60

// bearerRefreshBuffer is the safety window before expiry at which we
// proactively refresh. Stops a request from racing the cached token
// into expiry server-side between the cache check and the HTTP call.
const bearerRefreshBuffer = 60 * time.Second

// Floor offers bearer lifetime at 2x the refresh buffer. Defensive bound
// on pathologically small expires_in values - anything shorter than the
// refresh buffer would force a refetch on every GetOffers call, turning
// the cache into a per-request token-endpoint hammer.
const minOffersExpirySeconds = 2 * 60

// OffersBearerResult contains the access token and its lifetime from the offers OAuth endpoint.
type OffersBearerResult struct {
	AccessToken string
	ExpiresIn   int
}

// OffersClient provides anonymous access to the offers REST API. It holds
// the HTTP client, endpoint URLs, and the cached app-level bearer token.
//
// *Edeka embeds OffersClient by value and inherits GetOffers automatically,
// so callers with full credentials don't need a second client. Use
// NewOffersClient when no user login exists.
//
// GetOffers (and the underlying bearer refresh) is safe to call from
// multiple goroutines on the same instance. Configuration mutators
// (WithClient, SetReceiptPageSize, SetReceiptDelay, Reauthenticate) are
// not - callers must synchronize those with in-flight requests externally.
type OffersClient struct {
	// client is the HTTP client used for all requests. If nil, a default
	// client with a 30-second timeout is used. Must not be swapped
	// concurrently with in-flight requests.
	client    HTTPDoer
	endpoints endpoints

	// bearerMu guards offersBearer + offersBearerExpiry. Held only across
	// the cache check and assignment; the HTTP fetch itself runs unlocked
	// and competing goroutines re-check the cache after reacquiring.
	bearerMu           sync.Mutex
	offersBearer       string
	offersBearerExpiry time.Time
}

// NewOffersClient returns a client configured for anonymous offers API access.
// The offers API does not require user credentials - only an app-level OAuth
// token fetched via client_credentials on the first GetOffers call.
// A nil client uses a default client with a 30-second timeout.
func NewOffersClient(client HTTPDoer) *OffersClient {
	return &OffersClient{
		endpoints: defaultEndpoints,
		client:    client,
	}
}

func (o *OffersClient) httpClient() HTTPDoer {
	return resolveClient(o.client)
}

// GetOffers retrieves current offers for a specific market.
// For control over context, use GetOffersCtx.
func (o *OffersClient) GetOffers(marketGLN string, page, size int) (*OfferResponse, error) {
	return o.GetOffersCtx(context.Background(), marketGLN, page, size)
}

// GetAllOffers paginates through every offer for a market and merges
// pages into a single OfferResponse.
// For control over context, use GetAllOffersCtx.
func (o *OffersClient) GetAllOffers(marketGLN string) (*OfferResponse, error) {
	return o.GetAllOffersCtx(context.Background(), marketGLN)
}

// GetAllOffersCtx paginates through every offer for a market. Page
// size is fixed at 200 to keep request counts low. Header metadata
// (ValidFrom, ValidTill, Disclaimer, National) comes from the first
// page; Offset/Limit are rewritten to reflect the merged view
// (offset 0, limit = TotalCount).
func (o *OffersClient) GetAllOffersCtx(ctx context.Context, marketGLN string) (*OfferResponse, error) {
	const batchSize = 200
	var combined *OfferResponse
	page := 0
	for {
		resp, err := o.GetOffersCtx(ctx, marketGLN, page, batchSize)
		if err != nil {
			return nil, err
		}
		if combined == nil {
			combined = resp
		} else {
			combined.Offers = append(combined.Offers, resp.Offers...)
		}
		// Stop when TotalCount is absent (API omits it for national-wide
		// sets, which are inherently unpaginated), when we've collected
		// everything, or when the current page came back short.
		if resp.TotalCount == nil || len(combined.Offers) >= *resp.TotalCount || len(resp.Offers) < batchSize {
			break
		}
		page++
	}
	if combined != nil && combined.TotalCount != nil {
		zero := 0
		lim := *combined.TotalCount
		combined.Offset = &zero
		combined.Limit = &lim
	}
	return combined, nil
}

// GetOffersCtx retrieves current offers for a specific market.
// The bearer token for the offers API is fetched automatically via client_credentials
// and cached on the client until expiry.
// Results are always sorted by category (the API does not support unsorted results).
func (o *OffersClient) GetOffersCtx(ctx context.Context, marketGLN string, page, size int) (*OfferResponse, error) {
	if err := validateOffersArgs(marketGLN, page, size); err != nil {
		return nil, err
	}
	bearer, err := o.ensureOffersBearer(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtaining offers bearer: %w", err)
	}
	return getOffers(offersRequest{
		ctx:       ctx,
		client:    o.httpClient(),
		bearer:    bearer,
		baseURL:   o.endpoints.offers.dataURL,
		marketGLN: marketGLN,
		page:      page,
		size:      size,
	})
}

// GetOfferImage downloads an offer image by its URL, reusing the client's stored HTTPDoer.
// For control over context, use GetOfferImageCtx.
func (o *OffersClient) GetOfferImage(imageURL string) ([]byte, error) {
	return o.GetOfferImageCtx(context.Background(), imageURL)
}

// GetOfferImageCtx downloads an offer image by its URL, reusing the client's stored HTTPDoer.
// The imageURL is typically obtained from Offer.Image.ImageURL; only the scheme is
// validated (http:// or https:// required). Callers remain responsible for trusting
// the host and path.
//
// Named return required for closeWithWrap to attach close errors.
func (o *OffersClient) GetOfferImageCtx(ctx context.Context, imageURL string) (data []byte, err error) {
	if !strings.HasPrefix(imageURL, "https://") && !strings.HasPrefix(imageURL, "http://") {
		return nil, fmt.Errorf("imageURL must use http(s) scheme, got %q", imageURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating image request: %w", err)
	}

	req.Header.Set("User-Agent", uaDalvik)
	req.Header.Set("Connection", "Keep-Alive")

	res, err := o.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching offer image: %w", err)
	}
	defer closeWithWrap(res.Body, &err)

	// For non-2xx responses we still need the body for the error message; fall
	// through to the standard read + status-check path. For 2xx, validate the
	// content-type up front so a server mis-route doesn't cost us a 10 MB body
	// download. Empty Content-Type is tolerated - the CDN has been observed to
	// omit it for some responses. RFC 9110 media types are case-insensitive,
	// so lowercase before comparison.
	if res.StatusCode == http.StatusOK {
		if ct := res.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "image/") {
			return nil, fmt.Errorf("offer image %q: unexpected content type %q", imageURL, ct)
		}
	}

	data, err = readLimited(res.Body, maxResponseSize)
	if err != nil {
		return nil, fmt.Errorf("reading offer image: %w", err)
	}

	if err := checkHTTPStatus(res.StatusCode, res.Header.Get("Retry-After"), data); err != nil {
		return nil, fmt.Errorf("offer image %q: %w", imageURL, err)
	}

	return data, nil
}

// offersRequest bundles the per-call inputs for the offers REST endpoint.
type offersRequest struct {
	ctx       context.Context
	client    HTTPDoer
	bearer    string
	baseURL   string
	marketGLN string
	page      int
	size      int
}

func validateOffersArgs(marketGLN string, page, size int) error {
	if marketGLN == "" {
		return fmt.Errorf("marketGLN must not be empty")
	}
	if size <= 0 {
		return fmt.Errorf("size must be > 0, got %d", size)
	}
	if page < 0 {
		return fmt.Errorf("page must be >= 0, got %d", page)
	}
	return nil
}

// ------------------------------------------------------------------------------
// Simple variants - use context.Background() and a default client with a 30-second timeout
// ------------------------------------------------------------------------------

// FetchOffersBearer obtains a bearer token for the offers API via client_credentials grant.
// For control over context and HTTP client, use FetchOffersBearerCtx.
func FetchOffersBearer() (*OffersBearerResult, error) {
	return FetchOffersBearerCtx(context.Background(), nil)
}

// ------------------------------------------------------------------------------
// Ctx variants - explicit context and HTTP client (nil client = default client with a 30-second timeout)
// ------------------------------------------------------------------------------

// FetchOffersBearerCtx obtains a bearer token for the offers API via client_credentials grant.
// The offers API uses a separate OAuth system (b2b-login.api.edeka) from the user login.
// A nil client uses a default client with a 30-second timeout.
func FetchOffersBearerCtx(ctx context.Context, client HTTPDoer) (*OffersBearerResult, error) {
	return fetchOffersBearer(ctx, resolveClient(client), defaultEndpoints.offers.tokenURL)
}

// ------------------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------------------

// ensureOffersBearer returns a valid bearer token, fetching and caching a
// fresh one if needed. Safe for concurrent use: multiple callers that miss
// the cache may each fetch a bearer, but only one result wins the
// write-back (via a second cache check under the lock). The returned
// token is whichever value wins the race - the caller always gets a
// usable bearer, and the cache keeps the one with the later expiry.
func (o *OffersClient) ensureOffersBearer(ctx context.Context) (string, error) {
	o.bearerMu.Lock()
	if o.offersBearer != "" && time.Until(o.offersBearerExpiry) > bearerRefreshBuffer {
		bearer := o.offersBearer
		o.bearerMu.Unlock()
		return bearer, nil
	}
	o.bearerMu.Unlock()

	// Fetch outside the lock so a slow network call doesn't block every
	// goroutine calling GetOffers. Concurrent refreshes are wasteful but
	// tolerable - the re-check below ensures only one updates the cache.
	result, err := fetchOffersBearer(ctx, o.httpClient(), o.endpoints.offers.tokenURL)
	if err != nil {
		return "", err
	}
	expiresIn := result.ExpiresIn
	if expiresIn > maxOffersExpirySeconds {
		expiresIn = maxOffersExpirySeconds
	}
	if expiresIn < minOffersExpirySeconds {
		expiresIn = minOffersExpirySeconds
	}

	o.bearerMu.Lock()
	defer o.bearerMu.Unlock()
	// Second check: a parallel fetch may have won. Keep whichever expiry
	// is later so we don't shorten the effective lifetime of a just-won
	// token by overwriting with our (slightly older) result.
	newExpiry := time.Now().Add(time.Duration(expiresIn) * time.Second)
	if o.offersBearer == "" || newExpiry.After(o.offersBearerExpiry) {
		o.offersBearer = result.AccessToken
		o.offersBearerExpiry = newExpiry
		return result.AccessToken, nil
	}
	return o.offersBearer, nil
}

// Named return required for closeWithWrap to attach close errors.
func getOffers(r offersRequest) (resp *OfferResponse, err error) {
	u, parseErr := url.Parse(r.baseURL)
	if parseErr != nil {
		return nil, fmt.Errorf("parsing offers base URL %q: %w", r.baseURL, parseErr)
	}
	q := url.Values{
		"marketGln": {r.marketGLN},
		"size":      {strconv.Itoa(r.size)},
		"page":      {strconv.Itoa(r.page)},
		// The API requires sortedByCategory=true; unsorted results are not supported.
		"sortedByCategory": {"true"},
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating offer request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.bearer)
	req.Header.Set("User-Agent", uaOkHTTP)
	req.Header.Set("Connection", "Keep-Alive")

	res, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching offers: %w", err)
	}
	defer closeWithWrap(res.Body, &err)

	body, err := readLimited(res.Body, maxResponseSize)
	if err != nil {
		return nil, fmt.Errorf("reading offer response: %w", err)
	}

	if err := checkHTTPStatus(res.StatusCode, res.Header.Get("Retry-After"), body); err != nil {
		return nil, fmt.Errorf("offer request: %w", err)
	}

	var response OfferResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parsing offer response: %w", err)
	}

	applyAvailabilityToOffers(response.Offers)

	return &response, nil
}

// Named return required for closeWithWrap to attach close errors.
func fetchOffersBearer(ctx context.Context, client HTTPDoer, tokenURL string) (result *OffersBearerResult, err error) {
	data := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(offersClientID, offersClientSecret)

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending token request: %w", err)
	}
	defer closeWithWrap(res.Body, &err)

	body, err := readLimited(res.Body, maxResponseSize)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if err := checkHTTPStatus(res.StatusCode, res.Header.Get("Retry-After"), body); err != nil {
		return nil, fmt.Errorf("offers token: %w", err)
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	if strings.TrimSpace(tr.AccessToken) == "" || tr.ExpiresIn <= 0 {
		return nil, fmt.Errorf("invalid offers token response (access_token=%q, expires_in=%d)", tr.AccessToken, tr.ExpiresIn)
	}

	return &OffersBearerResult{
		AccessToken: tr.AccessToken,
		ExpiresIn:   tr.ExpiresIn,
	}, nil
}
