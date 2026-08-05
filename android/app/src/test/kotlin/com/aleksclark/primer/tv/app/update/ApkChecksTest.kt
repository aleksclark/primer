package com.aleksclark.primer.tv.app.update

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ApkChecksTest {
    @Test
    fun `digest accepts blank or exactly 64 hexadecimal characters`() {
        assertTrue(ApkChecks.validExpectedDigest(""))
        assertTrue(ApkChecks.validExpectedDigest("a".repeat(64)))
        assertTrue(ApkChecks.validExpectedDigest("ABCDEF".repeat(10) + "ABCD"))
        assertFalse(ApkChecks.validExpectedDigest("a".repeat(63)))
        assertFalse(ApkChecks.validExpectedDigest("g".repeat(64)))
    }

    @Test
    fun `digest comparison is case insensitive`() {
        assertTrue(ApkChecks.digestMatches("ABC123", "abc123"))
        assertFalse(ApkChecks.digestMatches("abc123", "abc124"))
    }

    @Test
    fun `positive published size must exactly match download`() {
        assertTrue(ApkChecks.sizeMatches(0, 123))
        assertTrue(ApkChecks.sizeMatches(123, 123))
        assertFalse(ApkChecks.sizeMatches(123, 122))
        assertFalse(ApkChecks.sizeMatches(123, 124))
    }
}
