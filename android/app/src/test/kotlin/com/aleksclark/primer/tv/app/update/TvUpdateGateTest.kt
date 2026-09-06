package com.aleksclark.primer.tv.app.update

import com.aleksclark.primer.tv.core.domain.AppRelease
import com.aleksclark.primer.updates.ReleaseTrust
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

class TvUpdateGateTest {
    private val keys = ReleaseTrust.newKeyPair()
    private val trust = Base64.getUrlEncoder().withoutPadding().encodeToString(keys.first)
    private val payload = """{"packageName":"com.aleksclark.primer.tv","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"${"a".repeat(64)}","sha256":"${"b".repeat(64)}","byteSize":12}""".toByteArray()
    private val signature = ReleaseTrust.sign(keys.second, payload)

    private fun release(
        available: Boolean = true,
        versionCode: Int = 2,
        sha256: String = "b".repeat(64),
        sizeBytes: Long = 12,
        signed: Boolean = true,
    ) = AppRelease(
        available = available,
        versionCode = versionCode,
        sizeBytes = sizeBytes,
        sha256 = sha256,
        downloadPath = "/api/v1/app/release/apk",
        packageName = "com.aleksclark.primer.tv",
        versionName = "0.2.0",
        signerSha256 = if (signed) "a".repeat(64) else null,
        minSdk = if (signed) 28 else null,
        channel = if (signed) "stable" else null,
        manifestPayloadBase64 = if (signed) Base64.getUrlEncoder().withoutPadding().encodeToString(payload) else null,
        manifestSignature = if (signed) signature else null,
        signingKeyId = if (signed) "ed25519-v1" else null,
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
    fun verifiedPayloadIsOfferedAndTamperedOuterFieldsAreRejected() {
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
