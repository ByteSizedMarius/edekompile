package edeka

import (
	"encoding/xml"
	"fmt"
)

// Market represents a single Edeka store returned by the market search
type Market struct {
	ID                  int              `xml:"id"`
	Name                string           `xml:"name"`
	Street              string           `xml:"street"`
	Location            string           `xml:"location"`
	ZipCode             string           `xml:"zipCode"`
	Phone               string           `xml:"phone"`
	Fax                 string           `xml:"fax"`
	Website             string           `xml:"www"`
	OpeningHours        string           `xml:"openingHours"`
	RetailerShopGroup   string           `xml:"retailerShopGroup"`
	LogoImageID         int              `xml:"logoImageId"`
	Coordinate          MarketCoordinate `xml:"coordinate"`
	Capabilities        []Capability     `xml:"capabilities>capability"`
	LoyaltyEnabled      bool             `xml:"loyaltyEnabled"`
	PaymentEnabled      bool             `xml:"paymentEnabled"`
	SubsidiaryCountryID int              `xml:"subsidiaryCountryId"`
}

type MarketCoordinate struct {
	Latitude  float64 `xml:"latitude"`
	Longitude float64 `xml:"longitude"`
}

type Capability struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	Visible     bool   `xml:"visible"`
}

const capabilityILN = "ILN" // International Location Number - industry term for GLN

// GLN extracts the GLN (Global Location Number) from the market's capabilities list.
// The API stores it under the capability description "ILN".
func (m Market) GLN() string {
	for _, c := range m.Capabilities {
		if c.Description == capabilityILN {
			return c.Name
		}
	}
	return ""
}

func (m Market) String() string {
	return fmt.Sprintf(
		"--------------------------------------\n"+
			"%s\n"+
			"  %s\n"+
			"  %s %s\n"+
			"  GLN: %s\n"+
			"  Phone: %s\n"+
			"  Coordinates: %.6f, %.6f",
		m.Name,
		m.Street,
		m.ZipCode, m.Location,
		m.GLN(),
		m.Phone,
		m.Coordinate.Latitude, m.Coordinate.Longitude,
	)
}

type marketSearchResponse struct {
	XMLName xml.Name `xml:"findRetailerShopsMultipleCountriesResponse"`
	Markets []Market `xml:"retailerShop"`
}
