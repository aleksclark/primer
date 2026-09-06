package com.aleksclark.primer.updates

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

class TvReleaseAdapterTest {
    private val key = ByteArray(32) { it.toByte() }
    private val trust = Base64.getUrlEncoder().withoutPadding().encodeToString(key)
    private val payload = """{"packageName":"com.aleksclark.primer.tv","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"${"a".repeat(64)}","sha256":"${"b".repeat(64)}","byteSize":12}""".toByteArray()
    private val signature = ReleaseTrust.testSignature(key, payload)

    private fun release(
        versionCode: Long = 2,
        sha256: String = "b".repeat(64),
        payloadBytes: ByteArray = payload,
        signatureValue: String = signature,
    ) = TvPublishedRelease(
        available = true,
        packageName = "com.aleksclark.primer.tv",
        versionCode = versionCode,
        versionName = "0.2.0",
        sizeBytes = 12,
        sha256 = sha256,
        downloadPath = "/releases/tv.apk",
        signerSha256 = "a".repeat(64),
        minSdk = 28,
        channel = "stable",
        manifestPayloadBase64 = Base64.getUrlEncoder().withoutPadding().encodeToString(payloadBytes),
        manifestSignature = signatureValue,
        signingKeyId = "ed25519-v1",
    )

    @Test
    fun missingTrustFieldsRequireServerUpgrade() {
        val decision = TvReleaseAdapter.decide(
            release().copy(signerSha256 = null, minSdk = null, manifestPayloadBase64 = null, manifestSignature = null, signingKeyId = null),
            trustRoot = trust,
            installedVersion = 1,
        )
        assertTrue(decision is TvReleaseDecision.UpgradeRequired)
    }

    @Test
    fun usesVerifiedPayloadNotUnsignedOuterFields() {
        val ready = TvReleaseAdapter.decide(release(), trust, 1)
        assertTrue(ready is TvReleaseDecision.Ready)
        assertEquals("com.aleksclark.primer.tv", (ready as TvReleaseDecision.Ready).manifest.packageName)
        assertEquals(2, ready.manifest.versionCode)
        val tamperedOuter = TvReleaseAdapter.decide(release(versionCode = 99), trust, 1)
        assertTrue(tamperedOuter is TvReleaseDecision.Rejected)
    }
}
