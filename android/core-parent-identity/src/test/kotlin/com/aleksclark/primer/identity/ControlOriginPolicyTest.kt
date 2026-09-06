package com.aleksclark.primer.identity

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ControlOriginPolicyTest {
    @Test
    fun acceptsConfiguredHttps() {
        assertEquals(
            "https://api.primerlms.com/tasks/api",
            ControlOriginPolicy.apiBase("https://api.primerlms.com/tasks/api", false),
        )
        assertEquals(
            "https://api.primerlms.com/api",
            ControlOriginPolicy.apiBase("https://api.primerlms.com", false),
        )
    }

    @Test
    fun rejectsCleartextExceptEmulator() {
        assertNull(ControlOriginPolicy.apiBase("http://api.primerlms.com/tasks/api", true))
        assertEquals(
            "http://10.0.2.2:8080/tasks/api",
            ControlOriginPolicy.apiBase("http://10.0.2.2:8080/tasks/api", true),
        )
        assertNull(ControlOriginPolicy.apiBase("http://10.0.2.2:8080/tasks/api", false))
    }

    @Test
    fun rejectsUserinfoQueryFragment() {
        assertNull(ControlOriginPolicy.apiBase("https://user@api.primerlms.com/tasks/api", false))
        assertNull(ControlOriginPolicy.apiBase("https://api.primerlms.com/tasks/api?x=1", false))
        assertNull(ControlOriginPolicy.apiBase("https://api.primerlms.com/tasks/api#x", false))
    }
}
