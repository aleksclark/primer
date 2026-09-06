package com.aleksclark.primer.updates

import android.os.Build
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Device/emulator proof that Tink Ed25519Verify accepts RFC 8032 and
 * Go-produced signatures on the declared minSdk. JVM unit tests do not
 * prove Android 9. Parent-owned; this lane does not run adb.
 */
@RunWith(AndroidJUnit4::class)
class ReleaseTrustAndroidTest {
    @Test
    fun rfc8032AndGoProducedManifestVerifyOnDevice() {
        assertTrue("instrumented proof requires API 28+", Build.VERSION.SDK_INT >= 28)
        val publicKey = GoSignedReleaseVectors.publicKey()
        assertTrue(
            ReleaseTrust.verifyEd25519(
                publicKey,
                ByteArray(0),
                GoSignedReleaseVectors.emptySignatureBase64Url(),
            ),
        )
        val payload = GoSignedReleaseVectors.TV_PAYLOAD.toByteArray(Charsets.UTF_8)
        assertTrue(ReleaseTrust.verifyEd25519(publicKey, payload, GoSignedReleaseVectors.TV_SIGNATURE_BASE64URL))
        assertFalse(
            ReleaseTrust.verifyEd25519(
                publicKey,
                payload,
                GoSignedReleaseVectors.digestZerosForgery(publicKey, payload),
            ),
        )
        SignedManifestCodec.verifyEnvelope(
            trustRoot = GoSignedReleaseVectors.TRUST_ROOT_BASE64URL,
            payloadBase64 = GoSignedReleaseVectors.TV_PAYLOAD_BASE64URL,
            signature = GoSignedReleaseVectors.TV_SIGNATURE_BASE64URL,
            signingKeyId = GoSignedReleaseVectors.SIGNING_KEY_ID,
        )
    }
}
