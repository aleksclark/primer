package com.aleksclark.primertasks.client

import org.junit.Assert.assertEquals
import org.junit.Test

class ApiOriginTest {
    @Test
    fun preservesExactMountedTasksApi() {
        assertEquals(
            "https://api.primerlms.com/tasks/api",
            TasksClient.apiOrigin("https://api.primerlms.com/tasks/api"),
        )
        assertEquals(
            "https://api.primerlms.com/tasks/api",
            TasksClient.apiOrigin("https://api.primerlms.com/tasks/api/"),
        )
    }

    @Test
    fun appendsApiForProductOriginAndTasksMount() {
        assertEquals("https://api.primerlms.com/api", TasksClient.apiOrigin("https://api.primerlms.com"))
        assertEquals("https://api.primerlms.com/tasks/api", TasksClient.apiOrigin("https://api.primerlms.com/tasks"))
        assertEquals("http://10.0.2.2:8080/api", TasksClient.apiOrigin("http://10.0.2.2:8080"))
    }
}
