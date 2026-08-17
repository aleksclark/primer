package config_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/config"
	"github.com/aleksclark/primer/identity/internal/logging"
)

// canonicalSealSecret returns a distinctive 32-byte key encoded as the one
// accepted IDENTITY_KEY_SEAL_SECRET form: base64url with no padding.
func canonicalSealSecret(t *testing.T) (encoded string, raw [32]byte) {
	t.Helper()
	_, err := rand.Read(raw[:])
	require.NoError(t, err)
	copy(raw[:], []byte("PLANT_SEAL_SECRET_VALUE_AAAA"))
	return base64.RawURLEncoding.EncodeToString(raw[:]), raw
}

func productionKeyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IDENTITY_ENV", "production")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@db:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "https://id.example")
	t.Setenv("IDENTITY_STYTCH_ENABLED", "true")
	t.Setenv("IDENTITY_STYTCH_ENV", "live")
	t.Setenv("IDENTITY_STYTCH_PROJECT_ID", "project-live-example")
	t.Setenv("IDENTITY_STYTCH_SECRET", "prefixed-secret-value")
	t.Setenv("IDENTITY_BROKER_ALLOWED_ORIGIN", "https://id.example")
	t.Setenv("IDENTITY_BROKER_DISCOVERY_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_LOGIN_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_BROKER_SIGNUP_REDIRECT_URL", "https://id.example/broker/stytch/callback")
	t.Setenv("IDENTITY_STYTCH_PUBLIC_TOKEN", "public-token-live-example")
	t.Setenv("IDENTITY_STATE_SEAL_KEYS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_STATE_SEAL_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_STATE_HASH_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_STATE_HASH_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_BROKER_COOKIE_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_BROKER_COOKIE_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_AUTHORIZATION_CODE_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_AUTHORIZATION_CODE_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_CLIENT_SECRET_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_CLIENT_SECRET_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_REFRESH_TOKEN_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_REFRESH_TOKEN_ACTIVE_VERSION", "1")
	t.Setenv("IDENTITY_CLIENT_ASSERTION_PEPPERS", "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	t.Setenv("IDENTITY_CLIENT_ASSERTION_ACTIVE_VERSION", "1")
}

func TestLoadRequiresCanonicalSealSecretInProduction(t *testing.T) {
	encoded, raw := canonicalSealSecret(t)
	productionKeyEnv(t)
	t.Setenv("IDENTITY_KEY_SEAL_SECRET", encoded)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, raw, cfg.Key.SealKey())
	assert.False(t, cfg.Key.AutoBootstrap)
}

func TestLoadRejectsMissingSealSecretInProduction(t *testing.T) {
	productionKeyEnv(t)
	require.NoError(t, os.Unsetenv("IDENTITY_KEY_SEAL_SECRET"))

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "seal")
}

func TestLoadRequiresSealSecretWhenKeyCustodyEnabled(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
	t.Setenv("IDENTITY_KEY_ENABLED", "true")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "seal")
}

func TestLoadAcceptsCanonicalSealSecretWhenKeyCustodyEnabled(t *testing.T) {
	encoded, raw := canonicalSealSecret(t)
	t.Setenv("IDENTITY_ENV", "test")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "https://id.example.test")
	t.Setenv("IDENTITY_KEY_ENABLED", "true")
	t.Setenv("IDENTITY_KEY_SEAL_SECRET", encoded)
	tokenEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.Key.Enabled)
	assert.Equal(t, raw, cfg.Key.SealKey())
}

func TestLoadRejectsNonCanonicalSealSecrets(t *testing.T) {
	encoded, raw := canonicalSealSecret(t)
	padded := encoded + "=="
	std := base64.StdEncoding.EncodeToString(raw[:])
	hexish := fmt.Sprintf("%x", raw[:])
	plain := "raw-plaintext-secret-is-not-encoded!"
	spaced := " " + encoded
	internalSpace := encoded[:10] + " " + encoded[10:]
	shortRaw := base64.RawURLEncoding.EncodeToString(raw[:16])
	longRaw := base64.RawURLEncoding.EncodeToString(append(raw[:], 0x01))

	cases := []struct {
		name  string
		value string
	}{
		{"raw plaintext bytes", plain},
		{"standard base64", std},
		{"padded base64url", padded},
		{"hex", hexish},
		{"leading whitespace", spaced},
		{"internal whitespace", internalSpace},
		{"decoded 16 bytes", shortRaw},
		{"decoded 33 bytes", longRaw},
		{"empty when enabled", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("IDENTITY_ENV", "test")
			t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
			t.Setenv("IDENTITY_ISSUER", "https://id.example.test")
			t.Setenv("IDENTITY_KEY_ENABLED", "true")
			t.Setenv("IDENTITY_KEY_SEAL_SECRET", tc.value)

			_, err := config.Load()
			require.Error(t, err, tc.name)
			msg := err.Error()
			assert.NotContains(t, msg, encoded)
			assert.NotContains(t, msg, plain)
			if tc.value != "" {
				assert.NotContains(t, msg, tc.value)
			}
			assert.Contains(t, strings.ToLower(msg), "seal")
		})
	}
}

func TestSealSecretNeverAppearsInErrorsJSONOrFormatting(t *testing.T) {
	encoded, raw := canonicalSealSecret(t)
	cfg := validConfig()
	cfg.Key.Enabled = true
	cfg.Key.SetSealSecretForTest(encoded)
	secretFormatted := ""
	for _, format := range []string{"%v", "%#v", "%+v", "%s", "%d", "%x"} {
		secretFormatted += fmt.Sprintf(format, cfg.Key.SealSecret)
	}
	assert.NotContains(t, secretFormatted, encoded)
	assert.NotContains(t, secretFormatted, string(raw[:]))
	require.NoError(t, cfg.Validate())

	blob, err := json.Marshal(cfg)
	require.NoError(t, err)
	assert.NotContains(t, string(blob), encoded)
	assert.NotContains(t, string(blob), string(raw[:]))

	formatted := fmt.Sprintf("%v %#v %+v %s", cfg.Key, cfg.Key, cfg, cfg.Key)
	assert.NotContains(t, formatted, encoded)
	assert.NotContains(t, formatted, string(raw[:]))

	cfg.Key.SetSealSecretForTest(encoded + " ")
	err = cfg.Validate()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), encoded)
	assert.NotContains(t, err.Error(), string(raw[:]))
}

func TestAutoBootstrapAllowedOnlyInDevelopmentAndTest(t *testing.T) {
	encoded, _ := canonicalSealSecret(t)
	for _, env := range []string{"development", "test"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("IDENTITY_ENV", env)
			t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
			t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
			t.Setenv("IDENTITY_KEY_ENABLED", "true")
			t.Setenv("IDENTITY_KEY_SEAL_SECRET", encoded)
			t.Setenv("IDENTITY_KEY_AUTO_BOOTSTRAP", "true")
			tokenEnv(t)
			if env == "test" {
				t.Setenv("IDENTITY_ISSUER", "https://id.example.test")
			}

			cfg, err := config.Load()
			require.NoError(t, err)
			assert.True(t, cfg.Key.AutoBootstrap)
		})
	}
}

func TestProductionRejectsAutoBootstrap(t *testing.T) {
	encoded, _ := canonicalSealSecret(t)
	productionKeyEnv(t)
	t.Setenv("IDENTITY_KEY_ENABLED", "true")
	t.Setenv("IDENTITY_KEY_SEAL_SECRET", encoded)
	t.Setenv("IDENTITY_KEY_AUTO_BOOTSTRAP", "true")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "bootstrap")
	assert.NotContains(t, err.Error(), encoded)
}

func TestLoadRejectsPartialKeyConfig(t *testing.T) {
	encoded, _ := canonicalSealSecret(t)
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")
	t.Setenv("IDENTITY_KEY_ENABLED", "false")
	t.Setenv("IDENTITY_KEY_AUTO_BOOTSTRAP", "true")
	t.Setenv("IDENTITY_KEY_SEAL_SECRET", encoded)

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "bootstrap")
	assert.NotContains(t, err.Error(), encoded)
}

func TestDevelopmentAllowsMissingSealSecretWhenCustodyDisabled(t *testing.T) {
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.False(t, cfg.Key.Enabled)
	assert.Equal(t, [32]byte{}, cfg.Key.SealKey())
}

// App HTTP signer readiness is deferred to the integration HTTP lane. This
// lane only proves config.Load/Validate fail closed so app.Run never reaches
// migrate/listen with missing or malformed production IDENTITY_KEY_*.
func TestProductionMissingOrMalformedIdentityKeyFailsLoadAndValidateBeforeMigrateListen(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "missing seal secret",
			env:  map[string]string{},
			want: "seal",
		},
		{
			name: "malformed seal secret",
			env:  map[string]string{"IDENTITY_KEY_SEAL_SECRET": "not-canonical-base64url"},
			want: "seal",
		},
		{
			name: "malformed enabled flag",
			env:  map[string]string{"IDENTITY_KEY_ENABLED": "not-a-bool"},
			want: "enabled",
		},
		{
			name: "malformed auto bootstrap flag",
			env: map[string]string{
				"IDENTITY_KEY_SEAL_SECRET":    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
				"IDENTITY_KEY_AUTO_BOOTSTRAP": "not-a-bool",
			},
			want: "bootstrap",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			productionKeyEnv(t)
			require.NoError(t, os.Unsetenv("IDENTITY_KEY_SEAL_SECRET"))
			require.NoError(t, os.Unsetenv("IDENTITY_KEY_ENABLED"))
			require.NoError(t, os.Unsetenv("IDENTITY_KEY_AUTO_BOOTSTRAP"))
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			_, err := config.Load()
			require.Error(t, err, "Load must fail before app migrate/listen")
			assert.Contains(t, strings.ToLower(err.Error()), tc.want)
			if secret, ok := tc.env["IDENTITY_KEY_SEAL_SECRET"]; ok && secret != "" {
				assert.NotContains(t, err.Error(), secret)
			}
		})
	}

	cfg := validConfig()
	cfg.Env = "production"
	cfg.Key = config.KeyConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "seal")
}

func TestNonProductionMayRemainUncomposedWithoutKeyCustody(t *testing.T) {
	for _, env := range []string{"development", "test"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("IDENTITY_ENV", env)
			t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
			issuer := "http://localhost:8090"
			if env == "test" {
				issuer = "https://id.example.test"
			}
			t.Setenv("IDENTITY_ISSUER", issuer)
			require.NoError(t, os.Unsetenv("IDENTITY_KEY_ENABLED"))
			require.NoError(t, os.Unsetenv("IDENTITY_KEY_SEAL_SECRET"))
			require.NoError(t, os.Unsetenv("IDENTITY_KEY_AUTO_BOOTSTRAP"))

			cfg, err := config.Load()
			require.NoError(t, err, "non-production may remain uncomposed")
			assert.False(t, cfg.Key.Enabled)
			assert.False(t, cfg.Key.AutoBootstrap)
			assert.Equal(t, [32]byte{}, cfg.Key.SealKey())
		})
	}
}

func TestLoadIgnoresHostileBareKeyVariables(t *testing.T) {
	const hostile = "hostile-bare-seal-secret-must-not-be-used"
	t.Setenv("KEY_ENABLED", "true")
	t.Setenv("KEY_AUTO_BOOTSTRAP", "true")
	t.Setenv("KEY_SEAL_SECRET", hostile)
	t.Setenv("SEAL_SECRET", hostile)
	t.Setenv("ENABLED", "true")
	t.Setenv("IDENTITY_ENV", "development")
	t.Setenv("IDENTITY_DATABASE_URL", "postgres://identity:***@localhost:5432/primer_identity?sslmode=disable")
	t.Setenv("IDENTITY_ISSUER", "http://localhost:8090")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.False(t, cfg.Key.Enabled)
	assert.False(t, cfg.Key.AutoBootstrap)
	assert.Equal(t, [32]byte{}, cfg.Key.SealKey())
}

func TestSealSecretUsesUnexportedWrapperAndClearsRawInput(t *testing.T) {
	encoded, raw := canonicalSealSecret(t)

	sealField, ok := reflect.TypeOf(config.KeyConfig{}).FieldByName("SealSecret")
	require.True(t, ok)
	assert.Equal(t, reflect.Struct, sealField.Type.Kind())
	for i := 0; i < sealField.Type.NumField(); i++ {
		assert.False(t, sealField.Type.Field(i).IsExported())
	}
	runtimeKeyField, ok := reflect.TypeOf(config.KeyConfig{}).FieldByName("sealKey")
	require.True(t, ok)
	assert.False(t, runtimeKeyField.IsExported())
	assert.Equal(t, reflect.TypeOf([32]byte{}), runtimeKeyField.Type)

	cfg := validConfig()
	cfg.Key.Enabled = true
	cfg.Key.SetSealSecretForTest(encoded)
	require.NoError(t, cfg.Validate())
	assert.Equal(t, raw, cfg.Key.SealKey())

	sealValue := reflect.ValueOf(cfg.Key).FieldByName("SealSecret")
	rawValue := sealValue.FieldByName("raw")
	require.True(t, rawValue.IsValid())
	assert.False(t, rawValue.CanInterface())
	assert.Empty(t, rawValue.String())
	assert.False(t, reflectedExportedConfigContains(reflect.ValueOf(*cfg), encoded, raw[:]))
}

func TestSealSecretClearsRawInputAfterValidationFailure(t *testing.T) {
	encoded, raw := canonicalSealSecret(t)
	cfg := validConfig()
	cfg.Key.Enabled = true
	cfg.Key.SetSealSecretForTest(encoded + " ")

	require.Error(t, cfg.Validate())
	sealValue := reflect.ValueOf(cfg.Key).FieldByName("SealSecret")
	assert.Empty(t, sealValue.FieldByName("raw").String())
	assert.False(t, reflectedExportedConfigContains(reflect.ValueOf(*cfg), encoded, raw[:]))
}

func TestValidatedKeyCanBeCopiedAndValidatedWithoutRawInput(t *testing.T) {
	encoded, raw := canonicalSealSecret(t)
	var key config.KeyConfig
	key.Enabled = true
	key.SetSealSecretForTest(encoded)
	require.NoError(t, key.Validate("test"))

	copied := key
	require.NoError(t, copied.Validate("test"))
	assert.Equal(t, raw, copied.SealKey())
	assert.Empty(t, reflect.ValueOf(copied).FieldByName("SealSecret").FieldByName("raw").String())
}

func TestConfigValidationClearsRawInputOnEarlyFailure(t *testing.T) {
	encoded, _ := canonicalSealSecret(t)
	cfg := validConfig()
	cfg.Key.SetSealSecretForTest(encoded)
	cfg.Env = "invalid"

	require.Error(t, cfg.Validate())
	assert.Empty(t, reflect.ValueOf(cfg.Key).FieldByName("SealSecret").FieldByName("raw").String())
}

func reflectedExportedConfigContains(v reflect.Value, encoded string, decoded []byte) bool {
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return false
		}
		return reflectedExportedConfigContains(v.Elem(), encoded, decoded)
	case reflect.String:
		return strings.Contains(v.String(), encoded) || strings.Contains(v.String(), string(decoded))
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			if reflectedExportedConfigContains(v.Field(i), encoded, decoded) {
				return true
			}
		}
	case reflect.Array, reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if v.CanInterface() {
				return strings.Contains(string(v.Bytes()), string(decoded))
			}
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if reflectedExportedConfigContains(v.Index(i), encoded, decoded) {
				return true
			}
		}
	}
	return false
}

func TestCredentialBearingConfigTypesHaveVerbIndependentSafeFormatting(t *testing.T) {
	cfg := validConfig()
	dsnPassword := "dsn-" + "password-PLANT"
	cfg.DatabaseURL = "postgres://identity:" + dsnPassword + "@db:5432/primer_identity"
	cfg.StateHashPeppers = "1:pepper-encoded-PLANT"
	cfg.Stytch.Secret = "stytch-secret-PLANT"
	seal, _ := canonicalSealSecret(t)
	cfg.Key.SetSealSecretForTest(seal)

	values := []any{*cfg, cfg, cfg.Stytch, &cfg.Stytch, cfg.Key, &cfg.Key}
	for _, value := range values {
		baseline := fmt.Sprintf("%v", value)
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%d", "%x"} {
			got := fmt.Sprintf(format, value)
			assert.Equal(t, baseline, got, "format %s for %T", format, value)
			assert.NotContains(t, got, dsnPassword)
			assert.NotContains(t, got, "pepper-encoded-PLANT")
			assert.NotContains(t, got, "stytch-secret-PLANT")
			assert.NotContains(t, got, seal)
		}
	}
}

func TestCredentialBearingConfigTypesMarshalCredentialFreeJSON(t *testing.T) {
	cfg := validConfig()
	dsnPassword := "dsn-" + "password-PLANT"
	cfg.DatabaseURL = "postgres://identity:" + dsnPassword + "@db:5432/primer_identity"
	cfg.StateHashPeppers = "1:pepper-encoded-PLANT"
	cfg.Stytch.Secret = "stytch-secret-PLANT"
	seal, _ := canonicalSealSecret(t)
	cfg.Key.SetSealSecretForTest(seal)

	for _, value := range []any{cfg, &cfg, cfg.Stytch, &cfg.Stytch, cfg.Key, &cfg.Key} {
		blob, err := json.Marshal(value)
		require.NoError(t, err, "%T must have a safe JSON projection", value)
		out := string(blob)
		assert.NotContains(t, out, dsnPassword)
		assert.NotContains(t, out, "pepper-encoded-PLANT")
		assert.NotContains(t, out, "stytch-secret-PLANT")
		assert.NotContains(t, out, seal)
	}
}

func TestRealRedactingLoggerDoesNotLeakCredentialBearingConfigValues(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewJSONLogger(&buf, "info")
	cfg := validConfig()
	dsnPassword := "dsn-" + "password-PLANT"
	cfg.DatabaseURL = "postgres://identity:" + dsnPassword + "@db:5432/primer_identity"
	cfg.StateHashPeppers = "1:pepper-encoded-PLANT"
	cfg.Stytch.Secret = "stytch-secret-PLANT"
	seal, _ := canonicalSealSecret(t)
	cfg.Key.SetSealSecretForTest(seal)

	logger.Info("credential projection", "config", cfg, "stytch", cfg.Stytch, "key", cfg.Key)
	out := buf.String()
	assert.NotContains(t, out, dsnPassword)
	assert.NotContains(t, out, "pepper-encoded-PLANT")
	assert.NotContains(t, out, "stytch-secret-PLANT")
	assert.NotContains(t, out, seal)
}
