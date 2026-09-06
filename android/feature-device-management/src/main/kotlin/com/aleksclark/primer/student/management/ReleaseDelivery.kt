package com.aleksclark.primer.student.management

import com.aleksclark.primer.updates.ArchiveChecks
import com.aleksclark.primer.updates.ArchiveIdentity
import com.aleksclark.primer.updates.ReleaseTrust
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primertasks.client.ReleaseManifest
import com.aleksclark.primertasks.client.ReleaseTarget
import java.io.File
import java.util.Base64
import kotlinx.serialization.json.Json

data class InstallOutcome(
    val status: String,
    val versionCode: Long?,
    val error: String? = null,
)

data class ApprovedPackage(
    val packageName: String,
    val signers: Set<String>,
    val installedVersion: Long?,
)

interface RemoteReleaseSink {
    val studentVersion: Long
    val installActive: Boolean
    val pendingTargetId: String
    val pendingTargetVersion: Long
    fun stagingDir(): File
    fun installedVersion(packageName: String): Long?
    fun approvedPackage(packageName: String): ApprovedPackage?
    fun installVerified(
        file: File,
        manifest: SignedManifest,
        authorized: () -> Boolean,
        approved: ApprovedPackage?,
    ): InstallOutcome
}

object ReleaseDelivery {
    const val STUDENT_PACKAGE = "com.aleksclark.primer.student"
    const val TV_PACKAGE = "com.aleksclark.primer.tv"
    const val SIGNING_ALG = "ed25519-v1"
    private val json = Json { ignoreUnknownKeys = false; encodeDefaults = true }

    fun verify(target: ReleaseTarget, trustRoot: String): SignedManifest {
        val key = ReleaseTrust.decodePinnedKey(trustRoot)
        check(target.signingKeyId == SIGNING_ALG) { "Release trust root is not configured" }
        val payloadB64 = target.manifestPayloadBase64.orEmpty()
        val signature = target.manifestSignature.orEmpty()
        check(payloadB64.isNotBlank() && signature.isNotBlank()) { "Release manifest is missing" }
        val payload = runCatching { Base64.getUrlDecoder().decode(payloadB64) }
            .getOrElse { error("Release manifest is missing") }
        check(ReleaseTrust.verifyEd25519(key, payload, signature)) { "Release manifest signature is invalid" }
        val decoded = json.decodeFromString(ReleaseManifest.serializer(), payload.decodeToString())
        check(decoded.minSdk in 1..Int.MAX_VALUE) { "APK minSdk is out of bounds" }
        val manifest = SignedManifest(
            packageName = decoded.packageName,
            channel = decoded.channel,
            versionCode = decoded.versionCode,
            versionName = decoded.versionName,
            minSdk = decoded.minSdk.toInt(),
            supportedAbis = decoded.supportedAbis,
            signerSha256 = decoded.signerSha256,
            sha256 = decoded.sha256,
            byteSize = decoded.byteSize,
        )
        ArchiveChecks.validateExpected(manifest.byteSize, manifest.sha256)
        check(manifest.packageName == target.packageName) { "APK belongs to another application" }
        check(manifest.channel == target.channel) { "Release channel differs" }
        check(manifest.versionCode == target.versionCode) { "APK version differs from target" }
        check(manifest.versionName == target.versionName) { "APK version name differs from target" }
        check(manifest.sha256.equals(target.sha256, true)) { "APK checksum mismatch" }
        check(manifest.byteSize == target.byteSize) { "APK is incomplete" }
        if (target.minSdk != null) check(manifest.minSdk.toLong() == target.minSdk) { "APK minSdk differs from target" }
        val signer = target.signerSha256.orEmpty()
        if (signer.isNotBlank()) check(manifest.signerSha256.equals(signer, true)) { "APK signing identity differs" }
        return manifest
    }

    fun expectedArchive(manifest: SignedManifest) = ArchiveIdentity(
        packageName = manifest.packageName,
        version = manifest.versionCode,
        signers = setOf(manifest.signerSha256),
        minSdk = manifest.minSdk,
        abis = manifest.supportedAbis.toSet(),
    )
}
