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
                """{"device":{"id":"d1","displayName":"A16","deviceModel":"SM-S166V","state":"active","desiredRevision":1,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"recovery":[],"releaseTargets":[]}""",
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

    private fun take() = requireNotNull(server.takeRequest(1, TimeUnit.SECONDS))

    private fun studentJson() =
        """{"id":"student-1","displayName":"Alice","createdAt":"2026-01-01T00:00:00Z"}"""
}
