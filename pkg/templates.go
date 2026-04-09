package edeka

import (
	"bytes"
	"encoding/xml"
	"text/template"
)

// templateActionMapper bundles a pre-parsed SOAP template with the SOAP action and endpoint URL.
type templateActionMapper struct {
	name     string
	template *template.Template
	action   string
	baseURL  string
}

var templateFuncs = template.FuncMap{"escape": xmlEscape}

func mustParseTemplate(name, text string) *template.Template {
	return template.Must(template.New(name).Funcs(templateFuncs).Parse(text))
}

// xmlEscape escapes special XML characters in a string to prevent XML injection in SOAP templates.
// Exposed to every template via the "escape" function - never bypass it for a string field.
func xmlEscape(s string) string {
	var buf bytes.Buffer
	// xml.EscapeText only errors on writer failure; bytes.Buffer.Write can't fail.
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// All string fields in SOAP templates MUST use {{escape .Field}} to prevent XML injection.
// Integer/bool fields are safe without escaping. If adding a new template, verify all
// user-controlled string data passes through the escape function.

const registerAnonTemplate = `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance"
            xmlns:d="http://www.w3.org/2001/XMLSchema"
            xmlns:c="http://schemas.xmlsoap.org/soap/encoding/"
            xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
    <v:Header />
    <v:Body>
        <n0:registerMobileApplicationForAnonymousUser xmlns:n0="http://registration.open.mobile.server.valuephone.com/">
            <registrationRequest>
                <mobileApplication>{{escape .MobileApplication}}</mobileApplication>
                <brand>{{escape .Brand}}</brand>
                <applicationVersion>{{escape .AppVersion}}</applicationVersion>
                <platform>{{escape .Platform}}</platform>
                <platformVersion>{{escape .PlatformVersion}}</platformVersion>
                <imei>{{escape .DeviceID}}</imei>
                <serialNumber>{{escape .DeviceID}}</serialNumber>
                <manufacturer>{{escape .Manufacturer}}</manufacturer>
                <model>{{escape .Model}}</model>
                <countryAlpha2>{{escape .CountryCode}}</countryAlpha2>
                <languageAlpha2>{{escape .LanguageCode}}</languageAlpha2>
            </registrationRequest>
        </n0:registerMobileApplicationForAnonymousUser>
    </v:Body>
</v:Envelope>`

const checkinTemplate = `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance"
            xmlns:d="http://www.w3.org/2001/XMLSchema"
            xmlns:c="http://schemas.xmlsoap.org/soap/encoding/"
            xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
    <v:Header />
    <v:Body>
        <n0:notifyPostLogin xmlns:n0="http://user.server.valuephone.com/">
            <countryAlpha2>{{escape .CountryCode}}</countryAlpha2>
            <languageAlpha2>{{escape .LanguageCode}}</languageAlpha2>
            <currentApplicationVersion>{{escape .AppVersion}}</currentApplicationVersion>
            <currentPlatform>{{escape .Platform}}</currentPlatform>
            <currentPlatformVersion>{{escape .PlatformVersion}}</currentPlatformVersion>
        </n0:notifyPostLogin>
    </v:Body>
</v:Envelope>`

const bearerToLoginTemplate = `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance" xmlns:d="http://www.w3.org/2001/XMLSchema" xmlns:c="http://schemas.xmlsoap.org/soap/encoding/" xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
    <v:Header/>
    <v:Body>
       <n0:login xmlns:n0="http://sso.mobile.vpserverapp.valuephone.com/">
          <mobileSSOContext>
             <brandId>{{escape .Brand}}</brandId>
             <accessToken>{{escape .AccessToken}}</accessToken>
          </mobileSSOContext>
          <mobileDeviceRegistrationRequest>
             <brand>{{escape .Brand}}</brand>
             <countryAlpha2>{{escape .CountryCode}}</countryAlpha2>
             <phoneName>{{escape .Model}}</phoneName>
             <serialNumber>{{escape .DeviceID}}</serialNumber>
          </mobileDeviceRegistrationRequest>
          <mobileApplicationRegistrationRequest>
             <mobileApplication>{{escape .MobileApplication}}</mobileApplication>
             <brand>{{escape .Brand}}</brand>
             <applicationVersion>{{escape .AppVersion}}</applicationVersion>
             <platform>{{escape .Platform}}</platform>
             <platformVersion>{{escape .PlatformVersion}}</platformVersion>
             <imei>{{escape .DeviceID}}</imei>
             <serialNumber>{{escape .DeviceID}}</serialNumber>
             <manufacturer>{{escape .Manufacturer}}</manufacturer>
             <model>{{escape .Model}}</model>
             <countryAlpha2>{{escape .CountryCode}}</countryAlpha2>
             <languageAlpha2>{{escape .LanguageCode}}</languageAlpha2>
          </mobileApplicationRegistrationRequest>
       </n0:login>
    </v:Body>
</v:Envelope>`

const checkRegistrationTemplate = `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance"
            xmlns:d="http://www.w3.org/2001/XMLSchema"
            xmlns:c="http://schemas.xmlsoap.org/soap/encoding/"
            xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
    <v:Header />
    <v:Body>
        <n0:checkMobileApplicationRegistrationLogin xmlns:n0="http://registration.open.mobile.server.valuephone.com/">
            <registrationUsername>{{escape .Username}}</registrationUsername>
            <registrationPassword>{{escape .Password}}</registrationPassword>
        </n0:checkMobileApplicationRegistrationLogin>
    </v:Body>
</v:Envelope>`

const getReceiptsTemplate = `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance"
            xmlns:d="http://www.w3.org/2001/XMLSchema"
            xmlns:c="http://schemas.xmlsoap.org/soap/encoding/"
            xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
    <v:Header />
    <v:Body>
        <n0:getAllReceiptsByApp xmlns:n0="http://receipt.mobile.server.valuephone.com/">
            <mobileContext>
                <countryAlpha2>{{escape .Context.CountryCode}}</countryAlpha2>
                <languageAlpha2>{{escape .Context.LanguageCode}}</languageAlpha2>
            </mobileContext>
            <limit>{{.Limit}}</limit>
            <offset>{{.Offset}}</offset>
        </n0:getAllReceiptsByApp>
    </v:Body>
</v:Envelope>`

const getReceiptTemplate = `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance"
           xmlns:d="http://www.w3.org/2001/XMLSchema"
           xmlns:c="http://schemas.xmlsoap.org/soap/encoding/"
           xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
   <v:Header />
   <v:Body>
       <n0:getFormattedReceipt xmlns:n0="http://receipt.mobile.server.valuephone.com/">
           <mobileContext>
               <countryAlpha2>{{escape .Context.CountryCode}}</countryAlpha2>
               <languageAlpha2>{{escape .Context.LanguageCode}}</languageAlpha2>
           </mobileContext>
           <receiptId>{{.ReceiptID}}</receiptId>
       </n0:getFormattedReceipt>
   </v:Body>
</v:Envelope>`

const findMarketsTemplate = `<v:Envelope xmlns:i="http://www.w3.org/2001/XMLSchema-instance"
            xmlns:d="http://www.w3.org/2001/XMLSchema"
            xmlns:c="http://schemas.xmlsoap.org/soap/encoding/"
            xmlns:v="http://schemas.xmlsoap.org/soap/envelope/">
    <v:Header />
    <v:Body>
        <n0:findRetailerShopsMultipleCountries xmlns:n0="http://retailer.mobile.server.valuephone.com/">
            <retailerCountries>
                <retailerCountry>
                    <countryAlpha2>{{escape .Context.CountryCode}}</countryAlpha2>
                    <retailerCountryId>{{.RetailerCountryID}}</retailerCountryId>
                </retailerCountry>
            </retailerCountries>
            <zipOrPlace>{{escape .ZipOrPlace}}</zipOrPlace>
            <limit>{{.Limit}}</limit>
            <offset>{{.Offset}}</offset>
        </n0:findRetailerShopsMultipleCountries>
    </v:Body>
</v:Envelope>`
