package com.aleksclark.primer.student.management

import com.aleksclark.primer.devicepolicy.ControlReadback
import com.aleksclark.primer.devicepolicy.InventoriedApp
import com.aleksclark.primer.devicepolicy.PolicyApplication
import com.aleksclark.primer.updates.GoSignedReleaseVectors
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.TasksClient
import com.google.crypto.tink.KeysetHandle
import java.io.File
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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
    override suspend fun publicEnrollment(): Pair<String, String> = "public-key" to "a".repeat(64)
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
            listOf(InventoriedApp("com.aleksclark.primer.student", "Primer Student", "0.1.0", 1, "aa"))
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
    fun replacementRequestedDuringDesiredFencesPolicy() = runBlocking {
        val origin = server.url("/").toString().trimEnd('/')
        val host = java.net.URI(origin).let { "${it.host}:${it.port}" }
        val deviceId = "11111111-1111-1111-1111-111111111111"
        secrets.binding = ManagementBinding("old-token", origin, deviceId, "a".repeat(64))
        val entered = java.util.concurrent.CountDownLatch(1)
        val release = java.util.concurrent.CountDownLatch(1)
        val blockDesired = java.util.concurrent.atomic.AtomicBoolean(false)
        val desired = """{"device":{"id":"$deviceId","displayName":"Student","deviceModel":"SM-S166V","state":"active","desiredRevision":1,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"policyRevision":{"id":"33333333-3333-3333-3333-333333333333","deviceId":"$deviceId","revision":1,"policy":{"approvedApps":[],"lockTask":{"enabled":true,"packages":["com.aleksclark.primer.student"]},"maintenance":{"allowParentUnlock":true}},"createdAt":"2026-01-01T00:00:00Z"},"recovery":[],"releaseTargets":[],"serverTime":"2026-01-01T00:00:00Z"}"""
        kotlinx.serialization.json.Json.decodeFromString(com.aleksclark.primertasks.client.DesiredState.serializer(), desired)
        server.dispatcher = object : okhttp3.mockwebserver.Dispatcher() {
            override fun dispatch(request: okhttp3.mockwebserver.RecordedRequest): MockResponse = when (request.path) {
                "/api/management-device/desired" -> {
                    if (blockDesired.get()) {
                        entered.countDown()
                        check(release.await(3, TimeUnit.SECONDS))
                    }
                    MockResponse().setBody(desired)
                }
                "/api/management-device/reports" -> MockResponse().setBody("""{"id":"r1","reportId":"22222222-2222-2222-2222-222222222222","deviceId":"$deviceId","policyRevision":1,"status":"applied","stale":false,"receivedAt":"2026-01-01T00:00:00Z"}""")
                "/api/management-device/enroll" -> MockResponse().setResponseCode(410).setBody("""{"code":"gone","message":"expired"}""")
                else -> MockResponse().setResponseCode(404)
            }
        }
        val applied = mutableListOf<Long>()
        val subject = session(applied = applied)
        // Establish that the same valid response really reaches the policy effect.
        subject.sync()
        assertEquals(listOf(1L), applied)
        applied.clear()
        blockDesired.set(true)
        val syncing = async { subject.sync() }
        try {
            kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
                assertTrue("desired request never reached HTTP boundary", entered.await(3, TimeUnit.SECONDS))
            }
            // Runs revocation immediately, then waits on the same session mutex.
            val replacing = async(start = kotlinx.coroutines.CoroutineStart.UNDISPATCHED) {
                subject.enroll("primer-management:v1:http://$host/management-device/enroll#00112233445566778899AABBCCDDEEFF", replace = true)
            }
            release.countDown()
            syncing.await()
            replacing.await()
            assertTrue("revoked generation applied the old policy: $applied", applied.isEmpty())
        } finally {
            release.countDown()
            syncing.cancelAndJoin()
        }
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
        val targetJson = """{"id":"${GoSignedReleaseVectors.SIGNING_KEY_ID.take(8)}1111-1111-1111-1111-111111111111","releaseId":"22222222-2222-2222-2222-222222222222","packageName":"com.aleksclark.primer.student","channel":"stable","status":"queued","versionCode":2,"versionName":"0.2.0","sha256":"${GoSignedReleaseVectors.SHA256}","byteSize":12,"targetVersion":1,"minSdk":28,"signerSha256":"${GoSignedReleaseVectors.SIGNER_SHA256}","manifestPayloadBase64":"${GoSignedReleaseVectors.STUDENT_PLAIN_PAYLOAD_BASE64URL}","manifestSignature":"${GoSignedReleaseVectors.STUDENT_PLAIN_SIGNATURE_BASE64URL}","signingKeyId":"${GoSignedReleaseVectors.SIGNING_KEY_ID}"}"""
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json").setBody(
                """{"device":{"id":"$deviceId","displayName":"Student","deviceModel":"SM-S166V","state":"active","desiredRevision":0,"appliedRevision":0,"createdAt":"2026-01-01T00:00:00Z"},"recovery":[],"releaseTargets":[$targetJson],"serverTime":"2026-01-01T00:00:00Z"}""",
            ),
        )
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json").setBody(
                """{"id":"r1","reportId":"33333333-3333-3333-3333-333333333333","deviceId":"$deviceId","policyRevision":0,"status":"requested","stale":false,"receivedAt":"2026-01-01T00:00:00Z"}""",
            ),
        )
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json").setBody(
                """{"id":"rc1","reportId":"44444444-4444-4444-4444-444444444444","targetId":"${GoSignedReleaseVectors.SIGNING_KEY_ID.take(8)}1111-1111-1111-1111-111111111111","status":"confirmed","receivedAt":"2026-01-01T00:00:00Z"}""",
            ),
        )
        val result = session(sink = sink, trustRoot = GoSignedReleaseVectors.TRUST_ROOT_BASE64URL).sync()
        assertFalse(result.message.contains("Unable to reach"))
        val receipt = outbox.pending(secrets.binding!!.origin, deviceId).none { it.kind == "release-receipt" && it.body.contains("\"status\":\"confirmed\"") && !it.body.contains("\"installedVersionCode\":2") }
        assertTrue(receipt)
    }
}
