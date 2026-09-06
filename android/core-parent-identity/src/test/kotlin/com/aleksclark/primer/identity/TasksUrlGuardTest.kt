package com.aleksclark.primer.identity

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TasksUrlGuardTest {
    @Test
    fun preservesMountedTasksApi() {
        assertEquals(
            "https://api.primerlms.com/tasks/api",
            TasksUrlGuard.endpoint(
                originRaw = "https://api.primerlms.com/tasks/api",
                apiRaw = "https://api.primerlms.com/tasks/api",
                configuredHttpsOrigin = "https://api.primerlms.com/tasks/api",
                allowEmulatorOrigin = false,
            )?.apiBase,
        )
    }

    @Test
    fun appendsLegacyApiForBareOrigin() {
        assertEquals(
            "https://api.primerlms.com/api",
            TasksUrlGuard.endpoint(
                originRaw = "https://api.primerlms.com",
                apiRaw = "https://api.primerlms.com",
                configuredHttpsOrigin = "https://api.primerlms.com",
                allowEmulatorOrigin = false,
            )?.apiBase,
        )
    }

    @Test
    fun rejectsCleartextExceptEmulator() {
        assertNull(
            TasksUrlGuard.endpoint(
                "http://api.primerlms.com/tasks/api",
                "http://api.primerlms.com/tasks/api",
                "http://api.primerlms.com/tasks/api",
                true,
            ),
        )
        assertEquals(
            "http://10.0.2.2:8080/tasks/api",
            TasksUrlGuard.endpoint(
                "http://10.0.2.2:8080/tasks/api",
                "http://10.0.2.2:8080/tasks/api",
                "http://10.0.2.2:8080/tasks/api",
                true,
            )?.apiBase,
        )
        assertNull(
            TasksUrlGuard.endpoint(
                "http://10.0.2.2:8080/tasks/api",
                "http://10.0.2.2:8080/tasks/api",
                "http://10.0.2.2:8080/tasks/api",
                false,
            ),
        )
    }
}
