package edeka

import "encoding/xml"

// mobileContext is a helper struct for data that is required by some SOAP endpoints.
type mobileContext struct {
	CountryCode  string
	LanguageCode string
}

// soapEnvelope is a generic envelope that wraps any response type.
type soapEnvelope[T any] struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		XMLName  xml.Name `xml:"Body"`
		Response T
	}
}

// loginResponse handles the SOAP double-nesting where the operation wrapper
// and the actual data response share the same element name: <loginResponse><loginResponse>...
type loginResponse struct {
	XMLName xml.Name `xml:"loginResponse"`
	Data    struct {
		XMLName        xml.Name `xml:"loginResponse"`
		Username       string   `xml:"username"`
		Password       string   `xml:"password"`
		RegistrationID string   `xml:"registrationId"`
		UserID         string   `xml:"userId"`
	}
}

type receiptListResponse struct {
	XMLName xml.Name `xml:"getAllReceiptsByAppResponse"`
	Data    struct {
		XMLName  xml.Name  `xml:"receiptResponse"`
		Receipts []Receipt `xml:"receipts>receipt"`
	}
}

type formattedReceiptResponse struct {
	XMLName xml.Name `xml:"getFormattedReceiptResponse"`
	Data    struct {
		XMLName xml.Name       `xml:"formattedReceiptResponse"`
		Receipt ReceiptDetails `xml:"formattedReceipt"`
	}
}

type registerAnonymousResponse struct {
	XMLName xml.Name    `xml:"registerMobileApplicationForAnonymousUserResponse"`
	Data    Credentials `xml:"registrationResponse"`
}

