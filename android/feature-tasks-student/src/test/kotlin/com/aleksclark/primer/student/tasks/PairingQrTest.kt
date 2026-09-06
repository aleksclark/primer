package com.aleksclark.primer.student.tasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertNotNull
import org.junit.Test

class PairingQrTest {
    @Test
    fun parsesPairingFieldsIncludingApiMountAndVersion() {
        val qr = PairingQrParser.parse(
            """{"v":1,"api":"/tasks/api","origin":"https://tasks.example.test","code":"ABC123","pairingId":"pair-1","exp":"tomorrow"}""",
        )

        assertNotNull(qr)
        assertEquals("https://tasks.example.test", qr?.origin)
        assertEquals("ABC123", qr?.code)
        assertEquals("pair-1", qr?.pairingId)
        assertEquals("/tasks/api", qr?.api)
        assertEquals(1, qr?.v)
    }

    @Test
    fun rejectsIncompleteQrPayload() {
        assertNull(PairingQrParser.parse("{" + "\"origin\":\"https://tasks.example.test\"}"))
        assertNull(PairingQrParser.parse("not json"))
    }

    @Test
    fun mountedProductionQrPinsExactTasksApi() {
        assertEquals(
            "https://tasks.example.test/tasks/api",
            ServerOriginPolicy.allowedApiBase(
                origin = "https://tasks.example.test",
                api = "/tasks/api",
                configuredHttpsOrigin = "https://tasks.example.test/tasks/api",
                allowEmulatorOrigin = false,
            ),
        )
    }

    @Test
    fun bareConfiguredOriginKeepsLegacyApiMount() {
        assertEquals(
            "https://tasks.example.test/api",
            ServerOriginPolicy.allowedApiBase(
                origin = "https://tasks.example.test/",
                api = "/api",
                configuredHttpsOrigin = "https://tasks.example.test",
                allowEmulatorOrigin = false,
            ),
        )
        assertEquals(
            "https://tasks.example.test/api",
            ServerOriginPolicy.allowedApiBase(
                origin = "https://tasks.example.test",
                api = null,
                configuredHttpsOrigin = "https://tasks.example.test",
                allowEmulatorOrigin = false,
            ),
        )
    }

    @Test
    fun rejectsWrongMountOriginVersionAndTraversal() {
        assertNull(
            ServerOriginPolicy.allowedApiBase(
                "https://tasks.example.test",
                "/api",
                "https://tasks.example.test/tasks/api",
                false,
            ),
        )
        assertNull(
            ServerOriginPolicy.allowedApiBase(
                "https://other.example.test",
                "/tasks/api",
                "https://tasks.example.test/tasks/api",
                false,
            ),
        )
        assertNull(
            ServerOriginPolicy.allowedApiBase(
                "https://tasks.example.test",
                "/tasks/api",
                "https://tasks.example.test/tasks/api",
                false,
                version = 2,
            ),
        )
        assertNull(
            ServerOriginPolicy.allowedApiBase(
                "https://tasks.example.test",
                "/tasks/api/../admin",
                "https://tasks.example.test/tasks/api",
                false,
            ),
        )
        assertNull(
            ServerOriginPolicy.allowedApiBase(
                "https://tasks.example.test",
                "/tasks/api?x=1",
                "https://tasks.example.test/tasks/api",
                false,
            ),
        )
        assertNull(
            ServerOriginPolicy.allowedApiBase(
                "https://user@tasks.example.test",
                "/tasks/api",
                "https://tasks.example.test/tasks/api",
                false,
            ),
        )
    }

    @Test
    fun debugEmulatorTranslationStillUsesAllowedMount() {
        assertEquals(
            "http://10.0.2.2:9090/api",
            ServerOriginPolicy.allowedApiBase(
                origin = "http://127.0.0.1:9090",
                api = "/api",
                configuredHttpsOrigin = "",
                allowEmulatorOrigin = true,
            ),
        )
        assertNull(
            ServerOriginPolicy.allowedApiBase(
                origin = "http://10.0.2.3:9090",
                api = "/api",
                configuredHttpsOrigin = "",
                allowEmulatorOrigin = true,
            ),
        )
    }
}
