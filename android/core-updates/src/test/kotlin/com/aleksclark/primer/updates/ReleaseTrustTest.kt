package com.aleksclark.primer.updates

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

class ReleaseTrustTest {
    private val publicKey = GoSignedReleaseVectors.publicKey()

    @Test
    fun missingTrustKeyFailsClosed() {
        org.junit.Assert.assertThrows(IllegalStateException::class.java) { ReleaseTrust.decodePinnedKey("") }
        org.junit.Assert.assertThrows(IllegalStateException::class.java) { ReleaseTrust.decodePinnedKey("abc") }
    }

    @Test
    fun rfc8032Test1EmptyMessageVerifiesAndForgeryIsRejected() {
        assertEquals(32, ReleaseTrust.decodePinnedKey(GoSignedReleaseVectors.TRUST_ROOT_BASE64URL).size)
        val emptySig = GoSignedReleaseVectors.emptySignatureBase64Url()
        assertTrue(ReleaseTrust.verifyEd25519(publicKey, ByteArray(0), emptySig))
        assertFalse(ReleaseTrust.verifyEd25519(publicKey, byteArrayOf(1), emptySig))
        assertFalse(
            ReleaseTrust.verifyEd25519(
                publicKey,
                ByteArray(0),
                GoSignedReleaseVectors.digestZerosForgery(publicKey, ByteArray(0)),
            ),
        )
    }

    @Test
    fun goProducedReleaseManifestVerifiesAndDigestZerosIsRejected() {
        val payload = GoSignedReleaseVectors.TV_PAYLOAD.toByteArray(Charsets.UTF_8)
        assertTrue(ReleaseTrust.verifyEd25519(publicKey, payload, GoSignedReleaseVectors.TV_SIGNATURE_BASE64URL))
        assertFalse(ReleaseTrust.verifyEd25519(publicKey, payload + 1, GoSignedReleaseVectors.TV_SIGNATURE_BASE64URL))
        assertFalse(
            ReleaseTrust.verifyEd25519(
                publicKey,
                payload,
                GoSignedReleaseVectors.digestZerosForgery(publicKey, payload),
            ),
        )
        val decoded = Base64.getUrlDecoder().decode(GoSignedReleaseVectors.TV_PAYLOAD_BASE64URL)
        assertTrue(decoded.contentEquals(payload))
        val manifest = SignedManifestCodec.verifyEnvelope(
            trustRoot = GoSignedReleaseVectors.TRUST_ROOT_BASE64URL,
            payloadBase64 = GoSignedReleaseVectors.TV_PAYLOAD_BASE64URL,
            signature = GoSignedReleaseVectors.TV_SIGNATURE_BASE64URL,
            signingKeyId = GoSignedReleaseVectors.SIGNING_KEY_ID,
        )
        assertEquals("com.aleksclark.primer.tv", manifest.packageName)
        assertEquals("Primer \"N+1\" 测试", manifest.versionName)
    }
}
