package com.aleksclark.primer.control

import com.aleksclark.primer.control.device.ControlSelfUpdatePhase
import com.aleksclark.primer.control.device.DeviceRepository
import com.aleksclark.primer.control.tasks.ParentTasksRepository
import com.aleksclark.primer.updates.InstallAttempt
import com.aleksclark.primer.updates.SelfUpdateCommands
import com.aleksclark.primer.updates.SelfUpdateEligibility
import com.aleksclark.primer.updates.SelfUpdateSessionState
import com.aleksclark.primer.updates.SignedManifest
import com.aleksclark.primer.identity.ParentIdentity
import com.aleksclark.primer.identity.SignInOutcome
import com.aleksclark.primer.identity.SignOutOutcome
import com.aleksclark.primertasks.client.Student
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.setMain
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ControlViewModelTest {
    private val dispatcher = UnconfinedTestDispatcher()
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
        Dispatchers.resetMain()
    }

    @Test
    fun delayedStudentOpenAfterLogoutDoesNotRepopulateNewAccount() = runBlocking {
        val hold = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path == "/api/auth/logout" -> MockResponse().setBody("""{"status":"ok"}""")
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(pageJson("listed"))
                    request.path == "/api/students/st-old" -> {
                        hold.await(2, TimeUnit.SECONDS)
                        MockResponse().setBody(studentJson("st-old", "Old Household"))
                    }
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        model.openStudent(Student(id = "st-old", displayName = "Old", createdAt = "2026-01-01T00:00:00Z"))
        model.signOut()
        awaitSignedOut(model)
        identity.signInAs("sid-b", "token-b")
        model.signIn()
        awaitHousehold(model)
        hold.countDown()
        delay(80)
        assertNull(model.state.value.selectedStudent)
        assertTrue(model.state.value.students.none { it.id == "st-old" })
    }

    @Test
    fun delayedRefreshAuthAfterLogoutDoesNotPublishOldSession() = runBlocking {
        server.dispatcher = sessionAndStudents()
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        identity.holdToken()
        model.onResume()
        delay(20)
        model.signOut()
        awaitSignedOut(model)
        identity.signInAs("sid-b", "token-b")
        identity.releaseToken()
        delay(80)
        model.signIn()
        awaitHousehold(model)
        assertTrue(model.state.value.householdOk)
        assertEquals("sid-b", identity.sessionId())
        assertNull(model.state.value.selectedStudent)
    }

    @Test
    fun captureAuthDoesNotBindNewTokenToOldSession() = runBlocking {
        val creates = mutableListOf<String?>()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.method == "POST" && request.path == "/api/students") {
                    creates += request.getHeader("Authorization")
                    return MockResponse().setBody(studentJson("st-new", "Created"))
                }
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        identity.holdToken()
        model.update { it.copy(studentName = "Eve", creatingStudent = true) }
        model.saveStudent()
        identity.signInAs("sid-b", "token-b")
        identity.releaseToken()
        delay(80)
        assertTrue(creates.none { it == "Bearer token-b" })
        assertTrue(creates.none { it == "Bearer token-a" })
    }

    @Test
    fun tokenRolloverCannotMutateOtherAccount() = runBlocking {
        val creates = mutableListOf<String?>()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.method == "POST" && request.path == "/api/students") {
                    creates += request.getHeader("Authorization")
                    return MockResponse().setBody(studentJson("st-new", "Created"))
                }
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        identity.signInAs("sid-b", "token-b")
        model.update { it.copy(studentName = "Eve", creatingStudent = true) }
        model.saveStudent()
        delay(80)
        assertTrue(creates.isEmpty())
        assertFalse(model.state.value.householdOk)
    }

    @Test
    fun revocationFailureDoesNotCallProviderSignOut() = runBlocking {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/auth/logout" -> MockResponse().setResponseCode(503).setBody(
                        """{"code":"unavailable","message":"down","detail":"down"}""",
                    )
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        model.signOut()
        delay(80)
        assertEquals(0, identity.providerSignOuts.get())
        assertTrue(model.state.value.logoutIncomplete)
        assertTrue(model.state.value.signedIn)
        assertEquals("sid-a", identity.sessionId())
    }

    @Test
    fun retryAfterServerSuccessProviderFailureOnlySignsOutProvider() = runBlocking {
        val logouts = AtomicInteger(0)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/auth/logout" -> {
                        logouts.incrementAndGet()
                        MockResponse().setBody("""{"status":"ok"}""")
                    }
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        identity.failProviderOnce = true
        val model = model(identity)
        awaitHousehold(model)
        model.signOut()
        delay(80)
        assertEquals(1, logouts.get())
        assertEquals(1, identity.providerSignOuts.get())
        assertTrue(model.state.value.logoutIncomplete)
        identity.failProviderOnce = false
        model.signOut()
        delay(80)
        assertEquals(1, logouts.get())
        assertEquals(2, identity.providerSignOuts.get())
        assertFalse(model.state.value.signedIn)
        assertFalse(model.state.value.logoutIncomplete)
    }

    @Test
    fun expiredLogoutTokenIsRefreshedWhenLiveSidMatches() = runBlocking {
        val logoutTokens = mutableListOf<String?>()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/auth/logout" -> {
                        logoutTokens += request.getHeader("Authorization")
                        if (request.getHeader("Authorization") == "Bearer token-a") {
                            MockResponse().setResponseCode(401).setBody(
                                """{"code":"unauthenticated","message":"expired","detail":"expired"}""",
                            )
                        } else {
                            MockResponse().setBody("""{"status":"ok"}""")
                        }
                    }
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        model.signOut()
        delay(80)
        assertTrue(model.state.value.logoutIncomplete)
        identity.signInAs("sid-a", "token-a2")
        model.signOut()
        delay(80)
        assertTrue(logoutTokens.contains("Bearer token-a"))
        assertTrue(logoutTokens.contains("Bearer token-a2"))
        assertFalse(model.state.value.signedIn)
    }

    @Test
    fun logoutWithNoLocalContextStillRevokesLiveSdkSession() = runBlocking {
        val logouts = AtomicInteger(0)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/students/st-denied" -> MockResponse().setResponseCode(403).setBody(
                        """{"code":"denied","message":"denied","detail":"denied"}""",
                    )
                    request.path == "/api/auth/logout" -> {
                        logouts.incrementAndGet()
                        MockResponse().setBody("""{"status":"ok"}""")
                    }
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        model.openStudent(Student(id = "st-denied", displayName = "Denied", createdAt = "2026-01-01T00:00:00Z"))
        delay(80)
        assertFalse(model.state.value.householdOk)
        assertEquals("sid-a", identity.sessionId())
        model.signOut()
        delay(80)
        assertEquals(1, logouts.get())
        assertEquals(1, identity.providerSignOuts.get())
        assertNull(identity.sessionId())
    }

    @Test
    fun doubleSignInOnlyCallsProviderOnce() = runBlocking {
        server.dispatcher = sessionAndStudents()
        val identity = FakeIdentity()
        identity.holdSignIn()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        model.signIn()
        model.signIn()
        assertEquals("Wait for the current change to finish.", model.state.value.message)
        identity.releaseSignIn()
        awaitHousehold(model)
        assertEquals(1, identity.signIns.get())
    }

    @Test
    fun cancelDuringTokenFetchReleasesMutationGate() = runBlocking {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/auth/logout" -> MockResponse().setBody("""{"status":"ok"}""")
                    request.method == "POST" && request.path == "/api/students" -> MockResponse().setBody(studentJson("st-1", "Created"))
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        identity.holdToken()
        model.update { it.copy(studentName = "Ada", creatingStudent = true) }
        model.saveStudent()
        delay(20)
        model.signOut()
        identity.releaseToken()
        awaitSignedOut(model)
        identity.signInAs("sid-b", "token-b")
        model.signIn()
        awaitHousehold(model)
        model.update { it.copy(studentName = "Bea", creatingStudent = true) }
        model.saveStudent()
        delay(80)
        assertTrue(model.state.value.students.isEmpty() || !model.state.value.mutating)
        assertNull(model.state.value.message ?: null)
    }

    @Test
    fun concurrentSignInAndMutationAreGated() = runBlocking {
        val hold = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.method == "POST" && request.path == "/api/students") {
                    hold.await(2, TimeUnit.SECONDS)
                    return MockResponse().setBody(studentJson("st-1", "Created"))
                }
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity)
        awaitHousehold(model)
        model.update { it.copy(studentName = "Ada", creatingStudent = true) }
        model.saveStudent()
        repeat(40) {
            if (model.state.value.mutating) return@repeat
            delay(25)
        }
        try {
            assertTrue("mutation never started: ${model.state.value}", model.state.value.mutating)
            model.signIn()
            assertEquals("Wait for the current change to finish.", model.state.value.message)
        } finally {
            hold.countDown()
        }
    }

    @Test
    fun prepareEvaluatesWithoutInstallingUntilEligibleUnattended() = runBlocking {
        val session = RecordingSession()
        session.eligibility = SelfUpdateEligibility(true, false, "Android may replace this package without a prompt")
        val apkBytes = ByteArray(12) { 'x'.code.toByte() }
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/managed-devices" -> MockResponse().setBody("""{"items":[]}""")
                    request.path == "/api/managed-releases" -> MockResponse().setBody(releasePageJson())
                    request.path == "/api/managed-releases/rel-2/apk" -> MockResponse().setBody(okio.Buffer().write(apkBytes))
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val dir = java.io.File.createTempFile("control-dl", "dir").apply {
            delete()
            mkdirs()
        }
        val model = model(identity, updater = coordinator(session), downloadDir = dir)
        awaitHousehold(model)
        model.setDiscovery(checkOnResume = true, periodicEnabled = false, unattendedCatchUp = false)
        model.loadDevices()
        awaitPhase(model, ControlSelfUpdatePhase.EligibleUnattended)
        assertEquals(1, session.evaluations)
        assertEquals(0, session.installs)
        model.installControlUpdate()
        repeat(40) {
            if (session.installs >= 1) return@repeat
            delay(25)
        }
        assertEquals(1, session.evaluations)
        assertEquals(1, session.installs)
    }

    @Test
    fun deferredAndFailedNeverDispatchInstaller() = runBlocking {
        val session = RecordingSession(pending = true, live = true, hasConfirmation = true)
        session.eligibility = SelfUpdateEligibility(true, false, "unattended")
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/managed-devices" -> MockResponse().setBody("""{"items":[]}""")
                    request.path == "/api/managed-releases" -> MockResponse().setBody(releasePageJson())
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val model = model(identity, updater = coordinator(session))
        awaitHousehold(model)
        model.setDiscovery(checkOnResume = false, periodicEnabled = false, unattendedCatchUp = false)
        model.loadDevices()
        awaitPhase(model, ControlSelfUpdatePhase.WaitingConfirmation)
        model.installControlUpdate()
        delay(40)
        assertEquals(0, session.evaluations)
        assertEquals(0, session.installs)
        assertEquals("Finish or cancel the current install confirmation first.", model.state.value.message)
    }

    @Test
    fun periodicCatalogTickIsCancelledByAuthFence() = runBlocking {
        val catalogs = AtomicInteger(0)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.path == "/api/managed-releases") catalogs.incrementAndGet()
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path == "/api/auth/logout" -> MockResponse().setBody("""{"status":"ok"}""")
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/managed-devices" -> MockResponse().setBody("""{"items":[]}""")
                    request.path == "/api/managed-releases" -> MockResponse().setBody("""{"items":[]}""")
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val now = AtomicLong(1_000)
        val model = model(identity, catalogPeriodMs = 40, clock = { now.get() })
        awaitHousehold(model)
        model.update { it.copy(discovery = it.discovery.copy(lastCatalogCheckAtMs = 1)) }
        now.set(1_000)
        model.setDiscovery(periodicEnabled = true, checkOnResume = false)
        delay(40)
        val before = catalogs.get()
        assertTrue("catalog never ticked: $before", before >= 1)
        model.signOut()
        awaitSignedOut(model)
        now.addAndGet(80)
        delay(40)
        assertEquals(before, catalogs.get())
    }

    @Test
    fun periodicCatalogTickDoesNotDownloadApks() = runBlocking {
        val catalogs = AtomicInteger(0)
        val apks = AtomicInteger(0)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.path == "/api/managed-releases") catalogs.incrementAndGet()
                if (request.path?.endsWith("/apk") == true) apks.incrementAndGet()
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/managed-devices" -> MockResponse().setBody("""{"items":[]}""")
                    request.path == "/api/managed-releases" -> MockResponse().setBody(releasePageJson())
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val now = AtomicLong(1_000)
        val model = model(identity, catalogPeriodMs = 40, clock = { now.get() })
        awaitHousehold(model)
        model.update { it.copy(discovery = it.discovery.copy(lastCatalogCheckAtMs = 1)) }
        now.set(1_000)
        model.setDiscovery(periodicEnabled = true, checkOnResume = false, unattendedCatchUp = false)
        delay(40)
        assertTrue("catalog never ticked: ${catalogs.get()}", catalogs.get() >= 1)
        assertEquals(0, apks.get())
    }

    @Test
    fun periodicCatalogNetworkFailureIsObservableAndDoesNotCrash() = runBlocking {
        val catalogs = AtomicInteger(0)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                if (request.path == "/api/managed-releases") {
                    catalogs.incrementAndGet()
                    return MockResponse().setResponseCode(500).setBody("""{"error":"down"}""")
                }
                return when {
                    request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                    request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                    request.path == "/api/managed-devices" -> MockResponse().setBody("""{"items":[]}""")
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
        val identity = FakeIdentity()
        identity.signInAs("sid-a", "token-a")
        val now = AtomicLong(1_000)
        val model = model(identity, catalogPeriodMs = 40, clock = { now.get() })
        awaitHousehold(model)
        model.update { it.copy(discovery = it.discovery.copy(lastCatalogCheckAtMs = 1)) }
        now.set(1_000)
        model.setDiscovery(periodicEnabled = true, checkOnResume = false)
        repeat(40) {
            if (model.state.value.message != null) return@repeat
            delay(25)
        }
        try {
            assertTrue(catalogs.get() >= 1)
            assertTrue("catalog failure was silent: ${model.state.value}", model.state.value.message != null)
            assertTrue(model.state.value.householdOk)
            // Neither the injected clock nor the test scheduler has advanced:
            // a failed due tick must not immediately loop while last-success is stale.
            delay(100)
            assertEquals("A failed tick must back off rather than spin on an overdue success timestamp", 1, catalogs.get())
        } finally {
            model.setDiscovery(periodicEnabled = false)
        }
    }

    private fun model(
        identity: FakeIdentity,
        updater: ControlSelfUpdateCoordinator? = null,
        downloadDir: java.io.File? = null,
        catalogPeriodMs: Long = 15L * 60L * 1000L,
        clock: () -> Long = { System.currentTimeMillis() },
    ): ControlViewModel {
        val http = OkHttpClient()
        val origin = server.url("/").toString()
        return ControlViewModel(
            identity = identity,
            apiBase = origin,
            http = http,
            updater = updater,
            downloadDir = downloadDir,
            io = dispatcher,
            catalogPeriodMs = catalogPeriodMs,
            clock = clock,
            tasksFactory = { token -> ParentTasksRepository(origin, token, http) },
            devicesFactory = { token -> DeviceRepository(origin, token, http) },
        )
    }

    private fun coordinator(session: RecordingSession) = ControlSelfUpdateCoordinator(
        context = object : android.content.ContextWrapper(null) {
            override fun getPackageName() = "com.aleksclark.primer.control"
        },
        trustRoot = "dGVzdA",
        session = session,
        unknownSourcesAllowed = { true },
        installedVersion = { 1L },
        decode = {
            SignedManifest(
                packageName = "com.aleksclark.primer.control",
                channel = "stable",
                versionCode = 2,
                versionName = "0.2.0",
                minSdk = 28,
                supportedAbis = listOf("arm64-v8a"),
                signerSha256 = "a".repeat(64),
                sha256 = "59ffe12a70df15109e0345955e3230a978f31ebc28d8fe3e42d306afb28b8e81",
                byteSize = 12,
            )
        },
    )

    private fun releasePageJson() = """{"items":[{
        "byteSize":12,
        "channel":"stable",
        "id":"rel-2",
        "manifest":{"byteSize":12,"channel":"stable","minSdk":28,"packageName":"com.aleksclark.primer.control","sha256":"59ffe12a70df15109e0345955e3230a978f31ebc28d8fe3e42d306afb28b8e81","signerSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","supportedAbis":["arm64-v8a"],"versionCode":2,"versionName":"0.2.0"},
        "manifestPayloadBase64":"payload",
        "manifestSignature":"sig",
        "minSdk":28,
        "packageName":"com.aleksclark.primer.control",
        "publishedAt":"2026-01-01T00:00:00Z",
        "sha256":"59ffe12a70df15109e0345955e3230a978f31ebc28d8fe3e42d306afb28b8e81",
        "signerSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "signingKeyId":"ed25519-v1",
        "status":"published",
        "supportedAbis":["arm64-v8a"],
        "versionCode":2,
        "versionName":"0.2.0"
    }]}"""

    private class RecordingSession(
        var pending: Boolean = false,
        var live: Boolean = false,
        var hasConfirmation: Boolean = false,
        var eligibility: SelfUpdateEligibility = SelfUpdateEligibility(false, true, "confirm"),
    ) : SelfUpdateCommands {
        var evaluations = 0
        var installs = 0
        override fun snapshot() = SelfUpdateSessionState(
            status = if (pending) "Waiting for system install confirmation" else "No self-update attempted",
            active = pending,
            pendingConfirmation = pending,
            installerSessionLive = live,
            desiredVersion = if (pending) 2 else 0,
            lastOutcome = InstallAttempt(if (pending) "blocked" else "queued"),
            hasConfirmationIntent = hasConfirmation,
        )
        override fun reconcile() = snapshot().lastOutcome
        override fun evaluate(apk: java.io.File, expected: SignedManifest): SelfUpdateEligibility {
            evaluations += 1
            return eligibility
        }
        override fun install(apk: java.io.File, expected: SignedManifest): InstallAttempt {
            installs += 1
            return snapshot().lastOutcome
        }
        override fun handleResult(intent: android.content.Intent, onUserAction: ((android.content.Intent) -> Boolean)?) = snapshot().lastOutcome
        override fun resumeUserAction(onUserAction: (android.content.Intent) -> Boolean) = snapshot().lastOutcome
        override fun cancel() = snapshot().lastOutcome
    }

    private fun sessionAndStudents() = object : Dispatcher() {
        override fun dispatch(request: RecordedRequest): MockResponse {
            return when {
                request.path == "/api/auth/session" -> MockResponse().setBody(sessionJson())
                request.path == "/api/auth/logout" -> MockResponse().setBody("""{"status":"ok"}""")
                request.path?.startsWith("/api/students?") == true -> MockResponse().setBody(emptyPage())
                else -> MockResponse().setResponseCode(404)
            }
        }
    }

    private suspend fun awaitPhase(model: ControlViewModel, phase: ControlSelfUpdatePhase) {
        repeat(40) {
            if (model.state.value.selfUpdate.phase == phase) return
            delay(25)
        }
        throw AssertionError("never reached $phase: ${model.state.value.selfUpdate} message=${model.state.value.message}")
    }

    private suspend fun awaitHousehold(model: ControlViewModel) {
        repeat(40) {
            if (model.state.value.householdOk) return
            delay(25)
        }
        throw AssertionError("household never ready: ${model.state.value}")
    }

    private suspend fun awaitSignedOut(model: ControlViewModel) {
        repeat(40) {
            if (!model.state.value.signedIn && model.state.value.ready) return
            delay(25)
        }
        throw AssertionError("never signed out: ${model.state.value}")
    }

    private fun sessionJson() = """{"subjectRef":"user","tenantId":"ten-1"}"""
    private fun emptyPage() = """{"items":[],"totalCount":0,"limit":20,"offset":0}"""
    private fun pageJson(name: String) =
        """{"items":[{"id":"st-listed","displayName":"$name","createdAt":"2026-01-01T00:00:00Z"}],"totalCount":1,"limit":20,"offset":0}"""
    private fun studentJson(id: String, name: String) =
        """{"id":"$id","displayName":"$name","createdAt":"2026-01-01T00:00:00Z"}"""

    private class FakeIdentity : ParentIdentity {
        override val configured = true
        private val sid = AtomicReference<String?>(null)
        private val token = AtomicReference<String?>(null)
        private val tokenHold = AtomicReference<CompletableDeferred<Unit>?>(null)
        private val signInHold = AtomicReference<CompletableDeferred<Unit>?>(null)
        val providerSignOuts = AtomicInteger(0)
        val signIns = AtomicInteger(0)
        var failProviderOnce = false

        fun signInAs(sessionId: String, jwt: String) {
            sid.set(sessionId)
            token.set(jwt)
        }

        fun holdToken() {
            tokenHold.set(CompletableDeferred())
        }

        fun releaseToken() {
            tokenHold.getAndSet(null)?.complete(Unit)
        }

        fun holdSignIn() {
            signInHold.set(CompletableDeferred())
        }

        fun releaseSignIn() {
            signInHold.getAndSet(null)?.complete(Unit)
        }

        override suspend fun ready() = true
        override suspend fun isSignedIn() = sid.get() != null
        override suspend fun sessionId() = sid.get()
        override suspend fun sessionToken(skipCache: Boolean): String? {
            tokenHold.get()?.await()
            return token.get()
        }

        override suspend fun signIn(email: String, password: String): SignInOutcome {
            signInHold.get()?.await()
            signIns.incrementAndGet()
            val id = sid.get() ?: return SignInOutcome.Failed("no session")
            return SignInOutcome.SignedIn(id)
        }

        override suspend fun signOutProvider(): SignOutOutcome {
            providerSignOuts.incrementAndGet()
            if (failProviderOnce) {
                failProviderOnce = false
                return SignOutOutcome.Failed("Clerk down")
            }
            sid.set(null)
            token.set(null)
            return SignOutOutcome.SignedOut
        }
    }
}
