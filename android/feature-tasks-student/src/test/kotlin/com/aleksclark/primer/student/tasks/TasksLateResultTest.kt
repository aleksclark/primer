package com.aleksclark.primer.student.tasks

import com.aleksclark.primertasks.client.TasksClient
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.Assert.*
import org.junit.Test

class TasksLateResultTest {
    @Test
    fun delayedRestoreCannotResurrectRevokedOrReplacedPairing() = runBlocking {
        for (success in listOf(false, true)) for (replace in listOf(false, true)) {
            val server = MockWebServer()
            val entered = CountDownLatch(1)
            val release = CountDownLatch(1)
            server.dispatcher = object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                    "/api/device/profile" -> {
                        entered.countDown()
                        check(release.await(5, TimeUnit.SECONDS))
                        if (success) MockResponse().setBody("""{"id":"student-1","displayName":"Old name","createdAt":"2026-01-01T00:00:00Z"}""")
                        else MockResponse().setResponseCode(503).setBody("down")
                    }
                    "/api/device/checklist" -> MockResponse().setBody("""{"items":[]}""")
                    "/api/device/today", "/api/device/upcoming" -> MockResponse().setBody("""{"items":[],"totalCount":0,"limit":100,"offset":0}""")
                    else -> MockResponse().setResponseCode(401).setBody("""{"code":"revoked","detail":"revoked","message":"revoked"}""")
                }
            }
            server.start()
            try {
                val tokens = InMemoryTokenStore().apply { token = "old-token" }
                val bindings = InMemoryBindingStore().apply { metadata = StudentMetadata("student-1", "Old", server.url("/").toString(), "pair-1") }
                val session = TasksSession(tokens, bindings, { TasksClient(it) }, "https://tasks.example.test", false)
                val restore = async(Dispatchers.Default) { session.restore() }
                assertTrue(entered.await(5, TimeUnit.SECONDS))
                assertTrue(session.loadOccurrence("old-token", server.url("/").toString(), "occ") is OccurrenceLookup.Revoked)
                assertNull(tokens.token)
                assertNull(bindings.metadata)
                val newer = StudentMetadata("student-2", "New", server.url("/").toString(), "pair-2")
                if (replace) { tokens.save("new-token"); bindings.save(newer) }
                release.countDown()
                assertEquals(TasksRestoreResult.Superseded, restore.await())
                assertEquals(if (replace) "new-token" else null, tokens.token)
                assertEquals(if (replace) newer else null, bindings.metadata)
            } finally {
                release.countDown()
                server.shutdown()
            }
        }
    }

    @Test
    fun delayedRealLookupCannotReopenDetailAfterBackOrSelection() = runBlocking {
        val server = MockWebServer()
        val entered = CountDownLatch(1)
        val release = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                entered.countDown()
                check(release.await(5, TimeUnit.SECONDS))
                return MockResponse().setBody("""{"id":"occ-a","status":"pending","title":"Fractions","instructions":"Explain","studentId":"student-1","scheduleId":"s","revisionId":"r","timezone":"UTC","dueAt":"2026-01-01T00:00:00Z","nominalAt":"2026-01-01T00:00:00Z","dueSemantics":"hard","dueOffsetMinutes":0,"attemptNumber":0,"scheduleVersion":1,"taskRevisionVersion":1,"studentCapability":"parent_approval"}""")
            }
        }
        server.start()
        try {
            val session = TasksSession(InMemoryTokenStore(), InMemoryBindingStore(), { TasksClient(it) }, "https://tasks.example.test", false)
            val gate = OccurrenceViewGate()
            val request = gate.begin("token", server.url("/").toString(), "occ-a")
            val lookup = async(Dispatchers.Default) { session.loadOccurrence(request.token, request.origin, request.occurrenceId) }
            assertTrue(entered.await(5, TimeUnit.SECONDS))
            gate.invalidate() // Back to today
            assertFalse(gate.accepts(request))
            val selectedB = gate.begin("token", request.origin, "occ-b")
            release.countDown()
            assertTrue(lookup.await() is OccurrenceLookup.Found)
            assertFalse(gate.accepts(request))
            assertTrue(gate.accepts(selectedB))
            assertEquals("occ-b", selectedB.occurrenceId)
        } finally {
            release.countDown()
            server.shutdown()
        }
    }
}
