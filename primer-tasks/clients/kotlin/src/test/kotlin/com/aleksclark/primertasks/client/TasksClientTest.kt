package com.aleksclark.primertasks.client

import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

class TasksClientTest {
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
    fun originPlusApiIsPreservedForBareHost() = runBlocking {
        server.enqueue(MockResponse().setBody(studentJson()))
        TasksClient(server.url("/").toString()).studentProfile("device-token")
        assertEquals("/api/device/profile", take().path)
    }

    @Test
    fun mountedTasksApiOriginIsNotDoubled() = runBlocking {
        server.enqueue(MockResponse().setBody(studentJson()))
        val origin = server.url("/tasks/api/").toString().trimEnd('/')
        TasksClient(origin).studentProfile("device-token")
        assertEquals("/tasks/api/device/profile", take().path)
    }

    @Test
    fun tasksMountWithoutApiGetsApiSuffix() = runBlocking {
        server.enqueue(MockResponse().setBody(studentJson()))
        val origin = server.url("/tasks/").toString().trimEnd('/')
        TasksClient(origin).studentProfile("device-token")
        assertEquals("/tasks/api/device/profile", take().path)
    }

    @Test
    fun studentMethodsKeepDeviceRoutesAndBearer() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"id":"occ-1","status":"awaiting_verification"}"""))
        val result = TasksClient(server.url("/").toString()).submitStudentOccurrence("device-token", "occ-1")
        val request = take()
        assertEquals("/api/device/occurrences/occ-1/submit", request.path)
        assertEquals("Bearer device-token", request.getHeader("Authorization"))
        assertEquals("POST", request.method)
        assertEquals("occ-1", result.id)
        assertEquals("awaiting_verification", result.status)
    }

    @Test
    fun parentAndDeviceCredentialsStayDistinct() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"items":[],"totalCount":0,"limit":20,"offset":0}"""))
        server.enqueue(MockResponse().setBody(studentJson()))
        val client = TasksClient(
            baseUrl = server.url("/tasks/api").toString(),
            parentCredentials = CredentialProvider { "parent-jwt" },
            deviceCredentials = CredentialProvider { "device-token" },
            managementCredentials = CredentialProvider { "mgmt-token" },
        )
        client.listStudents()
        val parent = take()
        assertEquals("/tasks/api/students", parent.path)
        assertEquals("Bearer parent-jwt", parent.getHeader("Authorization"))

        client.studentProfile("override-device")
        val device = take()
        assertEquals("/tasks/api/device/profile", device.path)
        assertEquals("Bearer override-device", device.getHeader("Authorization"))
    }

    @Test
    fun parentCallDoesNotSendDeviceBearer() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"items":[],"totalCount":0,"limit":20,"offset":0}"""))
        TasksClient(
            server.url("/").toString(),
            deviceCredentials = CredentialProvider { "device-token" },
            parentCredentials = CredentialProvider { "parent-jwt" },
        ).listStudents()
        val request = take()
        assertEquals("Bearer parent-jwt", request.getHeader("Authorization"))
        assertEquals("/api/students", request.path)
    }

    @Test
    fun managementDeviceRoutesUseManagementCredential() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"device":{"id":"00000000-0000-0000-0000-0000000000d1","displayName":"A16","deviceModel":"SM-S166V","state":"active","desiredRevision":1,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"recovery":[],"releaseTargets":[],"serverTime":"2026-01-01T00:00:00Z"}""",
            ),
        )
        TasksClient(
            server.url("/").toString(),
            parentCredentials = CredentialProvider { "parent-jwt" },
            managementCredentials = CredentialProvider { "mgmt-token" },
        ).managementDeviceDesired()
        val request = take()
        assertEquals("/api/management-device/desired", request.path)
        assertEquals("Bearer mgmt-token", request.getHeader("Authorization"))
    }

    @Test
    fun restConstraintViolationIsRejectedOnDecode() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"device":{"id":"not-a-uuid","displayName":"A16","deviceModel":"SM","state":"active","desiredRevision":1,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"recovery":[],"releaseTargets":[],"serverTime":"not-a-date"}""",
            ),
        )
        try {
            TasksClient(
                server.url("/").toString(),
                managementCredentials = CredentialProvider { "mgmt-token" },
            ).managementDeviceDesired()
            fail("expected constraint rejection")
        } catch (error: IllegalStateException) {
            assertTrue(error.message!!.contains("contract") || error.message!!.contains("payload"))
        }
    }

    @Test
    fun absentOptionalFieldsDecodeAsNull() = runBlocking {
        server.enqueue(MockResponse().setBody(studentJson()))
        val profile = TasksClient(server.url("/").toString()).studentProfile("device-token")
        assertNull(profile.archivedAt)
        assertEquals("Alice", profile.displayName)
    }

    @Test
    fun nestedScheduleNullEndAtAndIntegerWidth() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"id":"s1","studentId":"st1","templateId":"t1","revisionId":"r1","kind":"one_off","timezone":"America/Chicago","startAt":"2026-03-08T07:00:00Z","dueOffsetMinutes":30,"enabled":true,"version":1}""",
            ),
        )
        val created = TasksClient(
            server.url("/").toString(),
            parentCredentials = CredentialProvider { "parent-jwt" },
        ).createSchedule(
            ScheduleInput(
                studentId = "st1",
                templateId = "t1",
                revisionId = "r1",
                kind = "one_off",
                timezone = "America/Chicago",
                startAt = "2026-03-08T07:00:00Z",
                dueOffsetMinutes = 30L,
            ),
        )
        val request = take()
        assertEquals("/api/schedules", request.path)
        val body = request.body.readUtf8()
        assertTrue(body.contains("\"dueOffsetMinutes\":30"))
        assertTrue(!body.contains("endAt") || body.contains("\"endAt\":null"))
        assertNull(created.endAt)
        assertEquals(30L, created.dueOffsetMinutes)
        assertEquals(1L, created.version)
    }

    @Test
    fun listQueryOmitsNullsAndSendsPresentFilters() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"items":[],"totalCount":0,"limit":10,"offset":20}"""))
        TasksClient(
            server.url("/").toString(),
            parentCredentials = CredentialProvider { "parent-jwt" },
        ).listStudents(StudentsListQuery(q = "ali", limit = 10L, offset = 20L))
        val request = take()
        assertEquals("/api/students?q=ali&limit=10&offset=20", request.path)
    }

    @Test
    fun typedErrorSurfacesStatusCodeAndProblem() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setBody("""{"code":"conflict","message":"occurrence changed concurrently","detail":"occurrence changed concurrently"}"""),
        )
        try {
            TasksClient(
                server.url("/").toString(),
                parentCredentials = CredentialProvider { "parent-jwt" },
            ).decideOccurrence("occ-1", DecisionInput(accepted = true, reason = "done"))
            fail("expected typed conflict")
        } catch (error: TasksHttpException) {
            assertEquals(409, error.statusCode)
            assertEquals("conflict", error.code)
            assertEquals("occurrence changed concurrently", error.message)
        }
    }

    @Test
    fun missingParentCredentialFailsClosed() = runBlocking {
        try {
            TasksClient(server.url("/").toString()).createStudent(CreateStudent(displayName = "Alice"))
            fail("expected missing credential")
        } catch (error: TasksHttpException) {
            assertEquals(401, error.statusCode)
        }
        assertEquals(0, server.requestCount)
    }

    @Test
    fun requirementConfigRoundTripAsJsonObject() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"id":"r1","templateId":"t1","version":1,"title":"Brush","instructions":"twice","status":"draft","requirements":[{"id":"req1","kind":"parent_approval","configVersion":1,"config":{},"interaction":"parent_action","executor":"human"}],"createdAt":"2026-01-01T00:00:00Z"}""",
            ),
        )
        val created = TasksClient(
            server.url("/").toString(),
            parentCredentials = CredentialProvider { "parent-jwt" },
        ).createTask(
            TaskInput(
                title = "Brush",
                instructions = "twice",
                requirements = emptyList(),
            ),
        )
        assertEquals("Brush", created.title)
        assertEquals(1L, created.version)
        assertEquals(0, created.requirements.orEmpty().first().config.size)
    }

    @Test
    fun binaryArtifactStreamsExactBytesAndManagementAuth() = runBlocking {
        val apk = byteArrayOf(0x50, 0x4B, 0x03, 0x04, 0x00, 0x01, 0x02, 0xFF.toByte())
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/vnd.android.package-archive")
                .setHeader("Content-Length", apk.size.toString())
                .setBody(okio.Buffer().write(apk)),
        )
        val sink = java.io.ByteArrayOutputStream()
        val written = TasksClient(
            server.url("/").toString(),
            managementCredentials = CredentialProvider { "mgmt-token" },
            parentCredentials = CredentialProvider { "parent-jwt" },
        ).managementDeviceArtifact("rel-1", sink)
        val request = take()
        assertEquals("/api/management-device/artifacts/rel-1", request.path)
        assertEquals("Bearer mgmt-token", request.getHeader("Authorization"))
        assertEquals("GET", request.method)
        assertEquals(apk.size.toLong(), written)
        assertTrue(sink.toByteArray().contentEquals(apk))
    }

    @Test
    fun binaryArtifactRejectsOversizeDeclaredLengthWithoutConsumingBody() = runBlocking {
        val apk = ByteArray(32) { 0x7A }
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/vnd.android.package-archive")
                .setHeader("Content-Length", "64")
                .setBody(okio.Buffer().write(apk)),
        )
        try {
            TasksClient(
                server.url("/").toString(),
                managementCredentials = CredentialProvider { "mgmt-token" },
                maxBinaryBytes = 16,
            ).managementDeviceArtifact("rel-1", java.io.ByteArrayOutputStream())
            fail("expected size cap")
        } catch (error: TasksHttpException) {
            assertEquals(413, error.statusCode)
            assertEquals("too_large", error.code)
        }
        take()
        Unit
    }

    @Test
    fun parentDownloadUsesManagedReleaseApkNotManagementArtifact() = runBlocking {
        server.enqueue(MockResponse().setBody("apk-bytes"))
        val out = java.io.ByteArrayOutputStream()
        val written = TasksClient(
            server.url("/").toString(),
            parentCredentials = CredentialProvider { "parent-jwt" },
            managementCredentials = CredentialProvider { "mgmt-token" },
        ).downloadManagedReleaseArtifact("rel-2", out)
        val request = take()
        assertEquals("/api/managed-releases/rel-2/apk", request.path)
        assertEquals("Bearer parent-jwt", request.getHeader("Authorization"))
        assertEquals(9L, written)
        assertEquals("apk-bytes", out.toString())
    }

    @Test
    fun recoveryHistoryUsesParentGetOnManagedDeviceRecovery() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"items":[]}"""))
        TasksClient(
            server.url("/").toString(),
            parentCredentials = CredentialProvider { "parent-jwt" },
        ).listManagedRecoveryHistory("device-1")
        val request = take()
        assertEquals("/api/managed-devices/device-1/recovery", request.path)
        assertEquals("GET", request.method)
        assertEquals("Bearer parent-jwt", request.getHeader("Authorization"))
    }

    @Test
    fun studentOccurrenceDecodesCapabilityWithoutRequirementConfig() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"id":"occ-1","studentId":"st-1","scheduleId":"sch-1","revisionId":"r1","title":"Brush","instructions":"twice","status":"pending","nominalAt":"2026-01-01T00:00:00Z","dueAt":"2026-01-01T00:00:00Z","timezone":"UTC","dueOffsetMinutes":0,"dueSemantics":"offset_from_nominal","taskRevisionVersion":1,"scheduleVersion":1,"attemptNumber":0,"requirements":[{"id":"req-1","kind":"parent_approval","configVersion":1,"interaction":"parent_action","executor":"human"}],"studentCapability":"parent_approval","verification":[{"id":"req-1","kind":"parent_approval","interaction":"parent_action","attemptId":"","attemptStatus":"","dialogueStarted":false,"historyAttemptId":""}]}""",
            ),
        )
        val occurrence = TasksClient(
            server.url("/").toString(),
            deviceCredentials = CredentialProvider { "device-token" },
        ).studentOccurrence("device-token", "occ-1")
        assertEquals("parent_approval", occurrence.studentCapability)
        assertEquals("req-1", occurrence.requirements.orEmpty().single().id)
        assertEquals("parent_approval", occurrence.requirements.orEmpty().single().kind)
        assertEquals(1L, occurrence.requirements.orEmpty().single().configVersion)
        assertEquals(occurrence.requirements.orEmpty().single().id, occurrence.verification.orEmpty().single().id)
        assertEquals(false, occurrence.verification.orEmpty().single().dialogueStarted)
        assertEquals("", occurrence.verification.orEmpty().single().attemptId)
        assertEquals("/api/device/occurrences/occ-1", take().path)
    }

    @Test
    fun occurrenceRetryDecodesCanonicalRequirementAndPreviousAttempt() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"occurrenceId":"occ-1","requirementId":"req-1","previousAttemptId":"att-1","attemptNumber":2,"status":"open"}""",
            ),
        )
        val retry = TasksClient(
            server.url("/").toString(),
            parentCredentials = CredentialProvider { "parent-jwt" },
        ).retryOccurrence("occ-1")
        assertEquals("req-1", retry.requirementId)
        assertEquals("att-1", retry.previousAttemptId)
        assertEquals(2L, retry.attemptNumber)
        assertEquals("/api/occurrences/occ-1/retry", take().path)
    }

    @Test
    fun binaryArtifactRequiresManagementCredential() = runBlocking {
        try {
            TasksClient(server.url("/").toString(), parentCredentials = CredentialProvider { "parent-jwt" })
                .managementDeviceArtifact("rel-1", java.io.ByteArrayOutputStream())
            fail("expected missing credential")
        } catch (error: TasksHttpException) {
            assertEquals(401, error.statusCode)
        }
        assertEquals(0, server.requestCount)
    }

    private fun take() = requireNotNull(server.takeRequest(1, TimeUnit.SECONDS))

    private fun studentJson() =
        """{"id":"student-1","displayName":"Alice","createdAt":"2026-01-01T00:00:00Z"}"""
}
