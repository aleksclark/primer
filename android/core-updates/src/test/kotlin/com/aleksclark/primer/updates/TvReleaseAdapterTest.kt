package com.aleksclark.primer.updates

import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

class TvReleaseAdapterTest {
    private val key = ByteArray(32) { it.toByte() }
    private val trust = Base64.getUrlEncoder().withoutPadding().encodeToString(key)

    @Test
    fun missingTrustFieldsRequireServerUpgrade() {
        val decision = TvReleaseAdapter.decide(
            TvPublishedRelease(
                available = true,
                packageName = "com.aleksclark.primer.tv",
                versionCode = 2,
                versionName = "0.2.0",
                sizeBytes = 12,
                sha256 = "b".repeat(64),
                downloadPath = "/releases/tv.apk",
                signerSha256 = null,
                minSdk = null,
                channel = null,
                manifestPayloadBase64 = null,
                manifestSignature = null,
                signingKeyId = null,
            ),
            trustRoot = trust,
            installedVersion = 1,
        )
        assertTrue(decision is TvReleaseDecision.UpgradeRequired)
    }

    @Test
    fun blankChecksumIsRejectedNotTreatedAsOptional() {
        val decision = TvReleaseAdapter.decide(
            TvPublishedRelease(
                available = true,
                packageName = "com.aleksclark.primer.tv",
                versionCode = 2,
                versionName = "0.2.0",
                sizeBytes = 12,
                sha256 = "",
                downloadPath = "/releases/tv.apk",
                signerSha256 = "a".repeat(64),
                minSdk = 28,
                channel = "stable",
                manifestPayloadBase64 = "payload",
                manifestSignature = "sig",
                signingKeyId = "ed25519-v1",
            ),
            trustRoot = trust,
            installedVersion = 1,
        )
        assertTrue(decision is TvReleaseDecision.UpgradeRequired)
    }
}
