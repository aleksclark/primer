package com.aleksclark.primer.updates

data class TvPublishedRelease(
    val available: Boolean,
    val packageName: String,
    val versionCode: Long,
    val versionName: String?,
    val sizeBytes: Long,
    val sha256: String,
    val downloadPath: String,
    val signerSha256: String?,
    val minSdk: Int?,
    val channel: String?,
    val manifestPayloadBase64: String?,
    val manifestSignature: String?,
    val signingKeyId: String?,
)

sealed interface TvReleaseDecision {
    data class Ready(val manifest: SignedManifest, val downloadPath: String) : TvReleaseDecision
    data class UpgradeRequired(val reason: String) : TvReleaseDecision
    data class Rejected(val reason: String) : TvReleaseDecision
}

object TvReleaseAdapter {
    const val TV_PACKAGE = "com.aleksclark.primer.tv"

    fun decide(release: TvPublishedRelease, trustRoot: String, installedVersion: Long): TvReleaseDecision {
        if (!release.available) return TvReleaseDecision.Rejected("No TV release is published")
        if (release.packageName.isNotBlank() && release.packageName != TV_PACKAGE) {
            return TvReleaseDecision.Rejected("APK belongs to another application")
        }
        if (release.versionCode <= installedVersion) return TvReleaseDecision.Rejected("TV release is not newer")
        val missing = mutableListOf<String>()
        if (release.sha256.isBlank() || !release.sha256.matches(Regex("[0-9a-fA-F]{64}"))) missing += "sha256"
        if (release.sizeBytes <= 0) missing += "byteSize"
        if (release.signerSha256.isNullOrBlank()) missing += "signerSha256"
        if (release.minSdk == null) missing += "minSdk"
        if (release.manifestPayloadBase64.isNullOrBlank()) missing += "manifestPayload"
        if (release.manifestSignature.isNullOrBlank()) missing += "manifestSignature"
        if (release.signingKeyId.isNullOrBlank()) missing += "signingKeyId"
        if (missing.isNotEmpty()) {
            return TvReleaseDecision.UpgradeRequired(
                "TV release metadata is missing mandatory trust fields (${missing.joinToString()}). Upgrade the TV server; verification will not be weakened.",
            )
        }
        val key = runCatching { ReleaseTrust.decodePinnedKey(trustRoot) }.getOrElse {
            return TvReleaseDecision.Rejected("Release trust root is not configured")
        }
        val payload = runCatching { java.util.Base64.getUrlDecoder().decode(release.manifestPayloadBase64) }.getOrNull()
            ?: return TvReleaseDecision.Rejected("Release manifest is missing")
        if (!ReleaseTrust.verifyEd25519(key, payload, release.manifestSignature!!)) {
            return TvReleaseDecision.Rejected("Release manifest signature is invalid")
        }
        return TvReleaseDecision.Ready(
            SignedManifest(
                packageName = TV_PACKAGE,
                channel = release.channel ?: "stable",
                versionCode = release.versionCode,
                versionName = release.versionName.orEmpty(),
                minSdk = release.minSdk!!,
                supportedAbis = emptyList(),
                signerSha256 = release.signerSha256!!,
                sha256 = release.sha256,
                byteSize = release.sizeBytes,
            ),
            downloadPath = release.downloadPath,
        )
    }
}
