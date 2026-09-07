package com.aleksclark.primer.control.tasks

import com.aleksclark.primertasks.client.CredentialProvider
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class ParentTasksRepositoryTest {
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
    fun usesParentCredentialOnHouseholdRoutes() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"items":[],"totalCount":0,"limit":20,"offset":0}"""))
        ParentTasksRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" }).listStudents("", 0)
        val request = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/students?limit=20&offset=0", request?.path)
        assertEquals("Bearer parent-jwt", request?.getHeader("Authorization"))
    }

    @Test
    fun downloadsControlApkWithParentJwt() = runBlocking {
        server.enqueue(MockResponse().setBody("apk"))
        val out = java.io.ByteArrayOutputStream()
        ParentTasksRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" })
            .downloadReleaseArtifact("rel-2", out)
        val request = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/managed-releases/rel-2/apk", request?.path)
        assertEquals("Bearer parent-jwt", request?.getHeader("Authorization"))
        assertEquals("apk", out.toString())
    }

    @Test
    fun pagesStudentsWithoutFetchingTheHousehold() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"items":[],"totalCount":40,"limit":20,"offset":20}"""))
        ParentTasksRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" }).listStudents("ali", 20)
        val request = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/students?q=ali&limit=20&offset=20", request?.path)
    }

    @Test
    fun publishedTaskPickerAsksTheServerForPublishedTemplates() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"items":[],"totalCount":0,"limit":20,"offset":0}"""))
        ParentTasksRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" })
            .listTasks("brush", 0, "published")
        val request = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/tasks?q=brush&limit=20&offset=0&sort=title&dir=asc&status=published&view=templates", request?.path)
        assertEquals("Bearer parent-jwt", request?.getHeader("Authorization"))
    }

    @Test
    fun decideAndRetryUseParentJwtOnOccurrenceRoutes() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"occurrenceId":"occ-1","decisionId":"d1","accepted":true,"status":"completed"}"""))
        server.enqueue(MockResponse().setBody("""{"occurrenceId":"occ-1","requirementId":"req-1","previousAttemptId":"att-1","attemptNumber":2,"status":"awaiting_verification"}"""))
        val repo = ParentTasksRepository(server.url("/").toString(), CredentialProvider { "parent-jwt" })
        repo.decide("occ-1", true, " Parent observed completion. ")
        val decide = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/occurrences/occ-1/decision", decide?.path)
        assertEquals("POST", decide?.method)
        assertEquals("Bearer parent-jwt", decide?.getHeader("Authorization"))
        val decideBody = decide?.body?.readUtf8().orEmpty()
        assertTrue(decideBody.contains("\"reason\":\"Parent observed completion.\""))
        assertTrue(decideBody.contains("\"accepted\":true"))
        repo.retry("occ-1")
        val retry = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/occurrences/occ-1/retry", retry?.path)
        assertEquals("POST", retry?.method)
    }
}
