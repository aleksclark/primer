package com.aleksclark.primer.tv.app.update

import com.aleksclark.primer.tv.core.domain.AppRelease
import com.aleksclark.primer.updates.GoSignedReleaseVectors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TvUpdateGateTest {
    private val trust = GoSignedReleaseVectors.TRUST_ROOT_BASE64URL

    private fun release(
        available: Boolean = true,
        versionCode: Int = 2,
        sha256: String = GoSignedReleaseVectors.SHA256,
        sizeBytes: Long = 12,
        signed: Boolean = true,
        versionName: String = "0.2.0",
        payloadBase64: String = GoSignedReleaseVectors.TV_PLAIN_PAYLOAD_BASE64URL,
        signature: String = GoSignedReleaseVectors.TV_PLAIN_SIGNATURE_BASE64URL,
    ) = AppRelease(
        available = available,
        versionCode = versionCode,
        sizeBytes = sizeBytes,
        sha256 = sha256,
        downloadPath = "/api/v1/app/release/apk",
        packageName = "com.aleksclark.primer.tv",
        versionName = versionName,
        signerSha256 = if (signed) GoSignedReleaseVectors.SIGNER_SHA256 else null,
        minSdk = if (signed) 28 else null,
        channel = if (signed) "stable" else null,
        manifestPayloadBase64 = if (signed) payloadBase64 else null,
        manifestSignature = if (signed) signature else null,
        signingKeyId = if (signed) GoSignedReleaseVectors.SIGNING_KEY_ID else null,
    )

    @Test
    fun currentUnsignedMetadataRequiresServerUpgrade() {
        val next = TvUpdateGate.stateFor(release(signed = false), trust, installedVersion = 1)
        assertTrue(next is UpdateState.Failed)
        assertTrue((next as UpdateState.Failed).message.contains("Upgrade the TV server"))
    }

    @Test
    fun blankDigestOrSizeIsNotInstallable() {
        assertTrue(TvUpdateGate.stateFor(release(sha256 = "", signed = false), trust, 1) is UpdateState.Failed)
        assertTrue(TvUpdateGate.stateFor(release(sizeBytes = 0, signed = false), trust, 1) is UpdateState.Failed)
    }

    @Test
    fun verifiedGoSignedPayloadIsOfferedAndTamperedOuterFieldsAreRejected() {
        val available = TvUpdateGate.stateFor(release(), trust, 1)
        assertTrue(available is UpdateState.Available)
        assertEquals(2L, (available as UpdateState.Available).manifest.versionCode)
        assertTrue(TvUpdateGate.stateFor(release(versionCode = 99), trust, 1) is UpdateState.Failed)
    }

    @Test
    fun unpublishedOrCurrentVersionStaysUpToDate() {
        assertTrue(TvUpdateGate.stateFor(release(available = false, signed = false), trust, 1) is UpdateState.UpToDate)
        assertTrue(TvUpdateGate.stateFor(release(versionCode = 1, signed = false), trust, 1) is UpdateState.UpToDate)
    }
}
