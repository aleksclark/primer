package com.aleksclark.primertasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertNotNull
import org.junit.Test

class PairingQrTest {
    @Test
    fun parsesPairingFieldsAndIgnoresOtherPayloadFields() {
        val qr = PairingQrParser.parse(
            """{"v":1,"origin":"https://tasks.example.test","code":"ABC123","pairingId":"pair-1","exp":"tomorrow"}""",
        )

        assertNotNull(qr)
        assertEquals("https://tasks.example.test", qr?.origin)
        assertEquals("ABC123", qr?.code)
        assertEquals("pair-1", qr?.pairingId)
    }

    @Test
    fun rejectsIncompleteQrPayload() {
        assertNull(PairingQrParser.parse("{" + "\"origin\":\"https://tasks.example.test\"}"))
        assertNull(PairingQrParser.parse("not json"))
    }

    @Test
    fun acceptsOnlyConfiguredHttpsOrDebugEmulatorOrigin() {
        assertEquals(
            "https://tasks.example.test",
            ServerOriginPolicy.allowedOrigin("https://tasks.example.test/", "https://tasks.example.test", false),
        )
        assertNull(ServerOriginPolicy.allowedOrigin("http://tasks.example.test:9090", "http://tasks.example.test:9090", false))
        assertNull(ServerOriginPolicy.allowedOrigin("https://other.example.test", "https://tasks.example.test", false))
        assertEquals(
            "http://10.0.2.2:9090",
            ServerOriginPolicy.allowedOrigin("http://10.0.2.2:9090", "", true),
        )
        assertNull(ServerOriginPolicy.allowedOrigin("http://10.0.2.3:9090", "", true))
    }
}
