package edeka

import (
	"context"
	"fmt"
	"strings"
)

const defaultRetailerCountryID = 2650 // Edeka's internal ID for Germany in the Valuephone system

// marketSearchData is the template input for the findRetailerShops SOAP call.
type marketSearchData struct {
	Context           mobileContext
	RetailerCountryID int
	ZipOrPlace        string
	Limit             int
	Offset            int
}

// ------------------------------------------------------------------------------
// Simple variants - use context.Background()
// ------------------------------------------------------------------------------

// FindMarkets searches for Edeka stores by city name or zip code.
// For control over context, use FindMarketsCtx.
func (e *Edeka) FindMarkets(query string, limit, offset int) ([]Market, error) {
	return e.FindMarketsCtx(context.Background(), query, limit, offset)
}

// ------------------------------------------------------------------------------
// Ctx variants - explicit context
// ------------------------------------------------------------------------------

// FindMarketsCtx searches for Edeka stores by city name or zip code.
// The limit parameter controls the maximum number of results, offset enables pagination.
func (e *Edeka) FindMarketsCtx(ctx context.Context, query string, limit, offset int) ([]Market, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0, got %d", limit)
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0, got %d", offset)
	}

	reqData := marketSearchData{
		Context:           e.mobileContext(),
		RetailerCountryID: defaultRetailerCountryID,
		ZipOrPlace:        query,
		Limit:             limit,
		Offset:            offset,
	}

	var response soapEnvelope[marketSearchResponse]
	if err := e.callSOAP(ctx, e.endpoints.findMarkets, reqData, &response); err != nil {
		return nil, fmt.Errorf("searching markets for %q: %w", query, err)
	}
	return response.Body.Response.Markets, nil
}
