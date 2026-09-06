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

    fun digestingSink(output: OutputStream, size: Long, sha256: String): OutputStream {
        validateExpected(size, sha256)
        return DigestingOutputStream(output, size, sha256)
    }

    private class DigestingOutputStream(
        private val dest: OutputStream,
        private val expectedSize: Long,
        private val expectedSha256: String,
    ) : OutputStream() {
        private val digest = MessageDigest.getInstance("SHA-256")
        private var written = 0L
        private var closed = false
        override fun write(b: Int) {
            write(byteArrayOf(b.toByte()), 0, 1)
        }
        override fun write(b: ByteArray, off: Int, len: Int) {
            if (len <= 0) return
            written += len
            if (written > expectedSize) throw ArchiveRejected("APK exceeds published size")
            digest.update(b, off, len)
            dest.write(b, off, len)
        }
        override fun flush() = dest.flush()
        override fun close() {
            if (closed) return
            closed = true
            dest.close()
            if (written != expectedSize) throw ArchiveRejected("APK is incomplete")
            if (!digest.digest().hex().equals(expectedSha256, true)) throw ArchiveRejected("APK checksum mismatch")
        }
    }
    fun validateArchive(
        installed: ArchiveIdentity?,
        archive: ArchiveIdentity,
        sdk: Int,
        abis: Set<String>,
        expectedPackage: String? = installed?.packageName,
        expectedSigners: Set<String>? = installed?.signers,
        allowFirstInstall: Boolean = false,
    ) {
        if (expectedPackage != null) validate(archive.packageName == expectedPackage) { "APK belongs to another application" }
        if (installed != null) {
            validate(archive.packageName == installed.packageName) { "APK belongs to another application" }
            validate(archive.version > installed.version) { "APK must have a newer version code" }
            validate(installed.signers.isNotEmpty() && archive.signers == installed.signers) { "APK signing identity differs" }
        } else {
            validate(allowFirstInstall) { "APK is not already installed" }
            validate(expectedSigners != null && expectedSigners.isNotEmpty() && archive.signers == expectedSigners) {
                "APK signing identity differs"
            }
        }
        if (expectedSigners != null && expectedSigners.isNotEmpty()) {
            validate(archive.signers == expectedSigners) { "APK signing identity differs" }
        }
        validate(archive.minSdk <= sdk) { "APK requires a newer Android version" }
        validate(archive.abis.isEmpty() || archive.abis.intersect(abis).isNotEmpty()) { "APK has no compatible native ABI" }
    }
    internal fun ByteArray.hex() = joinToString("") { "%02x".format(it) }
}
