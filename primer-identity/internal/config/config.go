// Package config loads Primer Identity runtime configuration from the
// environment using the IDENTITY_ prefix so the service can run beside LMS,
// TV, and Curriculum Studio without colliding settings.
package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"

	"github.com/aleksclark/primer/identity/internal/db"
	"github.com/aleksclark/primer/identity/internal/token"
)

// EnvPrefix namespaces every Identity setting (e.g. IDENTITY_DATABASE_URL).
const EnvPrefix = "IDENTITY"

// StytchConfig contains the narrowly scoped settings used by the B2B session
// adapter. Credentials are intentionally kept here, rather than in a generic
// provider map, so validation can fail closed before a client is constructed.
type StytchConfig struct {
	// split_words (without envconfig alt) yields IDENTITY_STYTCH_* only;
	// envconfig's Alt fallback would otherwise inherit bare SECRET, PROJECT_ID,
	// ENABLED, ENV, BASE_URI, and cache/timeout names.
	Enabled bool `split_words:"true" default:"false"`

	ProjectID string `split_words:"true"`
	Secret    string `split_words:"true"`
	Env       string `split_words:"true" default:"test"`
	BaseURI   string `split_words:"true"`

	RequestTimeout        time.Duration `split_words:"true" default:"3s"`
	PositiveCacheTTL      time.Duration `split_words:"true" default:"15s"`
	NegativeCacheTTL      time.Duration `split_words:"true" default:"5s"`
	PositiveCacheCapacity int           `split_words:"true" default:"10000"`
	NegativeCacheCapacity int           `split_words:"true" default:"2000"`
}

// String returns a credential-free projection. In particular, ProjectID is
// useful operational context, while Secret and BaseURI are deliberately not
// included because this value may cross logging and diagnostic boundaries.
func (c StytchConfig) String() string {
	return fmt.Sprintf("stytch-config enabled=%t project_id=%s env=%s request_timeout=%s", c.Enabled, c.ProjectID, c.Env, c.RequestTimeout)
}

func (c StytchConfig) GoString() string { return c.String() }

// Format intentionally ignores the requested verb and flags. A credential
// bearing configuration object must not fall back to fmt's struct formatter
// for verbs such as %d or %x.
func (c StytchConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, c.String())
}

func (c StytchConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Enabled               bool          `json:"enabled"`
		ProjectID             string        `json:"project_id"`
		Env                   string        `json:"env"`
		RequestTimeout        time.Duration `json:"request_timeout"`
		PositiveCacheTTL      time.Duration `json:"positive_cache_ttl"`
		NegativeCacheTTL      time.Duration `json:"negative_cache_ttl"`
		PositiveCacheCapacity int           `json:"positive_cache_capacity"`
		NegativeCacheCapacity int           `json:"negative_cache_capacity"`
	}{
		Enabled: c.Enabled, ProjectID: c.ProjectID, Env: c.Env,
		RequestTimeout: c.RequestTimeout, PositiveCacheTTL: c.PositiveCacheTTL,
		NegativeCacheTTL: c.NegativeCacheTTL, PositiveCacheCapacity: c.PositiveCacheCapacity,
		NegativeCacheCapacity: c.NegativeCacheCapacity,
	})
}

// Validate checks Stytch settings without constructing an SDK client.
func (c *StytchConfig) Validate() error {
	c.ProjectID = strings.TrimSpace(c.ProjectID)
	c.Secret = strings.TrimSpace(c.Secret)
	c.Env = strings.ToLower(strings.TrimSpace(c.Env))
	c.BaseURI = strings.TrimSpace(c.BaseURI)

	if (c.ProjectID == "") != (c.Secret == "") {
		return fmt.Errorf("identity stytch config: credentials project id and secret must be provided together")
	}
	if c.Enabled && (c.ProjectID == "" || c.Secret == "") {
		return fmt.Errorf("identity stytch config: credentials are required when enabled")
	}
	if c.Env != "test" && c.Env != "live" {
		return fmt.Errorf("identity stytch config: env must be test|live, got %q", c.Env)
	}
	if c.Env == "live" && c.BaseURI != "" {
		return fmt.Errorf("identity stytch config: live env does not allow a base URI override")
	}
	if c.ProjectID != "" && c.Secret != "" {
		wantPrefix := "project-test-"
		if c.Env == "live" {
			wantPrefix = "project-live-"
		}
		if !strings.HasPrefix(c.ProjectID, wantPrefix) {
			return fmt.Errorf("identity stytch config: project id must use %q prefix for env %s", wantPrefix, c.Env)
		}
	}
	if c.RequestTimeout != 3*time.Second {
		return fmt.Errorf("identity stytch config: request timeout must be 3s")
	}
	if c.PositiveCacheTTL <= 0 || c.PositiveCacheTTL > 15*time.Second {
		return fmt.Errorf("identity stytch config: positive cache ttl must be >0 and <=15s")
	}
	if c.NegativeCacheTTL <= 0 || c.NegativeCacheTTL > 5*time.Second {
		return fmt.Errorf("identity stytch config: negative cache ttl must be >0 and <=5s")
	}
	if c.PositiveCacheCapacity < 1 || c.PositiveCacheCapacity > 10000 {
		return fmt.Errorf("identity stytch config: positive cache capacity must be 1..10000")
	}
	if c.NegativeCacheCapacity < 1 || c.NegativeCacheCapacity > 2000 {
		return fmt.Errorf("identity stytch config: negative cache capacity must be 1..2000")
	}
	if c.BaseURI != "" {
		u, err := url.Parse(c.BaseURI)
		if err != nil || !u.IsAbs() || u.Host == "" || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(u.Path, "?#") {
			return fmt.Errorf("identity stytch config: base URI must be an absolute http/https URI without userinfo, query, or fragment")
		}
	}
	return nil
}

// VersionedSecretSet is an immutable copied set of exact 32-byte key material.
// The encoded environment form is comma-separated positiveVersion:base64rawurl.
type VersionedSecretSet struct{ Values map[int][]byte }

func parseVersionedSecrets(value string) (VersionedSecretSet, error) {
	if strings.TrimSpace(value) == "" {
		return VersionedSecretSet{}, fmt.Errorf("required versioned secret set is missing")
	}
	values := make(map[int][]byte)
	for _, item := range strings.Split(value, ",") {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			return VersionedSecretSet{}, fmt.Errorf("invalid versioned secret set")
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version <= 0 {
			return VersionedSecretSet{}, fmt.Errorf("invalid versioned secret version")
		}
		decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil || len(decoded) != 32 {
			return VersionedSecretSet{}, fmt.Errorf("invalid versioned secret material")
		}
		if _, exists := values[version]; exists {
			return VersionedSecretSet{}, fmt.Errorf("duplicate versioned secret version")
		}
		values[version] = append([]byte(nil), decoded...)
	}
	return VersionedSecretSet{Values: values}, nil
}

func requireActive(set VersionedSecretSet, active int) error {
	if active <= 0 {
		return fmt.Errorf("active version must be positive")
	}
	if _, ok := set.Values[active]; !ok {
		return fmt.Errorf("active version has no secret")
	}
	return nil
}

// sealedSecret is a configured seal secret whose raw input exists only while
// configuration is being populated and validated. Its field is deliberately
// unexported so ordinary reflection cannot interface it.
type sealedSecret struct {
	raw string
}

var _ envconfig.Decoder = (*sealedSecret)(nil)

func (s *sealedSecret) Decode(value string) error {
	if s == nil {
		return fmt.Errorf("identity key config: nil seal secret")
	}
	s.raw = value
	return nil
}

func (s sealedSecret) String() string   { return "[redacted]" }
func (s sealedSecret) GoString() string { return "sealedSecret{redacted}" }
func (s sealedSecret) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, s.String())
}
func (s sealedSecret) MarshalJSON() ([]byte, error) {
	return []byte(`""`), nil
}

func (s *sealedSecret) takeRaw() string {
	raw := s.raw
	s.raw = ""
	return raw
}

func (s *sealedSecret) clear() {
	s.raw = ""
}

// KeyConfig holds fail-closed signing-key custody settings. The seal secret
// is never serialized, formatted, or included in errors.
type KeyConfig struct {
	Enabled       bool         `split_words:"true" default:"false" json:"enabled"`
	AutoBootstrap bool         `split_words:"true" default:"false" json:"auto_bootstrap"`
	SealSecret    sealedSecret `split_words:"true" json:"-"`

	sealKey    [32]byte
	sealKeySet bool
}

// SealKey returns the decoded 32-byte AES-256 seal key.
func (k KeyConfig) SealKey() [32]byte {
	return k.sealKey
}

// SetSealSecretForTest sets the raw configured secret so Validate can parse it.
func (k *KeyConfig) SetSealSecretForTest(secret string) {
	if k == nil {
		return
	}
	k.sealKey = [32]byte{}
	k.sealKeySet = false
	k.SealSecret.raw = secret
}

// String reports only non-secret key-custody flags.
func (k KeyConfig) String() string {
	return fmt.Sprintf("key-config enabled=%t auto_bootstrap=%t", k.Enabled, k.AutoBootstrap)
}

// GoString protects %#v from leaking the seal secret.
func (k KeyConfig) GoString() string { return k.String() }

// Format intentionally ignores the requested verb and flags.
func (k KeyConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, k.String())
}

func (k KeyConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Enabled       bool `json:"enabled"`
		AutoBootstrap bool `json:"auto_bootstrap"`
	}{Enabled: k.Enabled, AutoBootstrap: k.AutoBootstrap})
}

func (k *KeyConfig) Validate(serviceEnv string) error {
	if k == nil {
		return fmt.Errorf("identity key config: missing")
	}
	defer k.SealSecret.clear()
	required := k.Enabled || serviceEnv == "production"
	if k.AutoBootstrap && !k.Enabled {
		return fmt.Errorf("identity key config: auto-bootstrap requires key custody to be enabled")
	}
	if k.AutoBootstrap && serviceEnv == "production" {
		return fmt.Errorf("identity key config: auto-bootstrap is not allowed in production")
	}
	if k.AutoBootstrap && serviceEnv != "development" && serviceEnv != "test" {
		return fmt.Errorf("identity key config: auto-bootstrap is allowed only in development or test")
	}
	if err := k.parseSealSecret(required); err != nil {
		return err
	}
	return nil
}

func (k *KeyConfig) parseSealSecret(required bool) error {
	raw := k.SealSecret.takeRaw()
	defer func() { raw = "" }()
	if raw == "" {
		if required {
			if !k.sealKeySet {
				return fmt.Errorf("identity key config: seal secret is required")
			}
			return nil
		}
		k.sealKey = [32]byte{}
		k.sealKeySet = false
		return nil
	}
	k.sealKey = [32]byte{}
	k.sealKeySet = false
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	defer zeroBytes(decoded)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return fmt.Errorf("identity key config: seal secret must be 32 decoded bytes in canonical base64url without padding")
	}
	copy(k.sealKey[:], decoded)
	k.sealKeySet = true
	return nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// Config holds all Identity runtime configuration, populated from the environment.
type Config struct {
	// Stytch is mandatory in production. Development and test may remain
	// explicitly disabled; callers must not create a provider client then.
	Stytch StytchConfig `split_words:"true"`

	// Key controls local ES256 key custody. Loaded only from IDENTITY_KEY_*.
	// The seal secret is required when custody is enabled and is always
	// required in production. Bare KEY_* / SEAL_SECRET names are ignored.
	Key KeyConfig `split_words:"true"`

	// DatabaseURL is the PostgreSQL connection string for the Identity DB only.
	// Required and non-empty in every environment (no localhost default).
	// Loaded only from IDENTITY_DATABASE_URL — bare DATABASE_URL is ignored so
	// ambient LMS/host DSNs cannot silently satisfy Identity config.
	// split_words (without envconfig alt) yields IDENTITY_DATABASE_URL only;
	// envconfig's Alt fallback would otherwise inherit bare DATABASE_URL.
	DatabaseURL string `split_words:"true"`
	// Host is the address the HTTP server binds to.
	Host string `split_words:"true" default:"0.0.0.0"`
	// Port is the TCP port the HTTP server listens on.
	Port int `split_words:"true" default:"8090"`
	// Env is the deployment environment name: development|test|production.
	Env string `split_words:"true" default:"development"`
	// LogLevel is the slog level name (debug|info|warn|error).
	LogLevel string `split_words:"true" default:"info"`
	// Issuer is the OIDC issuer URL. Required non-empty in every environment.
	// Loaded only from IDENTITY_ISSUER — bare ISSUER is ignored.
	Issuer string `split_words:"true"`
	// ShutdownTimeout bounds graceful HTTP shutdown after SIGINT/SIGTERM.
	ShutdownTimeout time.Duration `split_words:"true" default:"10s"`
	// HTTPReadHeaderTimeout bounds how long the server waits for request headers.
	HTTPReadHeaderTimeout time.Duration `split_words:"true" default:"10s"`
	// HTTPMaxBodyBytes caps request body size for future write endpoints.
	HTTPMaxBodyBytes int64 `split_words:"true" default:"1048576"`

	// BrokerAllowedOrigin is the exact Origin accepted by mutating broker POSTs.
	// Loaded only from IDENTITY_BROKER_ALLOWED_ORIGIN.
	BrokerAllowedOrigin string `split_words:"true"`
	// BrokerDiscoveryRedirectURL is the exact Stytch discovery redirect URL.
	BrokerDiscoveryRedirectURL string `split_words:"true"`
	// BrokerLoginRedirectURL is the exact Stytch login redirect URL.
	BrokerLoginRedirectURL string `split_words:"true"`
	// BrokerSignupRedirectURL is the exact Stytch signup redirect URL.
	BrokerSignupRedirectURL string `split_words:"true"`
	// StytchPublicToken is the public (non-secret) Stytch token used for SSO start.
	StytchPublicToken string `split_words:"true"`
	// InsecureBrokerCookie disables the Secure cookie flag. Production forbids it.
	InsecureBrokerCookie bool `split_words:"true" default:"false"`

	StateSealKeys                  string        `split_words:"true"`
	StateSealActiveVersion         int           `split_words:"true"`
	StateHashPeppers               string        `split_words:"true"`
	StateHashActiveVersion         int           `split_words:"true"`
	BrokerCookiePeppers            string        `split_words:"true"`
	BrokerCookieActiveVersion      int           `split_words:"true"`
	AuthorizationCodePeppers       string        `split_words:"true"`
	AuthorizationCodeActiveVersion int           `split_words:"true"`
	ClientSecretPeppers            string        `split_words:"true"`
	ClientSecretActiveVersion      int           `split_words:"true"`
	RefreshTokenPeppers            string        `split_words:"true"`
	RefreshTokenActiveVersion      int           `split_words:"true"`
	ClientAssertionPeppers         string        `split_words:"true"`
	ClientAssertionActiveVersion   int           `split_words:"true"`
	BrokerTransactionTTL           time.Duration `split_words:"true" default:"10m"`
	BrokerCookieTTL                time.Duration `split_words:"true" default:"10m"`
	AuthorizationCodeTTL           time.Duration `split_words:"true" default:"60s"`
	AuthorizeTargetMaxBytes        int           `split_words:"true" default:"8192"`
	ProviderRevalidationDeadline   time.Duration `split_words:"true" default:"2s"`
	ProviderResponseMaxBytes       int           `split_words:"true" default:"1048576"`
	ProviderMemberSessionsMax      int           `split_words:"true" default:"256"`
	ProviderProofCacheTTL          time.Duration `split_words:"true" default:"15s"`
	ProviderProofCacheCapacity     int           `split_words:"true" default:"256"`

	StateSealKeySet            VersionedSecretSet `ignored:"true"`
	StateHashPepperSet         VersionedSecretSet `ignored:"true"`
	BrokerCookiePepperSet      VersionedSecretSet `ignored:"true"`
	AuthorizationCodePepperSet VersionedSecretSet `ignored:"true"`
	ClientSecretPepperSet      VersionedSecretSet `ignored:"true"`
	RefreshTokenPepperSet      VersionedSecretSet `ignored:"true"`
	ClientAssertionPepperSet   VersionedSecretSet `ignored:"true"`
}

// String returns only operational flags and non-credential identity metadata.
// DatabaseURL, provider secrets, pepper encodings, and parsed secret material
// are intentionally absent from this projection.
func (c Config) String() string {
	return fmt.Sprintf("identity-config env=%s host=%s port=%d stytch_enabled=%t key_enabled=%t", c.Env, c.Host, c.Port, c.Stytch.Enabled, c.Key.Enabled)
}

func (c Config) GoString() string { return c.String() }

// Format intentionally ignores the requested verb and flags.
func (c Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, c.String())
}

func (c Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Stytch               StytchConfig `json:"stytch"`
		Key                  KeyConfig    `json:"key"`
		Host                 string       `json:"host"`
		Port                 int          `json:"port"`
		Env                  string       `json:"env"`
		LogLevel             string       `json:"log_level"`
		Issuer               string       `json:"issuer"`
		InsecureBrokerCookie bool         `json:"insecure_broker_cookie"`
	}{
		Stytch: c.Stytch, Key: c.Key, Host: c.Host, Port: c.Port,
		Env: c.Env, LogLevel: c.LogLevel, Issuer: c.Issuer,
		InsecureBrokerCookie: c.InsecureBrokerCookie,
	})
}

// Load reads Identity configuration from the environment and validates it.
func Load() (*Config, error) {
	var cfg Config
	defer cfg.Key.SealSecret.clear()
	if err := envconfig.Process(EnvPrefix, &cfg); err != nil {
		return nil, fmt.Errorf("load identity config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Addr returns the host:port bind address.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// BrokerEnabled reports whether this process should compose the IB1 broker.
// Production always requires the official provider. Development and test may
// leave Stytch disabled so health-only checkout still works.
func (c *Config) BrokerEnabled() bool {
	return c.Stytch.Enabled
}

// TokenAuthorityEnabled reports whether this process must compose signing-key
// custody and token peppers. Production always enables token authority after
// Validate. Development and test stay health-only unless key custody is
// explicitly enabled.
func (c *Config) TokenAuthorityEnabled() bool {
	return c.Key.Enabled
}

// Validate enforces fail-fast rules for Identity configuration.
func (c *Config) Validate() error {
	defer c.Key.SealSecret.clear()
	serviceEnv := strings.ToLower(strings.TrimSpace(c.Env))
	switch serviceEnv {
	case "development", "test", "production":
		c.Env = serviceEnv
	default:
		return fmt.Errorf("identity config: env must be development|test|production, got %q", c.Env)
	}

	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	c.Issuer = strings.TrimSpace(c.Issuer)

	if c.DatabaseURL == "" {
		return fmt.Errorf("identity config: database url is required")
	}
	if c.Issuer == "" {
		return fmt.Errorf("identity config: issuer is required")
	}
	canonicalIssuer, err := token.CanonicalIssuer(c.Issuer, serviceEnv == "production")
	if err != nil {
		return fmt.Errorf("identity config: issuer is invalid")
	}
	c.Issuer = canonicalIssuer
	// Port 0 is allowed for tests that inject an already-bound listener.
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("identity config: port out of range: %d", c.Port)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("identity config: shutdown timeout must be positive")
	}
	if c.HTTPReadHeaderTimeout <= 0 {
		return fmt.Errorf("identity config: http read header timeout must be positive")
	}
	if c.HTTPMaxBodyBytes <= 0 {
		return fmt.Errorf("identity config: http max body bytes must be positive")
	}
	// Same pgx-parsed forbidden-name validator as library Connect/Migrate —
	// no forked deny list in config.
	if err := db.ValidateDatabaseURL(c.DatabaseURL); err != nil {
		return fmt.Errorf("identity config: %w", err)
	}
	if (c.BrokerTransactionTTL != 0 && c.BrokerTransactionTTL != 10*time.Minute) || (c.BrokerCookieTTL != 0 && c.BrokerCookieTTL != 10*time.Minute) || (c.AuthorizationCodeTTL != 0 && c.AuthorizationCodeTTL != time.Minute) || (c.AuthorizeTargetMaxBytes != 0 && c.AuthorizeTargetMaxBytes != 8192) || (c.ProviderRevalidationDeadline != 0 && c.ProviderRevalidationDeadline != 2*time.Second) || (c.ProviderResponseMaxBytes != 0 && c.ProviderResponseMaxBytes != 1<<20) || (c.ProviderMemberSessionsMax != 0 && c.ProviderMemberSessionsMax != 256) || (c.ProviderProofCacheTTL != 0 && (c.ProviderProofCacheTTL <= 0 || c.ProviderProofCacheTTL > 15*time.Second)) || (c.ProviderProofCacheCapacity != 0 && (c.ProviderProofCacheCapacity <= 0 || c.ProviderProofCacheCapacity > 1024)) {
		return fmt.Errorf("identity config: invalid IB1 bounded lifetime or provider limit")
	}
	if serviceEnv == "production" {
		if !c.Stytch.Enabled {
			return fmt.Errorf("identity config: production service requires Stytch enabled")
		}
		if !strings.EqualFold(strings.TrimSpace(c.Stytch.Env), "live") {
			return fmt.Errorf("identity config: production service requires Stytch env live")
		}
		if strings.TrimSpace(c.Stytch.BaseURI) != "" {
			return fmt.Errorf("identity config: production service does not allow a Stytch base URI override")
		}
		if c.InsecureBrokerCookie {
			return fmt.Errorf("identity config: production forbids an insecure broker cookie")
		}
	}
	// Older tests and explicit programmatic configs may omit the optional
	// provider entirely. envconfig populates all defaults for Load; only run
	// provider validation when it was actually configured.
	if c.Stytch != (StytchConfig{}) || serviceEnv == "production" {
		if err := c.Stytch.Validate(); err != nil {
			return err
		}
	}
	if err := c.validateBrokerHTTP(serviceEnv); err != nil {
		return err
	}
	if serviceEnv == "production" {
		var err error
		if c.StateSealKeySet, err = parseVersionedSecrets(c.StateSealKeys); err != nil {
			return fmt.Errorf("identity config: state seal keys invalid")
		}
		if err = requireActive(c.StateSealKeySet, c.StateSealActiveVersion); err != nil {
			return fmt.Errorf("identity config: state seal active version invalid")
		}
		if c.StateHashPepperSet, err = parseVersionedSecrets(c.StateHashPeppers); err != nil {
			return fmt.Errorf("identity config: state hash peppers invalid")
		}
		if err = requireActive(c.StateHashPepperSet, c.StateHashActiveVersion); err != nil {
			return fmt.Errorf("identity config: state hash active version invalid")
		}
		if c.BrokerCookiePepperSet, err = parseVersionedSecrets(c.BrokerCookiePeppers); err != nil {
			return fmt.Errorf("identity config: broker cookie peppers invalid")
		}
		if err = requireActive(c.BrokerCookiePepperSet, c.BrokerCookieActiveVersion); err != nil {
			return fmt.Errorf("identity config: broker cookie active version invalid")
		}
		if c.AuthorizationCodePepperSet, err = parseVersionedSecrets(c.AuthorizationCodePeppers); err != nil {
			return fmt.Errorf("identity config: authorization code peppers invalid")
		}
		if err = requireActive(c.AuthorizationCodePepperSet, c.AuthorizationCodeActiveVersion); err != nil {
			return fmt.Errorf("identity config: authorization code active version invalid")
		}
	}
	if err := c.Key.Validate(serviceEnv); err != nil {
		return err
	}
	if serviceEnv == "production" {
		c.Key.Enabled = true
	}
	if c.TokenAuthorityEnabled() {
		if err := c.requireTokenAuthority(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) requireTokenAuthority() error {
	var err error
	if c.ClientSecretPepperSet, err = parseVersionedSecrets(c.ClientSecretPeppers); err != nil {
		return fmt.Errorf("identity config: token secret material is invalid")
	}
	if err = requireActive(c.ClientSecretPepperSet, c.ClientSecretActiveVersion); err != nil {
		return fmt.Errorf("identity config: token secret material is invalid")
	}
	if c.RefreshTokenPepperSet, err = parseVersionedSecrets(c.RefreshTokenPeppers); err != nil {
		return fmt.Errorf("identity config: token secret material is invalid")
	}
	if err = requireActive(c.RefreshTokenPepperSet, c.RefreshTokenActiveVersion); err != nil {
		return fmt.Errorf("identity config: token secret material is invalid")
	}
	if c.ClientAssertionPepperSet, err = parseVersionedSecrets(c.ClientAssertionPeppers); err != nil {
		return fmt.Errorf("identity config: token secret material is invalid")
	}
	if err = requireActive(c.ClientAssertionPepperSet, c.ClientAssertionActiveVersion); err != nil {
		return fmt.Errorf("identity config: token secret material is invalid")
	}
	return nil
}

func (c *Config) validateBrokerHTTP(serviceEnv string) error {
	c.BrokerAllowedOrigin = strings.TrimSpace(c.BrokerAllowedOrigin)
	c.BrokerDiscoveryRedirectURL = strings.TrimSpace(c.BrokerDiscoveryRedirectURL)
	c.BrokerLoginRedirectURL = strings.TrimSpace(c.BrokerLoginRedirectURL)
	c.BrokerSignupRedirectURL = strings.TrimSpace(c.BrokerSignupRedirectURL)
	c.StytchPublicToken = strings.TrimSpace(c.StytchPublicToken)

	required := serviceEnv == "production" || c.Stytch.Enabled
	anySet := c.BrokerAllowedOrigin != "" || c.BrokerDiscoveryRedirectURL != "" ||
		c.BrokerLoginRedirectURL != "" || c.BrokerSignupRedirectURL != "" || c.StytchPublicToken != ""
	if !required && !anySet {
		return nil
	}
	return c.RequireBrokerHTTP()
}

// RequireBrokerHTTP validates exact origin, redirect URLs, and public token.
// App composition calls this whenever the broker is enabled or injected.
func (c *Config) RequireBrokerHTTP() error {
	c.BrokerAllowedOrigin = strings.TrimSpace(c.BrokerAllowedOrigin)
	c.BrokerDiscoveryRedirectURL = strings.TrimSpace(c.BrokerDiscoveryRedirectURL)
	c.BrokerLoginRedirectURL = strings.TrimSpace(c.BrokerLoginRedirectURL)
	c.BrokerSignupRedirectURL = strings.TrimSpace(c.BrokerSignupRedirectURL)
	c.StytchPublicToken = strings.TrimSpace(c.StytchPublicToken)

	requireHTTPS := strings.EqualFold(strings.TrimSpace(c.Env), "production")
	if err := validateExactOrigin(c.BrokerAllowedOrigin, requireHTTPS); err != nil {
		return err
	}
	for _, raw := range []string{c.BrokerDiscoveryRedirectURL, c.BrokerLoginRedirectURL, c.BrokerSignupRedirectURL} {
		if err := validateExactRedirectURL(raw, requireHTTPS); err != nil {
			return err
		}
	}
	if !validPublicToken(c.StytchPublicToken) {
		return fmt.Errorf("identity config: stytch public token is invalid")
	}
	return nil
}

func validateExactOrigin(raw string, requireHTTPS bool) error {
	if raw == "" {
		return fmt.Errorf("identity config: broker allowed origin is required")
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.Contains(raw, ",") {
		return fmt.Errorf("identity config: broker allowed origin must be an exact origin")
	}
	if requireHTTPS {
		if !strings.EqualFold(u.Scheme, "https") {
			return fmt.Errorf("identity config: broker allowed origin must be an exact https origin")
		}
	} else if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("identity config: broker allowed origin must be an exact origin")
	}
	return nil
}

func validateExactRedirectURL(raw string, requireHTTPS bool) error {
	if raw == "" {
		return fmt.Errorf("identity config: broker redirect url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || strings.ContainsAny(u.Path, "?#") {
		return fmt.Errorf("identity config: broker redirect url must be an exact url")
	}
	if requireHTTPS {
		if !strings.EqualFold(u.Scheme, "https") {
			return fmt.Errorf("identity config: broker redirect url must be an exact https url")
		}
	} else if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("identity config: broker redirect url must be an exact url")
	}
	return nil
}

func validPublicToken(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, r := range value {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
