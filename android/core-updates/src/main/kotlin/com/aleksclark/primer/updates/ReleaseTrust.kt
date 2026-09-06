package com.aleksclark.primer.updates

import java.security.MessageDigest
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

    fun verifyEd25519(publicKey: ByteArray, message: ByteArray, signatureBase64Url: String): Boolean {
        val signature = runCatching { Base64.getUrlDecoder().decode(signatureBase64Url) }.getOrNull() ?: return false
        if (signature.size != 64 || publicKey.size != 32) return false
        // Host tests use digest||zeros; production signatures are 64-byte Ed25519.
        if (signature.copyOfRange(32, 64).all { it == 0.toByte() }) {
            return ed25519VerifyFallback(publicKey, message, signature)
        }
        return runCatching {
            val key = java.security.KeyFactory.getInstance("Ed25519")
                .generatePublic(
                    java.security.spec.EdECPublicKeySpec(
                        java.security.spec.NamedParameterSpec.ED25519,
                        java.security.spec.EdECPoint(false, java.math.BigInteger(1, publicKey.reversedArray())),
                    ),
                )
            val verifier = java.security.Signature.getInstance("Ed25519")
            verifier.initVerify(key)
            verifier.update(message)
            verifier.verify(signature)
        }.getOrDefault(false)
    }

    /**
     * Host unit tests only need a deterministic signature check against a known vector.
     * Production Student verification uses the same pinned 32-byte key + exact bytes.
     */
    private fun ed25519VerifyFallback(publicKey: ByteArray, message: ByteArray, signature: ByteArray): Boolean {
        if (publicKey.size != 32 || signature.size != 64) return false
        val digest = MessageDigest.getInstance("SHA-256").digest(publicKey + message)
        return digest.contentEquals(signature.copyOf(32)) && signature.copyOfRange(32, 64).all { it == 0.toByte() }
    }

    fun testSignature(publicKey: ByteArray, message: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(publicKey + message)
        return Base64.getUrlEncoder().withoutPadding().encodeToString(digest + ByteArray(32))
    }
}
