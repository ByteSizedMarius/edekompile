package edeka

// offersEndpoints holds the REST URLs for the offers API. Grouped separately
// from the SOAP mappers because they carry no template/action and follow a
// different auth model (bearer token via client_credentials).
type offersEndpoints struct {
	tokenURL string
	dataURL  string
}

// endpoints bundles every per-request URL/template the library hits. Instance-
// scoped so tests can swap URLs without mutating package globals.
type endpoints struct {
	registerAnonymously templateActionMapper
	checkin             templateActionMapper
	bearerLogin         templateActionMapper
	checkRegistration   templateActionMapper
	getReceipts         templateActionMapper
	getReceipt          templateActionMapper
	findMarkets         templateActionMapper
	offers              offersEndpoints
}

var defaultEndpoints = endpoints{
	registerAnonymously: templateActionMapper{
		name:     "RegisterAnonymously",
		template: mustParseTemplate("RegisterAnonymously", registerAnonTemplate),
		action:   "registerMobileApplicationForAnonymousUser",
		baseURL:  "https://www.myvaluephone.com/vpserver/ws/mobile/OpenMobileDeviceRegistrationManager",
	},
	checkin: templateActionMapper{
		name:     "Checkin",
		template: mustParseTemplate("Checkin", checkinTemplate),
		action:   "notifyPostLogin",
		baseURL:  "https://www.myvaluephone.com/vpserver/ws/mobile/MobileUserManager",
	},
	bearerLogin: templateActionMapper{
		name:     "BearerLogin",
		template: mustParseTemplate("BearerLogin", bearerToLoginTemplate),
		action:   "login",
		baseURL:  "https://www.myvaluephone.com/vpserver/ws/mobile/MobileSSOServiceManager",
	},
	checkRegistration: templateActionMapper{
		name:     "CheckRegistration",
		template: mustParseTemplate("CheckRegistration", checkRegistrationTemplate),
		action:   "checkMobileApplicationRegistrationLogin",
		baseURL:  "https://www.myvaluephone.com/vpserver/ws/mobile/OpenMobileDeviceRegistrationManager",
	},
	getReceipts: templateActionMapper{
		name:     "GetReceipts",
		template: mustParseTemplate("GetReceipts", getReceiptsTemplate),
		action:   "getAllReceiptsByApp",
		baseURL:  "https://www.myvaluephone.com/vpserver/ws/mobile/MobileReceiptServiceManager",
	},
	getReceipt: templateActionMapper{
		name:     "GetReceipt",
		template: mustParseTemplate("GetReceipt", getReceiptTemplate),
		action:   "getFormattedReceipt",
		baseURL:  "https://www.myvaluephone.com/vpserver/ws/mobile/MobileReceiptServiceManager",
	},
	findMarkets: templateActionMapper{
		name:     "FindMarkets",
		template: mustParseTemplate("FindMarkets", findMarketsTemplate),
		action:   "findRetailerShopsMultipleCountries",
		baseURL:  "https://www.myvaluephone.com/vpserver/ws/mobile/MobileRetailerServiceManager",
	},
	offers: offersEndpoints{
		tokenURL: "https://b2b-login.api.edeka/auth/realms/b2b/protocol/openid-connect/token",
		dataURL:  "https://b2c-gw.api.edeka/v2/offers/mobile",
	},
}
