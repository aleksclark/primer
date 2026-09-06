package com.aleksclark.primer.student.management

import com.aleksclark.primer.devicepolicy.InventoriedApp
import com.aleksclark.primer.devicepolicy.PolicyApplication
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.TasksClient
import com.google.crypto.tink.KeysetHandle
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class InMemoryManagementSecrets : ManagementSecrets {
    var binding: ManagementBinding? = null
    var tokenCleared = 0
    override suspend fun save(binding: ManagementBinding) { this.binding = binding }
    override suspend fun read(): ManagementBinding? = binding
    override suspend fun publicEnrollment(): Pair<String, String> = "public-key" to "a".repeat(64)
    override suspend fun privateHandle(): KeysetHandle? = null
    override suspend fun clearTokenOnly() { binding = binding?.copy(token = ""); tokenCleared++ }
}

class ManagementSessionTest {
    private lateinit var server: MockWebServer
    private lateinit var secrets: InMemoryManagementSecrets

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        secrets = InMemoryManagementSecrets()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun session(applied: MutableList<Long> = mutableListOf()) = ManagementSession(
        credentials = secrets,
        clientFactory = { origin, token ->
            TasksClient(origin, managementCredentials = CredentialProvider { token() })
        },
        configuredHttpsOrigin = server.url("/").toString().trimEnd('/'),
        allowEmulatorOrigin = false,
        deviceName = "Student",
        deviceModel = "SM-S166V",
        stableDeviceKey = "stable-device-key-1",
        applyPolicy = { revision, _ ->
            applied += revision
            PolicyApplication(revision, "applied", emptyList(), "Applied revision $revision")
        },
        inventory = {
            listOf(InventoriedApp("com.aleksclark.primer.student", "Primer Student", "0.1.0", 1, "aa"))
        },
        applyRemoteRecovery = { _, _ -> false },
        studentVersion = "1",
        elapsedMs = { 100 },
        boot = { 7 },
    )

    @Test
    fun enrollCallsFacadeAndPersistsSeparateManagementToken() = runBlocking {
        val origin = server.url("/").toString().trimEnd('/')
        val host = java.net.URI(origin).let { "${it.host}:${it.port}" }
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody(
                    """{"token":"mgmt-token","responseLossPolicy":"fresh-parent-enrollment","device":{"id":"11111111-1111-1111-1111-111111111111","displayName":"Student","deviceModel":"SM-S166V","state":"active","desiredRevision":0,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"desired":{"device":{"id":"11111111-1111-1111-1111-111111111111","displayName":"Student","deviceModel":"SM-S166V","state":"active","desiredRevision":0,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"recovery":[],"releaseTargets":[],"serverTime":"2026-01-01T00:00:00Z"}}""",
                ),
        )
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody("""{"id":"r1","reportId":"22222222-2222-2222-2222-222222222222","deviceId":"11111111-1111-1111-1111-111111111111","policyRevision":0,"status":"requested","stale":false,"receivedAt":"2026-01-01T00:00:00Z"}"""),
        )
        val qr = "primer-management:v1:http://$host/management-device/enroll#00112233445566778899AABBCCDDEEFF"
        val result = session().enroll(qr)
        assertEquals(result.message, "mgmt-token", secrets.binding?.token)
        assertTrue(result.message, result.message.contains("No remote policy") || result.message.contains("Applied") || result.message.contains("requested"))
        val enroll = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/management-device/enroll", enroll?.path)
        assertEquals(null, enroll?.getHeader("Authorization"))
    }

    @Test
    fun revokedManagementTokenDoesNotClearTasksOrOwner() = runBlocking {
        secrets.binding = ManagementBinding("mgmt-token", server.url("/").toString().trimEnd('/'), "device-1", "a".repeat(64))
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"detail":"revoked"}"""))
        val result = session().sync()
        assertEquals(1, secrets.tokenCleared)
        assertTrue(result.message.contains("revoked"))
    }
}
