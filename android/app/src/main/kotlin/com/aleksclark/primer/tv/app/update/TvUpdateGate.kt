package com.aleksclark.primer.tv.app.update

import com.aleksclark.primer.tv.core.domain.AppRelease
import com.aleksclark.primer.updates.TvPublishedRelease
import com.aleksclark.primer.updates.TvReleaseAdapter
import com.aleksclark.primer.updates.TvReleaseDecision

internal fun AppRelease.toPublished(): TvPublishedRelease = TvPublishedRelease(
    available = available,
    packageName = packageName,
    versionCode = versionCode.toLong(),
    versionName = versionName,
    sizeBytes = sizeBytes,
    sha256 = sha256,
    downloadPath = downloadPath,
    signerSha256 = signerSha256,
    minSdk = minSdk,
    channel = channel,
    manifestPayloadBase64 = manifestPayloadBase64,
    manifestSignature = manifestSignature,
    signingKeyId = signingKeyId,
)

internal object TvUpdateGate {
    fun stateFor(release: AppRelease, trustRoot: String, installedVersion: Long): UpdateState =
        when (val decision = TvReleaseAdapter.decide(release.toPublished(), trustRoot, installedVersion)) {
            is TvReleaseDecision.Ready -> UpdateState.Available(release, decision.manifest)
            is TvReleaseDecision.UpgradeRequired -> UpdateState.Failed(decision.reason)
            is TvReleaseDecision.Rejected -> when {
                !release.available || release.versionCode.toLong() <= installedVersion -> UpdateState.UpToDate
                else -> UpdateState.Failed(decision.reason)
            }
        }
}
