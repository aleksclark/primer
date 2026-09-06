# feature-device-management

Student-side management enrollment adapter.

- QR: `primer-management:v1:{origin}{mount}/management-device/enroll#{32-hex-code}`
- Enrollment only from parent setup/maintenance. No silent household rebinding.
- Management bearer is a separate Keystore-backed store from Tasks.
- Remote recovery lease is delivery-expiry anchored on serverTime; reboot/expiry cannot reopen a cached lease.
- Policy sanitizer in `:core-device-policy` refuses dropping Student or applying mismatched signers.

Transport methods (`managementDeviceEnroll/desired/report`) come from `:tasks-client`. Artifact streaming is owned by the client worker; this module does not copy URLs or generated DTOs.
