package com.aleksclark.primer.updates

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TvReleaseAdapterTest {
    private fun release(
        versionCode: Long = 2,
        sha256: String = GoSignedReleaseVectors.SHA256,
        payloadBase64: String = GoSignedReleaseVectors.TV_PLAIN_PAYLOAD_BASE64URL,
        signature: String = GoSignedReleaseVectors.TV_PLAIN_SIGNATURE_BASE64URL,
        versionName: String = "0.2.0",
    ) = TvPublishedRelease(
        available = true,
        packageName = "com.aleksclark.primer.tv",
        versionCode = versionCode,
        versionName = versionName,
        sizeBytes = 12,
        sha256 = sha256,
        downloadPath = "/releases/tv.apk",
        signerSha256 = GoSignedReleaseVectors.SIGNER_SHA256,
        minSdk = 28,
        channel = "stable",
        manifestPayloadBase64 = payloadBase64,
        manifestSignature = signature,
        signingKeyId = GoSignedReleaseVectors.SIGNING_KEY_ID,
    )

    @Test
    fun missingTrustFieldsRequireServerUpgrade() {
        val decision = TvReleaseAdapter.decide(
            release().copy(signerSha256 = null, minSdk = null, manifestPayloadBase64 = null, manifestSignature = null, signingKeyId = null),
            trustRoot = GoSignedReleaseVectors.TRUST_ROOT_BASE64URL,
            installedVersion = 1,
        )
        assertTrue(decision is TvReleaseDecision.UpgradeRequired)
    }

    @Test
    fun usesGoSignedPayloadNotUnsignedOuterFields() {
        val ready = TvReleaseAdapter.decide(release(), GoSignedReleaseVectors.TRUST_ROOT_BASE64URL, 1)
        assertTrue(ready is TvReleaseDecision.Ready)
        assertEquals("com.aleksclark.primer.tv", (ready as TvReleaseDecision.Ready).manifest.packageName)
        assertEquals(2, ready.manifest.versionCode)
        val tamperedOuter = TvReleaseAdapter.decide(release(versionCode = 99), GoSignedReleaseVectors.TRUST_ROOT_BASE64URL, 1)
        assertTrue(tamperedOuter is TvReleaseDecision.Rejected)
    }
}
