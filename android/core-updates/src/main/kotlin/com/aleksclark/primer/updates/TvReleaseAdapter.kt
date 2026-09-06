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
        val manifest = runCatching {
            SignedManifestCodec.verifyEnvelope(
                trustRoot = trustRoot,
                payloadBase64 = release.manifestPayloadBase64!!,
                signature = release.manifestSignature!!,
                signingKeyId = release.signingKeyId!!,
            )
        }.getOrElse { return TvReleaseDecision.Rejected(it.message ?: "Release manifest is invalid") }
        return try {
            check(manifest.packageName == TV_PACKAGE) { "APK belongs to another application" }
            check(manifest.packageName == release.packageName || release.packageName.isBlank()) { "APK belongs to another application" }
            check(manifest.versionCode == release.versionCode) { "APK version differs from target" }
            if (!release.versionName.isNullOrBlank()) check(manifest.versionName == release.versionName) { "APK version name differs from target" }
            check(manifest.sha256.equals(release.sha256, true)) { "APK checksum mismatch" }
            check(manifest.byteSize == release.sizeBytes) { "APK is incomplete" }
            check(manifest.signerSha256.equals(release.signerSha256, true)) { "APK signing identity differs" }
            check(manifest.minSdk == release.minSdk) { "APK minSdk differs from target" }
            if (!release.channel.isNullOrBlank()) check(manifest.channel == release.channel) { "Release channel differs" }
            TvReleaseDecision.Ready(manifest, release.downloadPath)
        } catch (error: IllegalStateException) {
            TvReleaseDecision.Rejected(error.message ?: "TV release metadata does not match signed payload")
        }
    }
}
