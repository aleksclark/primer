package com.aleksclark.primer.updates

import com.aleksclark.primer.updates.generated.ReleaseManifest
import java.util.Base64
import kotlinx.serialization.json.Json

/**
 * Maps the generated OpenAPI ReleaseManifest wire document onto the domain
 * [SignedManifest]. This is not a Tasks client DTO and is not a handwritten
 * copy of Go fields.
 */
object SignedManifestCodec {
    private val json = Json { ignoreUnknownKeys = false; encodeDefaults = true }

    fun parseVerified(payload: ByteArray): SignedManifest {
        val decoded = json.decodeFromString(ReleaseManifest.serializer(), payload.decodeToString())
        check(decoded.minSdk in 1..Int.MAX_VALUE) { "APK minSdk is out of bounds" }
        ArchiveChecks.validateExpected(decoded.byteSize, decoded.sha256)
        check(decoded.signerSha256.matches(Regex("[0-9a-fA-F]{64}"))) { "APK signing identity differs" }
        check(decoded.packageName.isNotBlank()) { "APK belongs to another application" }
        return SignedManifest(
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
    }

    fun verifyEnvelope(trustRoot: String, payloadBase64: String, signature: String, signingKeyId: String): SignedManifest {
        check(signingKeyId == "ed25519-v1") { "Release trust root is not configured" }
        val key = ReleaseTrust.decodePinnedKey(trustRoot)
        val payload = runCatching { Base64.getUrlDecoder().decode(payloadBase64) }.getOrElse {
            error("Release manifest is missing")
        }
        check(ReleaseTrust.verifyEd25519(key, payload, signature)) { "Release manifest signature is invalid" }
        return parseVerified(payload)
    }
}
