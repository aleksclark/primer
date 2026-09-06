package com.aleksclark.primer.student.management

import com.aleksclark.primer.updates.ReleaseTrust
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primertasks.client.ReleaseTarget
import java.io.File
import java.util.Base64

interface RemoteReleaseSink {
    val studentVersion: Long
    val installActive: Boolean
    val pendingTargetId: String
    val pendingTargetVersion: Long
    fun remember(targetId: String, targetVersion: Long)
    fun stagingDir(): File
    fun installVerified(file: File, size: Long): String
}

object ReleaseDelivery {
    const val STUDENT_PACKAGE = "com.aleksclark.primer.student"
    const val SIGNING_ALG = "ed25519-v1"

    fun verify(target: ReleaseTarget, trustRoot: String): SignedManifest {
        check(target.packageName == STUDENT_PACKAGE) { "APK belongs to another application" }
        val key = ReleaseTrust.decodePinnedKey(trustRoot)
        check(target.signingKeyId == SIGNING_ALG) { "Release trust root is not configured" }
        val payloadB64 = target.manifestPayloadBase64.orEmpty()
        val signature = target.manifestSignature.orEmpty()
        check(payloadB64.isNotBlank() && signature.isNotBlank()) { "Release manifest is missing" }
        val payload = runCatching { Base64.getUrlDecoder().decode(payloadB64) }
            .getOrElse { error("Release manifest is missing") }
        check(ReleaseTrust.verifyEd25519(key, payload, signature)) { "Release manifest signature is invalid" }
        val text = payload.toString(Charsets.UTF_8)
        val manifest = parseCanonical(text)
        check(manifest.packageName == target.packageName) { "APK belongs to another application" }
        check(manifest.versionCode == target.versionCode) { "APK must have a newer version code" }
        check(manifest.sha256.equals(target.sha256, true)) { "APK checksum mismatch" }
        check(manifest.byteSize == target.byteSize) { "APK is incomplete" }
        val signer = target.signerSha256.orEmpty()
        if (signer.isNotBlank()) check(manifest.signerSha256.equals(signer, true)) { "APK signing identity differs" }
        check(ReleaseTrust.canonicalBytes(manifest).contentEquals(payload)) { "Release manifest is not canonical" }
        return manifest
    }

    private fun parseCanonical(raw: String): SignedManifest {
        val packageName = stringField(raw, "packageName")
        val channel = stringField(raw, "channel")
        val versionCode = longField(raw, "versionCode")
        val versionName = stringField(raw, "versionName")
        val minSdk = intField(raw, "minSdk")
        val signerSha256 = stringField(raw, "signerSha256")
        val sha256 = stringField(raw, "sha256")
        val byteSize = longField(raw, "byteSize")
        val abis = abisField(raw)
        return SignedManifest(packageName, channel, versionCode, versionName, minSdk, abis, signerSha256, sha256, byteSize)
    }

    private fun stringField(raw: String, name: String): String {
        val match = Regex("\"$name\":\"((?:\\\\.|[^\"\\\\])*)\"").find(raw) ?: error("Release manifest is missing")
        return match.groupValues[1]
    }

    private fun longField(raw: String, name: String): Long {
        val match = Regex("\"$name\":(-?\\d+)").find(raw) ?: error("Release manifest is missing")
        return match.groupValues[1].toLong()
    }

    private fun intField(raw: String, name: String): Int = longField(raw, name).toInt()

    private fun abisField(raw: String): List<String> {
        val match = Regex("\"supportedAbis\":\\[(.*?)]").find(raw) ?: return emptyList()
        return Regex("\"((?:\\\\.|[^\"\\\\])*)\"").findAll(match.groupValues[1]).map { it.groupValues[1] }.toList()
    }
}
