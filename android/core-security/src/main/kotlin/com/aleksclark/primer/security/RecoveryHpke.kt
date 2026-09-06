package com.aleksclark.primer.security

import com.google.crypto.tink.HybridDecrypt
import com.google.crypto.tink.HybridEncrypt
import com.google.crypto.tink.KeyTemplates
import com.google.crypto.tink.KeysetHandle
import com.google.crypto.tink.TinkJsonProtoKeysetFormat
import com.google.crypto.tink.hybrid.HybridConfig
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.util.Base64

/**
 * Shared Tink HPKE helper for management recovery envelopes.
 *
 * Control encrypts; Student decrypts. Do not invent X25519/KDF/cipher.
 * Do not use AndroidKeysetManager (it can fall back to plaintext).
 */
object RecoveryHpke {
    const val ALG = "TINK-HPKE-X25519-HKDF-SHA256-CHACHA20POLY1305-RAW-v1"
    const val TEMPLATE = "DHKEM_X25519_HKDF_SHA256_HKDF_SHA256_CHACHA20_POLY1305_RAW"
    const val CONTEXT_PREFIX = "primer-management/recovery/v1"
    const val PAYLOAD_VERSION = 1

    init {
        HybridConfig.register()
    }

    fun generatePrivateHandle(): KeysetHandle =
        KeysetHandle.generateNew(KeyTemplates.get(TEMPLATE))

    fun publicKeysetJson(privateHandle: KeysetHandle): ByteArray {
        val publicHandle = privateHandle.publicKeysetHandle
        return TinkJsonProtoKeysetFormat.serializeKeysetWithoutSecret(publicHandle)
            .toByteArray(StandardCharsets.UTF_8)
    }

    fun serializeCleartextPrivate(privateHandle: KeysetHandle): ByteArray =
        TinkJsonProtoKeysetFormat.serializeKeyset(privateHandle, secretKeyAccess())
            .toByteArray(StandardCharsets.UTF_8)

    fun parseCleartextPrivate(jsonUtf8: ByteArray): KeysetHandle =
        TinkJsonProtoKeysetFormat.parseKeyset(String(jsonUtf8, StandardCharsets.UTF_8), secretKeyAccess())

    fun parsePublicKeyset(jsonUtf8: ByteArray): KeysetHandle =
        TinkJsonProtoKeysetFormat.parseKeysetWithoutSecret(String(jsonUtf8, StandardCharsets.UTF_8))

    /** base64url-no-padding of the UTF-8 Tink public-keyset JSON. */
    fun encodePublicEnrollmentKey(publicJsonUtf8: ByteArray): String =
        Base64.getUrlEncoder().withoutPadding().encodeToString(publicJsonUtf8)

    fun decodePublicEnrollmentKey(encoded: String): ByteArray =
        Base64.getUrlDecoder().decode(encoded)

    /** SHA-256 of the exact decoded public-keyset JSON bytes, lowercase hex. */
    fun keyId(publicJsonUtf8: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(publicJsonUtf8).joinToString("") { "%02x".format(it) }

    fun contextInfo(deviceId: String, requestId: String, keyId: String): ByteArray {
        require(deviceId.isNotBlank() && requestId.isNotBlank() && keyId.isNotBlank())
        return listOf(CONTEXT_PREFIX, deviceId, requestId, keyId)
            .joinToString("\u0000")
            .toByteArray(StandardCharsets.UTF_8)
    }

    fun encrypt(
        publicJsonUtf8: ByteArray,
        plaintext: ByteArray,
        deviceId: String,
        requestId: String,
    ): RecoveryCiphertext {
        val keyId = keyId(publicJsonUtf8)
        val handle = parsePublicKeyset(publicJsonUtf8)
        val hybrid = handle.getPrimitive(HybridEncrypt::class.java)
        val ciphertext = hybrid.encrypt(plaintext, contextInfo(deviceId, requestId, keyId))
        return RecoveryCiphertext(
            keyId = keyId,
            alg = ALG,
            ciphertext = Base64.getUrlEncoder().withoutPadding().encodeToString(ciphertext),
        )
    }

    fun decrypt(
        privateHandle: KeysetHandle,
        envelope: RecoveryCiphertext,
        deviceId: String,
        requestId: String,
        expectedKeyId: String,
    ): ByteArray {
        check(envelope.alg == ALG) { "Unsupported recovery envelope algorithm" }
        check(envelope.keyId == expectedKeyId) { "Envelope keyId does not match enrolled device key" }
        val hybrid = privateHandle.getPrimitive(HybridDecrypt::class.java)
        val ciphertext = Base64.getUrlDecoder().decode(envelope.ciphertext)
        return hybrid.decrypt(ciphertext, contextInfo(deviceId, requestId, expectedKeyId))
    }

    fun encodePayload(payload: RecoveryPayload): ByteArray {
        require(payload.version == PAYLOAD_VERSION)
        require(payload.codes.size == 6)
        require(payload.codes.toSet().size == 6)
        return buildString {
            append('{')
            append("\"version\":").append(payload.version).append(',')
            append("\"deviceId\":\"").append(jsonEscape(payload.deviceId)).append("\",")
            append("\"requestId\":\"").append(jsonEscape(payload.requestId)).append("\",")
            append("\"keyId\":\"").append(jsonEscape(payload.keyId)).append("\",")
            append("\"codes\":[")
            append(payload.codes.joinToString(",") { "\"${jsonEscape(it)}\"" })
            append("]}")
        }.toByteArray(StandardCharsets.UTF_8)
    }

    fun decodePayload(bytes: ByteArray): RecoveryPayload {
        val text = String(bytes, StandardCharsets.UTF_8)
        val version = requiredInt(text, "version")
        check(version == PAYLOAD_VERSION) { "Unsupported recovery payload version" }
        val codes = Regex("\"codes\"\\s*:\\s*\\[(.*?)]", RegexOption.DOT_MATCHES_ALL)
            .find(text)
            ?.groupValues?.get(1)
            ?.let { body -> Regex("\"((?:\\\\.|[^\"\\\\])*)\"").findAll(body).map { it.groupValues[1] }.toList() }
            ?: error("Recovery payload missing codes")
        return RecoveryPayload(
            version = version,
            deviceId = requiredString(text, "deviceId"),
            requestId = requiredString(text, "requestId"),
            keyId = requiredString(text, "keyId"),
            codes = codes,
        )
    }

    fun encryptCodes(
        publicJsonUtf8: ByteArray,
        deviceId: String,
        requestId: String,
        codes: List<String>,
    ): RecoveryCiphertext {
        val keyId = keyId(publicJsonUtf8)
        val payload = RecoveryPayload(PAYLOAD_VERSION, deviceId, requestId, keyId, codes)
        return encrypt(publicJsonUtf8, encodePayload(payload), deviceId, requestId)
    }

    fun decryptCodes(
        privateHandle: KeysetHandle,
        envelope: RecoveryCiphertext,
        deviceId: String,
        requestId: String,
        expectedKeyId: String,
    ): RecoveryPayload {
        val payload = decodePayload(decrypt(privateHandle, envelope, deviceId, requestId, expectedKeyId))
        check(payload.deviceId == deviceId) { "Recovery payload deviceId mismatch" }
        check(payload.requestId == requestId) { "Recovery payload requestId mismatch" }
        check(payload.keyId == expectedKeyId) { "Recovery payload keyId mismatch" }
        return payload
    }

    private fun secretKeyAccess(): com.google.crypto.tink.SecretKeyAccess =
        com.google.crypto.tink.InsecureSecretKeyAccess.get()

    private fun jsonEscape(value: String): String = buildString {
        value.forEach { ch ->
            when (ch) {
                '\\' -> append("\\\\")
                '"' -> append("\\\"")
                else -> append(ch)
            }
        }
    }

    private fun requiredString(json: String, key: String): String {
        val match = Regex("\"$key\"\\s*:\\s*\"((?:\\\\.|[^\"\\\\])*)\"").find(json)
            ?: error("Recovery payload missing $key")
        return match.groupValues[1]
    }

    private fun requiredInt(json: String, key: String): Int {
        val match = Regex("\"$key\"\\s*:\\s*(-?\\d+)").find(json)
            ?: error("Recovery payload missing $key")
        return match.groupValues[1].toInt()
    }
}

data class RecoveryCiphertext(
    val keyId: String,
    val alg: String,
    val ciphertext: String,
)

data class RecoveryPayload(
    val version: Int,
    val deviceId: String,
    val requestId: String,
    val keyId: String,
    val codes: List<String>,
)
