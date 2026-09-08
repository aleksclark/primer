package com.aleksclark.primer.student.management

import com.aleksclark.primer.devicepolicy.ControlReadback
import com.aleksclark.primer.devicepolicy.InventoriedApp
import com.aleksclark.primer.devicepolicy.PolicyApplication
import com.aleksclark.primer.security.RecoveryHpke
import com.aleksclark.primer.updates.GoSignedReleaseVectors
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.DeviceCapabilities
import com.aleksclark.primertasks.client.EnrollInput
import com.aleksclark.primertasks.client.PolicyReportInput
import com.aleksclark.primertasks.client.ReleaseReceiptInput
import com.aleksclark.primertasks.client.TasksClient
import com.google.crypto.tink.KeysetHandle
import java.io.File
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.Rule
import org.junit.rules.TemporaryFolder

class InMemoryManagementSecrets : ManagementSecrets {
    var binding: ManagementBinding? = null
    var tokenCleared = 0
    var stable = "11111111-1111-1111-1111-111111111111"
    override suspend fun save(binding: ManagementBinding) { this.binding = binding }
    override suspend fun read(): ManagementBinding? = binding
    private val publicKeyJson by lazy { RecoveryHpke.publicKeysetJson(RecoveryHpke.generatePrivateHandle()) }
    override suspend fun publicEnrollment(): Pair<String, String> =
        RecoveryHpke.encodePublicEnrollmentKey(publicKeyJson) to RecoveryHpke.keyId(publicKeyJson)
    override suspend fun privateHandle(): KeysetHandle? = null
    override suspend fun clearTokenOnly() { binding = binding?.copy(token = ""); tokenCleared++ }
    override suspend fun stableDeviceKey(): String = stable
    override suspend fun expectedToken(): String? = binding?.token
    override fun snapshot(): ManagementBinding? = binding
}

class ManagementSessionTest {
    @get:Rule val temp = TemporaryFolder()
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
        sink: RemoteReleaseSink? = null,
        trustRoot: String = "",
        onPolicy: (() -> Unit)? = null,
        androidApi: Long = 28,
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
            onPolicy?.invoke()
            applied += revision
            extrasSeen += extras
            PolicyApplication(revision, if (extras.any { it.status == "unsupported" }) "partial" else "applied", extras, "Applied revision $revision")
        },
        inventory = {
            listOf(InventoriedApp("com.aleksclark.primer.student", "Primer Student", "0.1.0", 1, GoSignedReleaseVectors.SIGNER_SHA256))
        },
        applyRemoteRecovery = { _, _ -> "ack-recovery" },
        applyRemoteLease = { _, _ -> "ack-lease" },
        studentVersion = "1",
        outbox = outbox,
        localRestrictions = emptySet(),
        releaseSink = sink,
        trustRoot = trustRoot,
        elapsedMs = { 100 },
        boot = { 7 },
        // Plain JVM android.jar reports SDK_INT=0. Supply real supported device
        // facts without bypassing the generated request contract validator.
        deviceCapabilities = {
            DeviceCapabilities(androidApi = androidApi, supportedAbis = listOf("arm64-v8a"), deviceOwner = true, lockTaskSupported = true)
        },
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
                .setBody("""{"id":"55555555-5555-5555-5555-555555555555","reportId":"22222222-2222-2222-2222-222222222222","deviceId":"11111111-1111-1111-1111-111111111111","policyRevision":0,"status":"requested","stale":false,"receivedAt":"2026-01-01T00:00:00Z"}"""),
        )
        val qr = "primer-management:v1:http://$host/management-device/enroll#00112233445566778899AABBCCDDEEFF"
        val result = session().enroll(qr)
        assertEquals(result.message, "mgmt-token", secrets.binding?.token)
        assertTrue(result.message, result.message.contains("No remote policy") || result.message.contains("Applied") || result.message.contains("requested"))
        val enroll = server.takeRequest(1, TimeUnit.SECONDS)
        assertEquals("/api/management-device/enroll", enroll?.path)
        assertEquals(null, enroll?.getHeader("Authorization"))
        val enrollmentBody = Json.decodeFromString<EnrollInput>(requireNotNull(enroll).body.readUtf8())
        assertEquals(28L, enrollmentBody.capabilities?.androidApi)
        assertEquals(secrets.publicEnrollment().first, enrollmentBody.enrollmentPublicKey)
        assertEquals(secrets.publicEnrollment().second, enrollmentBody.enrollmentKeyId)
        val report = requireNotNull(server.takeRequest(1, TimeUnit.SECONDS))
        assertEquals("/api/management-device/reports", report.path)
        assertEquals("Bearer mgmt-token", report.getHeader("Authorization"))
        val reportBody = Json.decodeFromString<PolicyReportInput>(report.body.readUtf8())
        assertEquals(GoSignedReleaseVectors.SIGNER_SHA256, reportBody.installedApps?.single()?.signerSha256)
        assertFalse(result.retryable)
        assertTrue(outbox.pending(origin, requireNotNull(secrets.binding).deviceId).isEmpty())
    }

    @Test
    fun invalidEnrollmentCapabilitiesStillFailBeforeHttp() = runBlocking {
        val origin = server.url("/").toString().trimEnd('/')
        val host = java.net.URI(origin).let { "${it.host}:${it.port}" }
        val qr = "primer-management:v1:http://$host/management-device/enroll#00112233445566778899AABBCCDDEEFF"
        for (api in listOf(0L, 27L, 37L)) {
            server.enqueue(MockResponse().setResponseCode(400))
            val result = session(androidApi = api).enroll(qr)
            assertEquals("Unable to reach the management server.", result.message)
            assertNull(secrets.binding)
            assertEquals("API $api must be rejected by the generated contract before HTTP", 0, server.requestCount)
        }
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
        val applied = mutableListOf<Long>()
        val result = session(applied = applied).sync()
        // The generated response validator now rejects invalid date-time before
        // applyDesired's defensive parser runs. This is not an auth revocation.
        assertEquals("Unable to reach the management server. Last-known policy remains.", result.message)
        assertTrue(result.retryable)
        assertEquals("mgmt-token", secrets.binding?.token)
        assertEquals(0, secrets.tokenCleared)
        assertTrue(applied.isEmpty())
        assertTrue(outbox.pending(secrets.binding!!.origin, secrets.binding!!.deviceId).isEmpty())
        assertEquals(1, server.requestCount)
        assertEquals("/api/management-device/desired", server.takeRequest(1, TimeUnit.SECONDS)?.path)
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

    @Test
    fun authorizationLeaseTracksPersistedBindingAndGeneration() {
        val first = ManagementBinding("token-a", "https://example.invalid", "device-a", "a".repeat(64))
        val second = ManagementBinding("token-b", "https://example.invalid", "device-a", "a".repeat(64))
        secrets.binding = first
        val lease = ManagementAuthorization.capture(first) { secrets.snapshot() }
        assertTrue(lease.authorized())
        secrets.binding = second
        assertTrue(!lease.authorized())
        secrets.binding = first
        assertTrue(lease.authorized())
        ManagementAuthorization.revoke()
        assertTrue(!lease.authorized())
        secrets.binding = first
        assertTrue(!lease.authorized())
    }

    @Test
    fun cancelDuringDelayedDesiredDoesNotDispatchPolicyOrInstall() = runBlocking {
        val deviceId = "11111111-1111-1111-1111-111111111111"
        secrets.binding = ManagementBinding("mgmt-token", server.url("/").toString().trimEnd('/'), deviceId, "a".repeat(64))
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE))
        val applied = mutableListOf<Long>()
        var installs = 0
        val sink = object : RemoteReleaseSink {
            override val studentVersion: Long = 1
            override val installActive: Boolean = false
            override val pendingTargetId: String = ""
            override val pendingTargetVersion: Long = 0
            override fun stagingDir(): File = temp.newFolder()
            override fun installedVersion(packageName: String): Long? = 1
            override fun approvedPackage(packageName: String) = ApprovedPackage(packageName, setOf("aa"), 1)
            override fun installVerified(
                file: File,
                manifest: SignedManifest,
                authorized: () -> Boolean,
                approved: ApprovedPackage?,
                targetId: String?,
                targetVersion: Long?,
            ): InstallOutcome {
                installs += 1
                return InstallOutcome("failed", 1, "should not run")
            }
        }
        val job = async { session(applied = applied, sink = sink).sync() }
        delay(50)
        job.cancelAndJoin()
        assertEquals(0, applied.size)
        assertEquals(0, installs)
        assertTrue(outbox.pending(secrets.binding!!.origin, deviceId).isEmpty())
        val first = ManagementAuthorization.capture(secrets.binding!!) { secrets.snapshot() }
        ManagementAuthorization.revoke()
        assertTrue(!first.authorized())
    }

    @Test
    fun replaceRevokesPriorLeaseBeforeNewEnrollmentHttp() = runBlocking {
        val origin = server.url("/").toString().trimEnd('/')
        val host = java.net.URI(origin).let { "${it.host}:${it.port}" }
        val previous = ManagementBinding("old-token", origin, "11111111-1111-1111-1111-111111111111", "a".repeat(64))
        secrets.binding = previous
        val prior = ManagementAuthorization.capture(previous) { secrets.snapshot() }
        assertTrue(prior.authorized())
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE))
        val applied = mutableListOf<Long>()
        val job = async {
            session(applied = applied).enroll(
                "primer-management:v1:http://$host/management-device/enroll#00112233445566778899AABBCCDDEEFF",
                replace = true,
            )
        }
        delay(50)
        assertTrue(!prior.authorized())
        job.cancelAndJoin()
        assertEquals(0, applied.size)
        assertEquals("old-token", secrets.binding?.token)
        assertTrue(!prior.authorized())
    }

    @Test
    fun confirmedReceiptRequiresObservedInstalledVersion() = runBlocking {
        val deviceId = "11111111-1111-1111-1111-111111111111"
        secrets.binding = ManagementBinding("mgmt-token", server.url("/").toString().trimEnd('/'), deviceId, "a".repeat(64))
        val sink = object : RemoteReleaseSink {
            override val studentVersion: Long = 1
            override val installActive: Boolean = false
            override val pendingTargetId: String = ""
            override val pendingTargetVersion: Long = 0
            override fun stagingDir(): File = temp.newFolder()
            override fun installedVersion(packageName: String): Long? = 2
            override fun approvedPackage(packageName: String) = ApprovedPackage(packageName, setOf(GoSignedReleaseVectors.SIGNER_SHA256), 2)
            override fun installVerified(
                file: File,
                manifest: SignedManifest,
                authorized: () -> Boolean,
                approved: ApprovedPackage?,
                targetId: String?,
                targetVersion: Long?,
            ): InstallOutcome = error("must not install when already at target")
        }
        val targetId = "66666666-6666-6666-6666-666666666666"
        val targetJson = """{"id":"$targetId","releaseId":"22222222-2222-2222-2222-222222222222","packageName":"com.aleksclark.primer.student","channel":"stable","status":"queued","versionCode":2,"versionName":"0.2.0","sha256":"${GoSignedReleaseVectors.SHA256}","byteSize":12,"targetVersion":1,"minSdk":28,"signerSha256":"${GoSignedReleaseVectors.SIGNER_SHA256}","manifestPayloadBase64":"${GoSignedReleaseVectors.STUDENT_PLAIN_PAYLOAD_BASE64URL}","manifestSignature":"${GoSignedReleaseVectors.STUDENT_PLAIN_SIGNATURE_BASE64URL}","signingKeyId":"${GoSignedReleaseVectors.SIGNING_KEY_ID}"}"""
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json").setBody(
                """{"device":{"id":"$deviceId","displayName":"Student","deviceModel":"SM-S166V","state":"active","desiredRevision":0,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"recovery":[],"releaseTargets":[$targetJson],"serverTime":"2026-01-01T00:00:00Z"}""",
            ),
        )
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json").setBody(
                """{"id":"55555555-5555-5555-5555-555555555555","reportId":"33333333-3333-3333-3333-333333333333","deviceId":"$deviceId","policyRevision":0,"status":"requested","stale":false,"receivedAt":"2026-01-01T00:00:00Z"}""",
            ),
        )
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json").setBody(
                """{"id":"rc1","reportId":"44444444-4444-4444-4444-444444444444","targetId":"$targetId","status":"confirmed","receivedAt":"2026-01-01T00:00:00Z"}""",
            ),
        )
        val result = session(sink = sink, trustRoot = GoSignedReleaseVectors.TRUST_ROOT_BASE64URL).sync()
        assertEquals("No remote policy yet", result.message)
        assertFalse(result.retryable)
        assertEquals("/api/management-device/desired", server.takeRequest(1, TimeUnit.SECONDS)?.path)
        assertEquals("/api/management-device/reports", server.takeRequest(1, TimeUnit.SECONDS)?.path)
        // Inspect the receipt that actually crossed the public HTTP boundary.
        // An empty outbox alone would pass even if no receipt was ever created.
        val receipt = requireNotNull(server.takeRequest(1, TimeUnit.SECONDS))
        assertEquals("/api/management-device/release-receipts", receipt.path)
        assertEquals("Bearer mgmt-token", receipt.getHeader("Authorization"))
        val body = Json.decodeFromString<ReleaseReceiptInput>(receipt.body.readUtf8())
        assertEquals(targetId, body.targetId)
        assertEquals(1L, body.targetVersion)
        assertEquals("confirmed", body.status)
        assertEquals(2L, body.installedVersionCode)
        assertEquals(3, server.requestCount)
        assertTrue(outbox.pending(secrets.binding!!.origin, deviceId).isEmpty())
    }
}
