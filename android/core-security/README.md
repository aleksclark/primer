# core-security

Shared Tink HPKE helper for Primer management recovery.

Public API for Control (`:app-control`) and Student:

- `RecoveryHpke.ALG` = `TINK-HPKE-X25519-HKDF-SHA256-CHACHA20POLY1305-RAW-v1`
- `RecoveryHpke.TEMPLATE` = `DHKEM_X25519_HKDF_SHA256_HKDF_SHA256_CHACHA20_POLY1305_RAW`
- `RecoveryHpke.generatePrivateHandle()`
- `RecoveryHpke.publicKeysetJson(handle)` UTF-8 Tink public-keyset JSON
- `RecoveryHpke.encodePublicEnrollmentKey(json)` base64url-no-padding
- `RecoveryHpke.keyId(json)` SHA-256 of exact decoded public bytes
- `RecoveryHpke.encryptCodes(publicJson, deviceId, requestId, codes)`
- Envelope `{keyId, alg, ciphertext}` — no caller-managed nonce
- contextInfo = UTF-8 `"primer-management/recovery/v1\\0"+deviceId+"\\0"+requestId+"\\0"+keyId`
- Persist private keyset with `AndroidKeystoreKeyset.wrap/unwrap` (`serializeEncryptedKeyset` + Keystore AEAD). Fail closed. Do not use `AndroidKeysetManager`.

Request UUID is the server intent ID and must be known before encryption.
