package com.aleksclark.primer.updates

import org.junit.Assert.assertEquals
import org.junit.Test

class SignedManifestCodecTest {
    @Test
    fun generatedWireRoundTripsToDomainManifest() {
        val payload = """{"packageName":"com.aleksclark.primer.control","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":26,"supportedAbis":["arm64-v8a"],"signerSha256":"${"a".repeat(64)}","sha256":"${"b".repeat(64)}","byteSize":12}"""
        val parsed = SignedManifestCodec.parseVerified(payload.toByteArray())
        assertEquals("com.aleksclark.primer.control", parsed.packageName)
        assertEquals(2L, parsed.versionCode)
        assertEquals(26, parsed.minSdk)
        assertEquals(12L, parsed.byteSize)
        assertEquals(listOf("arm64-v8a"), parsed.supportedAbis)
    }
}
