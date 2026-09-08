package com.aleksclark.primer.student.tasks

import com.aleksclark.primertasks.client.TasksClient
import com.aleksclark.primertasks.client.TasksHttpException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class InMemoryTokenStore : TokenStore {
    var token: String? = null
    var cleared = 0
    override suspend fun save(token: String) { this.token = token }
    override suspend fun read(): String? = token
    override suspend fun clear() { token = null; cleared++ }
}

class InMemoryBindingStore : BindingStore {
    var metadata: StudentMetadata? = null
    var cleared = 0
    override suspend fun save(metadata: StudentMetadata) { this.metadata = metadata }
    override suspend fun read(): StudentMetadata? = metadata
    override suspend fun clear() { metadata = null; cleared++ }
}

class TasksSessionIsolationTest {
    private lateinit var server: MockWebServer
    private lateinit var tokens: InMemoryTokenStore
    private lateinit var bindings: InMemoryBindingStore

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        tokens = InMemoryTokenStore()
        bindings = InMemoryBindingStore()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun session() = TasksSession(
        tokenStore = tokens,
        bindingStore = bindings,
        clientFactory = { TasksClient(server.url("/").toString()) },
        configuredHttpsOrigin = "https://tasks.example.test",
        allowEmulatorOrigin = false,
    )

    @Test
    fun pairPersistsCredentialBeforeFollowUpLoads() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"studentId":"student-1","token":"new-token"}"""))
        server.enqueue(MockResponse().setResponseCode(500).setBody("boom"))

        val result = session().pair(
            """{"v":1,"api":"/api","origin":"https://tasks.example.test","code":"ABC123ABC123ABC123ABC123ABC123AB","pairingId":"pair-1"}""",
        )

        assertTrue(result is TasksRestoreResult.Unavailable)
        val unavailable = result as TasksRestoreResult.Unavailable
        assertEquals("new-token", unavailable.token)
        assertEquals("student-1", unavailable.metadata.studentId)
        assertEquals("Unable to load the checklist. Try again.", unavailable.message)
        assertEquals("new-token", tokens.token)
        assertEquals("student-1", bindings.metadata?.studentId)
        assertEquals("/api/device/pair", server.takeRequest(1, TimeUnit.SECONDS)?.path)
        assertEquals("/api/device/profile", server.takeRequest(1, TimeUnit.SECONDS)?.path)
        assertEquals(0, tokens.cleared)
    }

    @Test
    fun profileStudentMismatchDoesNotSwitchBindingOrClearOwnerMaterial() = runBlocking {
        tokens.token = "saved-token"
        bindings.metadata = StudentMetadata("student-1", "Maya", server.url("/").toString(), "pair-1")
        server.enqueue(MockResponse().setBody("""{"id":"student-2","displayName":"Other","createdAt":"2026-01-01T00:00:00Z"}"""))

        val result = session().restore() as TasksRestoreResult.Unpaired
        assertEquals("saved-token", tokens.token)
        assertEquals("student-1", bindings.metadata?.studentId)
        assertEquals(0, tokens.cleared)
        assertTrue(result.message!!.contains("different student"))
    }

    @Test
    fun occurrence401ClearsOnlyExpectedTasksSession() = runBlocking {
        tokens.token = "saved-token"
        bindings.metadata = StudentMetadata("student-1", "Maya", server.url("/").toString(), "pair-1")
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"detail":"revoked"}"""))

        val lookup = session().loadOccurrence("saved-token", server.url("/").toString(), "occ-1")
        assertTrue(lookup is OccurrenceLookup.Revoked)
        assertNull(tokens.token)
        assertNull(bindings.metadata)
        assertEquals(1, tokens.cleared)
        assertEquals(1, bindings.cleared)
    }

    @Test
    fun staleToken401DoesNotClearNewerSession() = runBlocking {
        tokens.token = "newer-token"
        bindings.metadata = StudentMetadata("student-1", "Maya", server.url("/").toString(), "pair-1")
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"detail":"revoked"}"""))

        val lookup = session().loadOccurrence("stale-token", server.url("/").toString(), "occ-1")
        assertTrue(lookup is OccurrenceLookup.Revoked)
        assertEquals("newer-token", tokens.token)
        assertEquals(0, tokens.cleared)
    }

    @Test
    fun networkFailureIsNotForeignUnavailable() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(503).setBody("down"))
        val lookup = session().loadOccurrence("saved-token", server.url("/").toString(), "occ-1")
        assertTrue(lookup is OccurrenceLookup.Failed)
        assertEquals("Unable to refresh this task.", (lookup as OccurrenceLookup.Failed).message)
        assertEquals(0, tokens.cleared)
    }

    @Test
    fun conflictRefreshesAuthoritativeOccurrenceWithoutSecondWrite() = runBlocking {
        tokens.token = "saved-token"
        server.enqueue(MockResponse().setResponseCode(409).setBody("""{"detail":"conflict"}"""))
        server.enqueue(
            MockResponse().setBody(
                """{"id":"occ-1","status":"in_progress","title":"Fractions","instructions":"Explain","studentId":"student-1","scheduleId":"s","revisionId":"r","timezone":"UTC","dueAt":"2026-01-01T00:00:00Z","nominalAt":"2026-01-01T00:00:00Z","dueSemantics":"hard","dueOffsetMinutes":0,"attemptNumber":1,"scheduleVersion":1,"taskRevisionVersion":1}""",
            ),
        )

        val result = session().start("saved-token", server.url("/").toString(), "occ-1")
        assertTrue(result is OccurrenceActionResult.Conflict)
        assertEquals("in_progress", (result as OccurrenceActionResult.Conflict).occurrence.status)
        assertEquals("POST", server.takeRequest(1, TimeUnit.SECONDS)?.method)
        assertEquals("GET", server.takeRequest(1, TimeUnit.SECONDS)?.method)
        assertEquals(0, tokens.cleared)
    }

    @Test
    fun unsupportedSubmitKeepsServerOccurrenceAndDoesNotInventCompletion() = runBlocking {
        tokens.token = "saved-token"
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setBody("""{"code":"unsupported_task","message":"this assigned work cannot be submitted as parent approval","detail":"this assigned work cannot be submitted as parent approval"}"""),
        )
        server.enqueue(
            MockResponse().setBody(
                """{"id":"occ-1","status":"in_progress","title":"Dialogue","instructions":"Talk","studentId":"student-1","scheduleId":"s","revisionId":"r","timezone":"UTC","dueAt":"2026-01-01T00:00:00Z","nominalAt":"2026-01-01T00:00:00Z","dueSemantics":"hard","dueOffsetMinutes":0,"attemptNumber":0,"scheduleVersion":1,"taskRevisionVersion":1,"studentCapability":"unsupported","requirements":[{"id":"req-1","kind":"agent_dialogue","configVersion":1,"interaction":"student_chat","executor":"agent"}]}""",
            ),
        )

        val result = session().submit("saved-token", server.url("/").toString(), "occ-1")
        assertTrue(result is OccurrenceActionResult.Conflict)
        val conflict = result as OccurrenceActionResult.Conflict
        assertEquals("in_progress", conflict.occurrence.status)
        assertEquals("unsupported", conflict.occurrence.studentCapability)
        assertEquals(StudentOccurrenceCopy.UNSUPPORTED, conflict.message)
        assertEquals(0, tokens.cleared)
    }

    @Test
    fun restoreNetworkFailureKeepsPairingAndDoesNotClearProtectedData() = runBlocking {
        tokens.token = "saved-token"
        bindings.metadata = StudentMetadata("student-1", "Maya", server.url("/").toString(), "pair-1")
        server.enqueue(MockResponse().setResponseCode(503).setBody("down"))

        val result = session().restore()
        assertTrue(result is TasksRestoreResult.Unavailable)
        val unavailable = result as TasksRestoreResult.Unavailable
        assertEquals("saved-token", unavailable.token)
        assertEquals("student-1", unavailable.metadata.studentId)
        assertEquals("Unable to load the checklist. Try again.", unavailable.message)
        assertEquals("saved-token", tokens.token)
        assertEquals(0, tokens.cleared)
        assertEquals(0, bindings.cleared)
    }

    @Test(expected = CancellationException::class)
    fun cancellationIsRethrown() {
        runBlocking {
            val cancelling = object : TokenStore {
                override suspend fun save(token: String) = Unit
                override suspend fun read(): String? = throw CancellationException("cancelled")
                override suspend fun clear() = Unit
            }
            TasksSession(
                tokenStore = cancelling,
                bindingStore = bindings,
                clientFactory = { TasksClient(server.url("/").toString()) },
                configuredHttpsOrigin = "https://tasks.example.test",
                allowEmulatorOrigin = false,
            ).restore()
        }
    }

    @Test
    fun revokedPairingDoesNotInventOwnerOrRecoveryClears() {
        val unpaired = TasksRestoreResult.Unpaired("This device pairing is no longer active. Scan a new QR code.")
        assertNull(unpaired.retainedToken)
        assertNull(unpaired.retainedMetadata)
    }

    @Test
    fun incomingWarmLinkOpensTasksAndLeaveClearsPendingLink() {
        val first = TasksDeepLinkRouting.incomingLink(
            TasksNavState(),
            scheme = "primerstudent",
            host = "occurrences",
            lastPathSegment = "abc",
            serialized = "primerstudent://occurrences/abc",
        )
        assertTrue(first.showTasks)
        assertEquals("primerstudent://occurrences/abc", first.pendingOccurrenceLink)
        val left = TasksDeepLinkRouting.leave(first)
        assertEquals(false, left.showTasks)
        assertNull(left.pendingOccurrenceLink)
        val openedManually = TasksNavState(showTasks = true)
        assertNull(openedManually.pendingOccurrenceLink)
        val afterLeave = TasksDeepLinkRouting.incomingLink(
            left,
            scheme = null,
            host = null,
            lastPathSegment = null,
            serialized = null,
        )
        assertEquals(false, afterLeave.showTasks)
        assertNull(afterLeave.pendingOccurrenceLink)
    }

    @Test
    fun leaveFromChecklistReturnsToHomeNavState() {
        val opened = TasksNavState(showTasks = true)
        val home = TasksDeepLinkRouting.leave(opened)
        assertEquals(false, home.showTasks)
        assertNull(home.pendingOccurrenceLink)
    }
}
