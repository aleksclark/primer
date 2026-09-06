package com.aleksclark.primer.student.management

import com.aleksclark.primer.devicepolicy.ControlReadback
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
    var stable = "11111111-1111-1111-1111-111111111111"
    override suspend fun save(binding: ManagementBinding) { this.binding = binding }
    override suspend fun read(): ManagementBinding? = binding
    override suspend fun publicEnrollment(): Pair<String, String> = "public-key" to "a".repeat(64)
    override suspend fun privateHandle(): KeysetHandle? = null
    override suspend fun clearTokenOnly() { binding = binding?.copy(token = ""); tokenCleared++ }
    override suspend fun stableDeviceKey(): String = stable
    override suspend fun expectedToken(): String? = binding?.token
}

class ManagementSessionTest {
    private lateinit var server: MockWebServer
    private lateinit var secrets: InMemoryManagementSecrets
    private lateinit var outbox: InMemoryManagementOutbox

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        secrets = InMemoryManagementSecrets()
        outbox = InMemoryManagementOutbox()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun session(
        applied: MutableList<Long> = mutableListOf(),
        extrasSeen: MutableList<List<ControlReadback>> = mutableListOf(),
    ) = ManagementSession(
        credentials = secrets,
        clientFactory = { origin, token ->
            TasksClient(origin, managementCredentials = CredentialProvider { token() })
        },
        configuredHttpsOrigin = server.url("/").toString().trimEnd('/'),
        allowEmulatorOrigin = false,
        deviceName = "Student",
        deviceModel = "SM-S166V",
        applyPolicy = { revision, _, extras, _, _ ->
            applied += revision
            extrasSeen += extras
            PolicyApplication(revision, if (extras.any { it.status == "unsupported" }) "partial" else "applied", extras, "Applied revision $revision")
        },
        inventory = {
            listOf(InventoriedApp("com.aleksclark.primer.student", "Primer Student", "0.1.0", 1, "aa"))
        },
        applyRemoteRecovery = { _, _ -> false },
        applyRemoteLease = { _, _ -> false },
        studentVersion = "1",
        outbox = outbox,
        localRestrictions = emptySet(),
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
    fun existingEnrollmentIsNotSilentlyReplaced() = runBlocking {
        val origin = server.url("/").toString().trimEnd('/')
        val host = java.net.URI(origin).let { "${it.host}:${it.port}" }
        secrets.binding = ManagementBinding("mgmt-token", origin, "device-1", "a".repeat(64))
        val result = session().enroll("primer-management:v1:http://$host/management-device/enroll#00112233445566778899AABBCCDDEEFF")
        assertTrue(result.message, result.message.contains("already enrolled"))
        assertEquals("mgmt-token", secrets.binding?.token)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun revokedManagementTokenDoesNotClearTasksOrOwner() = runBlocking {
        secrets.binding = ManagementBinding("mgmt-token", server.url("/").toString().trimEnd('/'), "device-1", "a".repeat(64))
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"detail":"revoked"}"""))
        val result = session().sync()
        assertEquals(1, secrets.tokenCleared)
        assertTrue(result.message.contains("revoked"))
    }

    @Test
    fun resourceSpecific403DoesNotClearEnrollment() = runBlocking {
        secrets.binding = ManagementBinding("mgmt-token", server.url("/").toString().trimEnd('/'), "device-1", "a".repeat(64))
        server.enqueue(MockResponse().setResponseCode(403).setBody("""{"code":"artifact_forbidden","detail":"no","message":"no"}"""))
        val result = session().sync()
        assertEquals(0, secrets.tokenCleared)
        assertEquals("mgmt-token", secrets.binding?.token)
        assertTrue(result.message.contains("Unable to refresh"))
    }

    @Test
    fun invalidServerTimeFailsClosed() = runBlocking {
        secrets.binding = ManagementBinding("mgmt-token", server.url("/").toString().trimEnd('/'), "11111111-1111-1111-1111-111111111111", "a".repeat(64))
        server.enqueue(
            MockResponse().setBody(
                """{"device":{"id":"11111111-1111-1111-1111-111111111111","displayName":"Student","deviceModel":"SM-S166V","state":"active","desiredRevision":0,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"recovery":[],"releaseTargets":[],"serverTime":"not-a-time"}""",
            ),
        )
        val result = session().sync()
        assertTrue(result.message.contains("serverTime"))
        assertEquals("mgmt-token", secrets.binding?.token)
    }

    @Test
    fun unsupportedRemoteControlsAreReportedPartial() {
        val extras = RemotePolicyProjection.extraControls(
            lockTaskEnabled = true,
            lockTaskPackages = listOf("com.aleksclark.primer.student"),
            allowKeyguard = true,
            allowOverview = true,
            allowStatusBar = null,
            requiredPackages = emptyList(),
            userRestrictions = emptyList(),
            allowParentUnlock = true,
            studentPackage = "com.aleksclark.primer.student",
            localRestrictions = emptySet(),
        )
        assertTrue(extras.any { it.name == "lock-task-overview" && it.status == "unsupported" })
        assertEquals("partial", com.aleksclark.primer.devicepolicy.PolicyGuard.overallStatus(extras))
    }
}
