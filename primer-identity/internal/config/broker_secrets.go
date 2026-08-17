package config

import (
	"errors"
)

// ErrBrokerSecretsUnavailable is the single non-oracular composition failure.
// It never contains supplied key material, versions, or environment values.
var ErrBrokerSecretsUnavailable = errors.New("identity config: broker secret material is unavailable")

// BrokerSecretSet is the composed, copied IB1 secret material. Every value is
// exactly 32 bytes and every active version resolves to present material.
type BrokerSecretSet struct {
	StateSealKeys          map[int][]byte
	StateSealActiveVersion int

	StateHashPeppers       map[int][]byte
	StateHashActiveVersion int

	BrokerCookiePeppers       map[int][]byte
	BrokerCookieActiveVersion int

	AuthorizationCodePeppers       map[int][]byte
	AuthorizationCodeActiveVersion int
}

// BrokerSecrets composes every versioned seal key and secret pepper the IB1
// broker requires. It is environment-independent so credential-free
// development and test runs use the same fail-closed path as production, and
// it must be called before the process listens.
//
// Only IDENTITY_-prefixed values feed this composition; envconfig Alt fallback
// stays disabled so bare names cannot supply Identity secrets. Returned maps
// and slices are defensive copies.
func (c *Config) BrokerSecrets() (BrokerSecretSet, error) {
	sealKeys, err := copiedSecretSet(c.StateSealKeys, c.StateSealActiveVersion)
	if err != nil {
		return BrokerSecretSet{}, err
	}
	stateHash, err := copiedSecretSet(c.StateHashPeppers, c.StateHashActiveVersion)
	if err != nil {
		return BrokerSecretSet{}, err
	}
	cookie, err := copiedSecretSet(c.BrokerCookiePeppers, c.BrokerCookieActiveVersion)
	if err != nil {
		return BrokerSecretSet{}, err
	}
	code, err := copiedSecretSet(c.AuthorizationCodePeppers, c.AuthorizationCodeActiveVersion)
	if err != nil {
		return BrokerSecretSet{}, err
	}
	return BrokerSecretSet{
		StateSealKeys: sealKeys, StateSealActiveVersion: c.StateSealActiveVersion,
		StateHashPeppers: stateHash, StateHashActiveVersion: c.StateHashActiveVersion,
		BrokerCookiePeppers: cookie, BrokerCookieActiveVersion: c.BrokerCookieActiveVersion,
		AuthorizationCodePeppers: code, AuthorizationCodeActiveVersion: c.AuthorizationCodeActiveVersion,
	}, nil
}

// copiedSecretSet parses, validates, and deep-copies one versioned set. Every
// failure collapses to ErrBrokerSecretsUnavailable so no parse detail, version,
// or byte of material can leak through an error string.
func copiedSecretSet(encoded string, active int) (map[int][]byte, error) {
	parsed, err := parseVersionedSecrets(encoded)
	if err != nil {
		return nil, ErrBrokerSecretsUnavailable
	}
	if err := requireActive(parsed, active); err != nil {
		return nil, ErrBrokerSecretsUnavailable
	}
	out := make(map[int][]byte, len(parsed.Values))
	for version, material := range parsed.Values {
		if len(material) != 32 {
			return nil, ErrBrokerSecretsUnavailable
		}
		out[version] = append([]byte(nil), material...)
	}
	return out, nil
}
