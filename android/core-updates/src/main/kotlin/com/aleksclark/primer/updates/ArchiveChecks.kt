package com.aleksclark.primer.updates

import java.io.InputStream
import java.io.OutputStream
import java.security.MessageDigest

data class ArchiveIdentity(
    val packageName: String,
    val version: Long,
    val signers: Set<String>,
    val minSdk: Int,
    val abis: Set<String>,
)

class ArchiveRejected(message: String) : IllegalArgumentException(message)

object ArchiveChecks {
    private inline fun validate(value: Boolean, reason: () -> String) {
        if (!value) throw ArchiveRejected(reason())
    }
    const val MAX_APK_BYTES = 256L * 1024 * 1024
    fun validateExpected(size: Long, sha256: String) {
        validate(size in 1..MAX_APK_BYTES) { "Expected size must be between 1 and $MAX_APK_BYTES bytes" }
        validate(sha256.matches(Regex("[0-9a-fA-F]{64}"))) { "A SHA-256 checksum is required" }
    }
    fun copyVerified(input: InputStream, output: OutputStream, size: Long, sha256: String) {
        validateExpected(size, sha256)
        val digest = MessageDigest.getInstance("SHA-256")
        var written = 0L
        val buffer = ByteArray(32 * 1024)
        while (true) {
            val count = input.read(buffer)
            if (count < 0) break
            written += count
            validate(written <= size) { "APK exceeds published size" }
            digest.update(buffer, 0, count)
            output.write(buffer, 0, count)
        }
        validate(written == size) { "APK is incomplete" }
        validate(digest.digest().hex().equals(sha256, true)) { "APK checksum mismatch" }
    }
    fun validateArchive(installed: ArchiveIdentity, archive: ArchiveIdentity, sdk: Int, abis: Set<String>) {
        validate(archive.packageName == installed.packageName) { "APK belongs to another application" }
        validate(archive.version > installed.version) { "APK must have a newer version code" }
        // v1 deliberately requires the exact current signer set, not intersecting history.
        validate(installed.signers.isNotEmpty() && archive.signers == installed.signers) { "APK signing identity differs" }
        validate(archive.minSdk <= sdk) { "APK requires a newer Android version" }
        validate(archive.abis.isEmpty() || archive.abis.intersect(abis).isNotEmpty()) { "APK has no compatible native ABI" }
    }
    internal fun ByteArray.hex() = joinToString("") { "%02x".format(it) }
}
