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

    @Test(expected = IllegalArgumentException::class)
    fun rejectsBlankOrigin() {
        TasksClient.apiOrigin("")
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsRelativeOrigin() {
        TasksClient.apiOrigin("/tasks/api")
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsUserinfo() {
        TasksClient.apiOrigin("https://user:pass@api.primerlms.com/tasks/api")
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsQuery() {
        TasksClient.apiOrigin("https://api.primerlms.com/tasks/api?x=1")
    }

    @Test(expected = IllegalArgumentException::class)
    fun rejectsFragment() {
        TasksClient.apiOrigin("https://api.primerlms.com/tasks/api#frag")
    }
}
