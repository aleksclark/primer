package com.aleksclark.primertasks

import com.aleksclark.primertasks.client.TasksClient
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

class TasksClientRouteTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun bearerProfileUsesDeviceOwnedRoute() = runBlocking {
        server.enqueue(MockResponse().setBody(studentJson()))

        TasksClient(server.url("/").toString()).studentProfile("device-token")

        val request = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/device/profile", request?.path)
        assertEquals("Bearer device-token", request?.getHeader("Authorization"))
    }

    @Test
    fun bearerSubmitUsesDeviceOwnedRoute() = runBlocking {
        server.enqueue(MockResponse().setBody("{\"id\":\"occ-1\",\"status\":\"awaiting_verification\"}"))

        TasksClient(server.url("/").toString()).submitStudentOccurrence("device-token", "occ-1")

        val request = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/device/occurrences/occ-1/submit", request?.path)
        assertEquals("Bearer device-token", request?.getHeader("Authorization"))
        assertEquals("POST", request?.method)
    }

    @Test
    fun bearerChecklistUsesDeviceOwnedRoute() = runBlocking {
        server.enqueue(MockResponse().setBody("{\"items\":[]}"))

        TasksClient(server.url("/").toString()).studentChecklist("device-token")

        val request = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/device/checklist", request?.path)
        assertEquals("Bearer device-token", request?.getHeader("Authorization"))
    }

    private fun studentJson() =
        """{"id":"student-1","displayName":"Alice","createdAt":"2026-01-01T00:00:00Z"}"""
}
