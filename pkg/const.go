package edeka

import (
	"slices"
	"time"

	// tzdata is embedded so LoadLocation("Europe/Berlin") works on stripped-down
	// Linux containers and Windows where the system tz database may be absent.
	_ "time/tzdata"
)

// berlinTZ is the timezone Edeka receipts are implicitly in. The receipt list
// endpoint emits timestamps without offset info; the detail endpoint includes
// zone abbreviations (CET/CEST) that ParseInLocation resolves via this location.
var berlinTZ = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic("edeka: loading Europe/Berlin timezone: " + err.Error())
	}
	return loc
}()

var deviceMap = map[string][]string{
	"Samsung": {
		"SM-G975U",
		"SM-N975U",
		"SM-G973U",
		"SM-S911B",
		"SM-S918B",
		"SM-F946B",
		"SM-S901B",
	},
	"OnePlus": {
		"HD1925",
		"AC2003",
	},
	"Xiaomi": {
		"M2007J3SY",
		"2201123G",
	},
	"LG": {
		"LM-G820",
	},
	"Google": {
		"Pixel 8 Pro",
		"Pixel 7 Pro",
		"Pixel 8",
		"Pixel 7a",
	},
}

var manufacturers []string

func init() {
	manufacturers = make([]string, 0, len(deviceMap))
	for k := range deviceMap {
		manufacturers = append(manufacturers, k)
	}
	slices.Sort(manufacturers)
}

const (
	// DefaultAuthFileName is the default auth file name resolved relative to
	// the CURRENT WORKING DIRECTORY at call time. A binary invoked from
	// different directories will read/write different auth files. Pass an
	// absolute path to SaveAuthTo / LoadFromFilepath for stable storage.
	DefaultAuthFileName = "edeka_auth.json"

	// APK-version-tracking constants. When bumping to a new Android app
	// version, update ALL of these in lockstep:
	//   - defaultAppVersion: the app's "versionName" from the APK manifest
	//   - defaultPlatformVersion: the target Android SDK level
	//   - sdkVersion: the Valuephone SDK version embedded in the APK (grep the
	//     decompiled code for "SDK version" strings)
	//   - uaOkHTTP: OkHttp version bundled in the APK
	//   - uaDalvik: Android version + device string. Keep "Android 14" lined
	//     up with defaultPlatformVersion (34 → 14, 35 → 15).
	defaultAppVersion      = "5.4.1"
	defaultPlatformVersion = "34"
	sdkVersion             = "6.71.1"

	// User-agent strings mimicking the Android app's HTTP stack.
	// Colocated for easier bulk updates when the app version changes.
	uaKSOAP  = "kSOAP/2.0; %s; %s; %s" // PlatformVersion, Brand, AppVersion
	uaOkHTTP = "okhttp/4.12.0"
	// Used for offer image requests (CDN). Hardcodes a device because the image CDN
	// doesn't correlate this with the device info sent in SOAP registration headers.
	uaDalvik = "Dalvik/2.1.0 (Linux; U; Android 14; Pixel 4 XL Build/UQ1A.240205.004)"

	// DefaultReceiptPageSize is the number of receipts per page when no
	// custom size is set via SetReceiptPageSize.
	DefaultReceiptPageSize = 50

	defaultReceiptDelay = 1500 * time.Millisecond
)
