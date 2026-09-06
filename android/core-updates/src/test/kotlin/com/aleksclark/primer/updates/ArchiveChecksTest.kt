package com.aleksclark.primer.updates

import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.security.MessageDigest
import org.junit.Assert.*
import org.junit.Test

class ArchiveChecksTest {
    private val installed = ArchiveIdentity("primer.student", 1, setOf("key-a"), 28, emptySet())
    private val newer = installed.copy(version = 2)
    private fun validate(candidate: ArchiveIdentity) = ArchiveChecks.validateArchive(installed, candidate, 36, setOf("arm64-v8a"))
    private fun digest(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

    @Test fun `accepts newer same-package same-current-signer compatible archive`() { validate(newer) }
    @Test fun `first install of approved package requires exact signer`() {
        ArchiveChecks.validateArchive(
            installed = null,
            archive = newer.copy(packageName = "com.aleksclark.primer.tv"),
            sdk = 36,
            abis = setOf("arm64-v8a"),
            expectedPackage = "com.aleksclark.primer.tv",
            expectedSigners = setOf("key-a"),
            allowFirstInstall = true,
        )
        assertThrows(IllegalArgumentException::class.java) {
            ArchiveChecks.validateArchive(
                installed = null,
                archive = newer.copy(packageName = "com.aleksclark.primer.tv"),
                sdk = 36,
                abis = setOf("arm64-v8a"),
                expectedPackage = "com.aleksclark.primer.tv",
                expectedSigners = setOf("key-a"),
                allowFirstInstall = false,
            )
        }
    }
    @Test fun `wrong package rejected`() { assertThrows(IllegalArgumentException::class.java) { validate(newer.copy(packageName = "other")) } }
    @Test fun `same version and downgrade rejected`() {
        for (version in listOf(0L, 1L)) assertThrows(IllegalArgumentException::class.java) { validate(newer.copy(version = version)) }
    }
    @Test fun `historical signer intersection and extra signers are insufficient`() {
        for (signers in listOf(emptySet(), setOf("key-b"), setOf("key-a", "key-b"))) {
            assertThrows(IllegalArgumentException::class.java) { validate(newer.copy(signers = signers)) }
        }
    }
    @Test fun `SDK and ABI incompatibility rejected`() {
        assertThrows(IllegalArgumentException::class.java) { validate(newer.copy(minSdk = 37)) }
        assertThrows(IllegalArgumentException::class.java) { validate(newer.copy(abis = setOf("x86_64"))) }
        validate(newer.copy(abis = setOf("arm64-v8a", "x86_64")))
    }
    @Test fun `mandatory metadata bounded before reading`() {
        for (size in listOf(-1L, 0L, ArchiveChecks.MAX_APK_BYTES + 1)) {
            assertThrows(IllegalArgumentException::class.java) { ArchiveChecks.validateExpected(size, "0".repeat(64)) }
        }
        for (sha in listOf("", "abc", "g".repeat(64))) {
            assertThrows(IllegalArgumentException::class.java) { ArchiveChecks.validateExpected(1, sha) }
        }
    }
    @Test fun `copies exact digest and length`() {
        val bytes = "test bytes".toByteArray()
        val output = ByteArrayOutputStream()
        ArchiveChecks.copyVerified(ByteArrayInputStream(bytes), output, bytes.size.toLong(), digest(bytes))
        assertArrayEquals(bytes, output.toByteArray())
    }
    @Test fun `truncation excess bytes and wrong digest fail`() {
        val bytes = "test bytes".toByteArray()
        for (size in listOf(bytes.size - 1L, bytes.size + 1L)) assertThrows(IllegalArgumentException::class.java) {
            ArchiveChecks.copyVerified(ByteArrayInputStream(bytes), ByteArrayOutputStream(), size, digest(bytes))
        }
        assertThrows(IllegalArgumentException::class.java) {
            ArchiveChecks.copyVerified(ByteArrayInputStream(bytes), ByteArrayOutputStream(), bytes.size.toLong(), "0".repeat(64))
        }
    }
    @Test fun `same-size substituted bytes fail digest`() {
        val original = "test bytes".toByteArray()
        val substitute = "xxxx bytes".toByteArray()
        assertEquals(original.size, substitute.size)
        assertThrows(IllegalArgumentException::class.java) {
            ArchiveChecks.copyVerified(ByteArrayInputStream(substitute), ByteArrayOutputStream(), original.size.toLong(), digest(original))
        }
    }
    @Test fun `over-limit chunk is not written`() {
        val bytes = ByteArray(100)
        val output = ByteArrayOutputStream()
        assertThrows(IllegalArgumentException::class.java) { ArchiveChecks.copyVerified(ByteArrayInputStream(bytes), output, 1, digest(bytes)) }
        assertEquals(0, output.size())
    }
    @Test fun `digesting sink verifies length and digest on close`() {
        val bytes = "test bytes".toByteArray()
        val output = ByteArrayOutputStream()
        ArchiveChecks.digestingSink(output, bytes.size.toLong(), digest(bytes)).use { it.write(bytes) }
        assertArrayEquals(bytes, output.toByteArray())
        val truncated = ByteArrayOutputStream()
        assertThrows(IllegalArgumentException::class.java) {
            ArchiveChecks.digestingSink(truncated, bytes.size.toLong(), digest(bytes)).use { it.write(bytes, 0, bytes.size - 1) }
        }
        val overflow = ByteArrayOutputStream()
        assertThrows(IllegalArgumentException::class.java) {
            ArchiveChecks.digestingSink(overflow, 1, digest(bytes)).use { it.write(bytes) }
        }
        assertEquals(0, overflow.size())
    }
}
