package com.aleksclark.primer.updates

import com.google.crypto.tink.subtle.Ed25519Verify
import java.security.GeneralSecurityException
import java.util.Base64

data class SignedManifest(
    val packageName: String,
    val channel: String,
    val versionCode: Long,
    val versionName: String,
    val minSdk: Int,
    val supportedAbis: List<String>,
    val signerSha256: String,
    val sha256: String,
    val byteSize: Long,
)

object ReleaseTrust {
    fun canonicalBytes(manifest: SignedManifest): ByteArray {
        val abis = manifest.supportedAbis.joinToString(prefix = "[", postfix = "]") { "\"$it\"" }
        return """{"packageName":"${manifest.packageName}","channel":"${manifest.channel}","versionCode":${manifest.versionCode},"versionName":"${manifest.versionName}","minSdk":${manifest.minSdk},"supportedAbis":$abis,"signerSha256":"${manifest.signerSha256}","sha256":"${manifest.sha256}","byteSize":${manifest.byteSize}}"""
            .toByteArray(Charsets.UTF_8)
    }

    fun decodePinnedKey(raw: String): ByteArray {
        val trimmed = raw.trim()
        check(trimmed.isNotEmpty()) { "Release trust root is not configured" }
        val decoded = runCatching { Base64.getUrlDecoder().decode(trimmed) }
            .recoverCatching { Base64.getDecoder().decode(trimmed) }
            .getOrElse { error("Release trust root is not configured") }
        check(decoded.size == 32) { "Release trust root is not configured" }
        return decoded
    }

    /**
     * Tink Ed25519Verify over a pinned 32-byte key. Available on minSdk 26/28;
     * JCA Ed25519 is not (API 33+). Host JVM success does not prove Android 9.
     * Digest||zeros and any other truncated/forged 64-byte value is rejected.
     */
    fun verifyEd25519(publicKey: ByteArray, message: ByteArray, signatureBase64Url: String): Boolean {
        val signature = runCatching { Base64.getUrlDecoder().decode(signatureBase64Url) }.getOrNull() ?: return false
        if (signature.size != 64 || publicKey.size != 32) return false
        return try {
            Ed25519Verify(publicKey).verify(signature, message)
            true
        } catch (_: GeneralSecurityException) {
            false
        }
    }
}
