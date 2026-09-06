package com.aleksclark.primer.updates

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

class ReleaseTrustTest {
    private val keys = ReleaseTrust.newKeyPair()
    private val publicKey = keys.first
    private val privateKey = keys.second
    private val manifest = SignedManifest(
        packageName = "com.aleksclark.primer.student",
        channel = "stable",
        versionCode = 2,
        versionName = "0.2.0",
        minSdk = 28,
        supportedAbis = listOf("arm64-v8a"),
        signerSha256 = "a".repeat(64),
        sha256 = "b".repeat(64),
        byteSize = 12,
    )

    @Test
    fun missingTrustKeyFailsClosed() {
        org.junit.Assert.assertThrows(IllegalStateException::class.java) { ReleaseTrust.decodePinnedKey("") }
        org.junit.Assert.assertThrows(IllegalStateException::class.java) { ReleaseTrust.decodePinnedKey("abc") }
    }

    @Test
    fun tinkEd25519VerifiesCanonicalBytesOnPinnedKey() {
        val encoded = Base64.getUrlEncoder().withoutPadding().encodeToString(publicKey)
        assertEquals(32, ReleaseTrust.decodePinnedKey(encoded).size)
        val message = ReleaseTrust.canonicalBytes(manifest)
        val signature = ReleaseTrust.sign(privateKey, message)
        assertTrue(ReleaseTrust.verifyEd25519(publicKey, message, signature))
        assertFalse(ReleaseTrust.verifyEd25519(publicKey, message + 1, signature))
        assertFalse(ReleaseTrust.verifyEd25519(publicKey, message, ReleaseTrust.sign(privateKey, message + 1)))
        val digestFallback = java.security.MessageDigest.getInstance("SHA-256").digest(publicKey + message) + ByteArray(32)
        assertFalse(
            ReleaseTrust.verifyEd25519(
                publicKey,
                message,
                Base64.getUrlEncoder().withoutPadding().encodeToString(digestFallback),
            ),
        )
    }
}
