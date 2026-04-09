// Package edeka provides a Go client for the Edeka mobile API.
//
// # Authentication
//
// First-time setup requires a bearer token from a browser session at
// https://login.edeka/app. The library exchanges this for a long-lived
// API token pair that is saved to an auth file:
//
//	ed, err := edeka.CredentialsFromBearer(bearerToken)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := ed.SaveAuth(); err != nil {
//	    log.Fatal(err)
//	}
//
// On subsequent runs, load the saved token pair from the auth file:
//
//	ed, err := edeka.LoginFromAuthFile()
//
// If you have a token pair but no auth file, you can pass them directly:
//
//	ed, err := edeka.LoginWithCredentials(tokenID, tokenSecret, nil)
//
// Every public call that does I/O has a Ctx-suffixed variant (e.g.
// LoginFromAuthFileCtx, FindMarketsCtx, GetOffersCtx) that accepts a
// context.Context - and for free functions, an HTTPDoer - for full control
// over cancellation and HTTP transport. The unsuffixed forms shown here
// are the primary public API.
//
// # Receipts
//
// Retrieve digital receipts from Edeka stores:
//
//	receipts, _ := ed.GetReceipts(0)
//	details, _ := ed.GetReceipt(receiptID)
//	all, _ := ed.GetAllReceipts()
//
// # Markets
//
// Search for Edeka stores by city or zip code:
//
//	markets, _ := ed.FindMarkets("Mannheim", 30, 0)
//
// # Offers
//
// Get current offers for a specific market. The Edeka instance handles bearer auth automatically:
//
//	offers, _ := ed.GetOffers(marketGLN, 0, 50)
//
// For anonymous offers access (no user credentials needed), construct a minimal client:
//
//	offers, _ := edeka.NewOffersClient(nil).GetOffers(marketGLN, 0, 50)
package edeka
