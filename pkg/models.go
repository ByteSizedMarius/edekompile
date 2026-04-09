package edeka

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Edeka is the main struct that holds the configuration and credentials for the API.
// All methods apart from registration/login are called on this struct.
//
// The embedded OffersClient is how *Edeka exposes GetOffers: a full-credential
// instance can call every method that *OffersClient has without an explicit
// hop through a nested field. That embedding is by value so every *Edeka has
// its own bearer cache, independent of any standalone *OffersClient. The
// HTTPDoer stored on the embedded OffersClient doubles as the SOAP client
// for every *Edeka call (receipts, markets, auth) - there is no second
// client field on Edeka itself.
//
// Concurrency: GetOffers, GetOfferImage, and the underlying bearer refresh
// (inherited from the embedded OffersClient) are safe to call from multiple
// goroutines on the same instance - the bearer cache is mutex-protected.
// Everything else (GetReceipts, GetReceipt, GetAllReceipts, FindMarkets,
// Reauthenticate, and the config mutators WithClient / SetReceiptPageSize /
// SetReceiptDelay) is NOT safe for concurrent use. Callers that only need
// concurrent access to the anonymous offers API should use NewOffersClient
// directly instead.
type Edeka struct {
	OffersClient

	config Config
	creds  Credentials

	// temporaryCreds is true when creds came from anonymous registration.
	// Temporary credentials can't fetch receipts and can't be saved via SaveAuth.
	temporaryCreds bool

	// pageSize controls receipt pagination size. Zero means use the default (50).
	// Set via SetReceiptPageSize.
	pageSize int

	// delay controls the sleep between receipt pagination requests. Interpreted
	// only when delaySet is true - that lets SetReceiptDelay(0) mean "no delay"
	// without colliding with the unset-default zero value.
	delay    time.Duration
	delaySet bool
}

func newEdeka(config Config, creds Credentials, client HTTPDoer) *Edeka {
	return &Edeka{
		OffersClient: OffersClient{endpoints: defaultEndpoints, client: client},
		config:       config,
		creds:        creds,
	}
}

// WithClient sets the HTTP client and returns the receiver for chaining.
// Passing nil (or never calling this) uses the library's default client,
// which applies a 30-second timeout. To opt out of the timeout, pass
// http.DefaultClient or a custom client with no Timeout set.
// Must not be called concurrently with in-flight requests.
func (e *Edeka) WithClient(c HTTPDoer) *Edeka {
	e.client = c
	return e
}

// SaveAuth saves the current configuration and credentials to edeka_auth.json
// in the CURRENT WORKING DIRECTORY at call time. A process invoked from a
// different directory will write to a different file. Use SaveAuthTo with an
// absolute path for stable storage.
func (e *Edeka) SaveAuth() error {
	return e.SaveAuthTo(DefaultAuthFileName)
}

// SaveAuthTo saves the current configuration and credentials to the specified path.
// Returns an error if the credentials are temporary (from anonymous registration).
func (e *Edeka) SaveAuthTo(path string) error {
	if e.temporaryCreds {
		return errors.New("cannot save temporary credentials (use CredentialsFromBearer to get permanent credentials)")
	}
	var auth AuthFile
	auth.fromData(e.config, e.creds)
	return auth.saveToPath(path)
}

// Config returns the configuration used by this instance.
func (e *Edeka) Config() Config { return e.config }

// Credentials returns the API credentials used by this instance.
// The returned struct includes the cleartext Password - intended for
// save/export flows that need the raw pair. Logging Credentials directly
// goes through its redacting String(); callers that log via struct access
// must treat the Password as secret.
func (e *Edeka) Credentials() Credentials { return e.creds }

func (e *Edeka) effectiveReceiptDelay() time.Duration {
	if !e.delaySet {
		return defaultReceiptDelay
	}
	return e.delay
}

// mobileContext builds the country/language pair that every SOAP data call
// embeds in its request body, lifted off the instance's Config.
func (e *Edeka) mobileContext() mobileContext {
	return mobileContext{
		CountryCode:  e.config.CountryCode,
		LanguageCode: e.config.LanguageCode,
	}
}

// SetReceiptPageSize sets the number of receipts requested per pagination page.
// Must be in range [1, 500]. Zero or unset falls back to the default (50).
func (e *Edeka) SetReceiptPageSize(n int) error {
	if n < 1 || n > 500 {
		return fmt.Errorf("receipt page size must be in [1, 500], got %d", n)
	}
	e.pageSize = n
	return nil
}

// SetReceiptDelay sets the sleep between receipt pagination requests.
// SetReceiptDelay(0) means "no delay". Negative values are rejected.
// If never called, the default (1500ms) is used.
func (e *Edeka) SetReceiptDelay(d time.Duration) error {
	if d < 0 {
		return fmt.Errorf("receipt delay must be >= 0, got %s", d)
	}
	e.delay = d
	e.delaySet = true
	return nil
}

// ------------------------------------------------------------------------------
// Device and Header Configuration
// ------------------------------------------------------------------------------

// DeviceConfig holds the device information that is sent to the API during registration and checkins.
type DeviceConfig struct {
	DeviceID     string `json:"device_id"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
}

// Random generates a random IMEI with a valid Luhn check digit
// and selects a random device from a predefined list.
func (dc *DeviceConfig) Random() {
	dc.DeviceID = generateIMEI()
	dc.Manufacturer, dc.Model = getRandomDevice()
}

// requiredField pairs a field's human-readable name with its current value.
// Used by requireNonEmpty so Verify methods can list their required fields
// declaratively instead of repeating the same loop.
type requiredField struct{ name, val string }

// requireNonEmpty returns an error naming the first zero-valued field.
// context is the label for the entity being validated ("config", "auth file").
func requireNonEmpty(context string, fields []requiredField) error {
	for _, f := range fields {
		if f.val == "" {
			return fmt.Errorf("%s missing required field: %s", context, f.name)
		}
	}
	return nil
}

// Config holds header values the API expects in every request
type Config struct {
	DeviceConfig
	MobileApplication string
	Brand             string
	AppVersion        string
	Platform          string
	PlatformVersion   string
	CountryCode       string
	LanguageCode      string
}

// Verify checks if all required fields are set.
func (c *Config) Verify() error {
	return requireNonEmpty("config", []requiredField{
		{"MobileApplication", c.MobileApplication},
		{"Brand", c.Brand},
		{"AppVersion", c.AppVersion},
		{"Platform", c.Platform},
		{"PlatformVersion", c.PlatformVersion},
		{"DeviceID", c.DeviceID},
		{"Manufacturer", c.Manufacturer},
		{"Model", c.Model},
		{"CountryCode", c.CountryCode},
		{"LanguageCode", c.LanguageCode},
	})
}

// FillDefaults sets default values for header fields that are still zero-valued.
// It does not set DeviceConfig (DeviceID, Manufacturer, Model) - call
// DeviceConfig.Random() or load from an auth file separately.
func (c *Config) FillDefaults() {
	if c.MobileApplication == "" {
		c.MobileApplication = "OVERALLKBP"
	}
	if c.Brand == "" {
		c.Brand = "EDEKA"
	}
	if c.AppVersion == "" {
		c.AppVersion = defaultAppVersion
	}
	if c.Platform == "" {
		c.Platform = "ANDROID"
	}
	if c.PlatformVersion == "" {
		c.PlatformVersion = defaultPlatformVersion
	}
	if c.CountryCode == "" {
		c.CountryCode = "DE"
	}
	if c.LanguageCode == "" {
		c.LanguageCode = "de"
	}
}

// ------------------------------------------------------------------------------
// Authentication File
// ------------------------------------------------------------------------------

// AuthFile stores the device configuration and generated API token pair on the filesystem in cleartext.
// This is required because otherwise the user would have to provide a valid oauth bearer, or the software
// would have to register for an anonymous account every time it is started. To prevent spam, this file is written.
//
// Attention: Anyone with access to this file can query the API in your name.
type AuthFile struct {
	DeviceConfig    DeviceConfig `json:"device_config"`
	SoapCredentials Credentials  `json:"soap_credentials"`
}

func (a *AuthFile) fromData(config Config, creds Credentials) {
	a.DeviceConfig = config.DeviceConfig
	a.SoapCredentials = creds
}

// Verify checks if all required fields are set.
func (a *AuthFile) Verify() error {
	return requireNonEmpty("auth file", []requiredField{
		{"DeviceID", a.DeviceConfig.DeviceID},
		{"Manufacturer", a.DeviceConfig.Manufacturer},
		{"Model", a.DeviceConfig.Model},
		{"Username", a.SoapCredentials.Username},
		{"Password", a.SoapCredentials.Password},
	})
}

func (a *AuthFile) saveToPath(path string) error {
	if err := a.Verify(); err != nil {
		return fmt.Errorf("auth file incomplete: %w", err)
	}

	jsonData, err := json.MarshalIndent(a, "", "    ")
	if err != nil {
		return fmt.Errorf("marshaling auth file: %w", err)
	}

	// Write atomically: a crash mid-write would otherwise leave a corrupted file
	// that forces the user through the whole bearer-exchange flow again.
	// os.CreateTemp creates with mode 0600 on Unix; file-mode bits are ignored
	// on Windows (ACL inherited from parent).
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating auth file directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp auth file: %w", err)
	}
	tmpPath := tmp.Name()
	// If anything below fails, remove the temp file. Safe to call after a
	// successful rename - os.Remove returns an error we can ignore for the
	// "already gone" case.
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(jsonData); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp auth file: %w", err)
	}
	// Flush to disk before rename so a crash between write and rename can't
	// leave a zero-length file on filesystems that don't order data+metadata.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temp auth file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp auth file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp auth file: %w", err)
	}

	return nil
}

// LoadFromFilepath reads and validates the AuthFile from the given path.
// Returns an error if the file is missing required fields after unmarshaling.
func (a *AuthFile) LoadFromFilepath(path string) error {
	jsonData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading auth file: %w", err)
	}

	if err = json.Unmarshal(jsonData, a); err != nil {
		return fmt.Errorf("unmarshaling auth file: %w", err)
	}

	if err = a.Verify(); err != nil {
		return fmt.Errorf("auth file %s: %w", path, err)
	}

	return nil
}

// ------------------------------------------------------------------------------
// Misc. Helpers
// ------------------------------------------------------------------------------

// Credentials holds the API token pair used for authentication.
// The field names match the XML tags from the API response.
type Credentials struct {
	Username string `xml:"username" json:"username"`
	Password string `xml:"password" json:"password"`
}

// String renders the Credentials with the password redacted. Keeps
// fmt.Printf("%v", creds) or incidental logging from leaking the secret
// into stdout/log files. %#v and struct field access still expose the
// raw password - callers that need to log credentials deliberately must
// reach for those explicitly.
func (c Credentials) String() string {
	return fmt.Sprintf("{Username:%s Password:***}", c.Username)
}
