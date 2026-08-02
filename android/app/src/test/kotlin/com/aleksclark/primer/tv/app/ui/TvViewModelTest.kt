package com.aleksclark.primer.tv.app.ui

import com.aleksclark.primer.tv.app.FakeGrantStore
import com.aleksclark.primer.tv.app.FakeSettingsStore
import com.aleksclark.primer.tv.app.data.AppContainer
import com.aleksclark.primer.tv.core.data.DeviceSettings
import com.aleksclark.primer.tv.core.domain.FormFactor
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * View model behaviour against a real HTTP stack.
 *
 * The repository talks over OkHttp on its own threads, so these tests run on a
 * real dispatcher and poll for the resulting state rather than advancing
 * virtual time, which would not wait for a socket.
 */
class TvViewModelTest {

    private lateinit var server: MockWebServer
    private lateinit var scope: CoroutineScope

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    }

    @After
    fun tearDown() {
        scope.cancel()
        server.shutdown()
    }

    private fun container(settings: DeviceSettings = DeviceSettings()) = AppContainer(
        settingsStore = FakeSettingsStore(settings),
        grantStore = FakeGrantStore(),
    )

    private fun viewModel(container: AppContainer) = TvViewModel(container, scope)

    private fun baseUrl() = server.url("/").toString()

    /** Waits for an asynchronous outcome, failing the test rather than hanging. */
    private fun awaitUntil(reason: String, predicate: () -> Boolean) = runBlocking {
        runCatching {
            withTimeout(5_000) { while (!predicate()) delay(10) }
        }.onFailure { throw AssertionError("timed out waiting for $reason") }
        Unit
    }

    private fun pairedContainer(): Pair<AppContainer, FakeSettingsStore> {
        val settings = FakeSettingsStore(DeviceSettings(baseUrl = baseUrl(), token = "secret"))
        return AppContainer(settings, FakeGrantStore()) to settings
    }

    @Test
    fun `pairing is blocked until both fields are filled`() {
        val viewModel = viewModel(container())

        assertFalse(viewModel.pairing.value.canSubmit)

        viewModel.onBaseUrlChanged("tv.local:8081")
        assertFalse("a code is still required", viewModel.pairing.value.canSubmit)

        viewModel.onCodeChanged("ABCD")
        assertTrue(viewModel.pairing.value.canSubmit)
    }

    @Test
    fun `an unparseable server address is reported without a request`() {
        val viewModel = viewModel(container())

        viewModel.onBaseUrlChanged("http://")
        viewModel.onCodeChanged("ABCD")
        viewModel.submitPairing()

        assertNotNull(viewModel.pairing.value.error)
        assertEquals("no call should have been made", 0, server.requestCount)
    }

    @Test
    fun `a successful pairing stores the token and lands on the catalog`() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"token":"secret","device":{"id":"d1","name":"Playroom","kind":"tv_box","createdAt":"2025-02-01T00:00:00Z","updatedAt":"2025-02-01T00:00:00Z"}}""",
            ),
        )
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"items":[],"serverTime":"2025-03-01T12:00:00Z"}"""),
        )

        val settings = FakeSettingsStore()
        val viewModel = viewModel(AppContainer(settings, FakeGrantStore()))

        viewModel.onBaseUrlChanged(baseUrl())
        viewModel.onCodeChanged("ABCD")
        viewModel.submitPairing()

        awaitUntil("the token to be stored") { runBlocking { settings.current().isPaired } }
        assertEquals("secret", runBlocking { settings.current().token })
        assertEquals(Destination.CATALOG, viewModel.destination.value)
        assertNull(viewModel.pairing.value.error)
    }

    @Test
    fun `a rejected code surfaces the server's explanation and stays on pairing`() {
        server.enqueue(
            MockResponse().setResponseCode(403).setBody(
                """{"status":403,"title":"Forbidden","detail":"pairing code already used"}""",
            ),
        )

        val viewModel = viewModel(container())

        viewModel.onBaseUrlChanged(baseUrl())
        viewModel.onCodeChanged("ABCD")
        viewModel.submitPairing()

        awaitUntil("the refusal to surface") { viewModel.pairing.value.error != null }
        assertEquals("pairing code already used", viewModel.pairing.value.error)
        assertFalse(viewModel.pairing.value.submitting)
    }

    @Test
    fun `a revoked token during a catalog refresh forces re-pairing`() {
        server.enqueue(
            MockResponse().setResponseCode(401).setBody(
                """{"status":401,"title":"Unauthorized","detail":"device revoked"}""",
            ),
        )

        val settings = FakeSettingsStore(
            DeviceSettings(baseUrl = baseUrl(), token = "stale", deviceId = "d1"),
        )
        val viewModel = viewModel(AppContainer(settings, FakeGrantStore()))
        awaitUntil("settings to load") { viewModel.settings.value.isPaired }

        viewModel.refreshCatalog()

        awaitUntil("the dead token to be dropped") { runBlocking { settings.current().token } == null }
        assertEquals(Destination.PAIRING, viewModel.destination.value)
    }

    @Test
    fun `unpairing clears credentials and the stored grant`() {
        val (container, settings) = pairedContainer()
        val viewModel = viewModel(container)

        viewModel.unpair()

        awaitUntil("the pairing to be cleared") { runBlocking { settings.current().token } == null }
        assertEquals(Destination.PAIRING, viewModel.destination.value)
        assertEquals(
            "the server address is kept so re-pairing does not need retyping",
            baseUrl(),
            runBlocking { settings.current().baseUrl },
        )
    }

    @Test
    fun `back exits from the catalog but unwinds every other screen`() {
        val (container, _) = pairedContainer()
        val viewModel = viewModel(container)
        awaitUntil("settings to load") { viewModel.settings.value.isPaired }

        viewModel.openSettings()
        assertTrue(viewModel.back())
        assertEquals(Destination.CATALOG, viewModel.destination.value)

        assertFalse("back at the top level exits the app", viewModel.back())
    }

    @Test
    fun `an unpaired device cannot escape pairing with back`() {
        val viewModel = viewModel(container())

        viewModel.openPairing()

        assertFalse(viewModel.back())
        assertEquals(Destination.PAIRING, viewModel.destination.value)
    }

    @Test
    fun `an empty catalog reports emptiness rather than an error`() {
        server.enqueue(
            MockResponse().setResponseCode(200)
                .setBody("""{"items":[],"serverTime":"2025-03-01T12:00:00Z"}"""),
        )

        val (container, _) = pairedContainer()
        val viewModel = viewModel(container)
        awaitUntil("settings to load") { viewModel.settings.value.isPaired }

        viewModel.refreshCatalog()

        awaitUntil("the catalog to settle") { !viewModel.catalog.value.loading }
        assertTrue(viewModel.catalog.value.isEmpty)
        assertNull(viewModel.catalog.value.error)
    }

    @Test
    fun `a catalog groups items into rails and marks rationed titles`() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"items":[${catalogItem(DOC_ID, "Inertia", "educational")},${catalogItem(FILM_ID, "Apollo 13", "entertainment")}],"serverTime":"2025-03-01T12:00:00Z"}""",
            ),
        )

        val (container, _) = pairedContainer()
        val viewModel = viewModel(container)
        awaitUntil("settings to load") { viewModel.settings.value.isPaired }

        viewModel.refreshCatalog()
        awaitUntil("the catalog to load") { viewModel.catalog.value.view != null }

        val view = viewModel.catalog.value.view!!
        assertEquals(listOf("Learn", "Entertainment"), view.rails.map { it.title })

        val documentary = viewModel.catalog.value.card(DOC_ID)!!
        val film = viewModel.catalog.value.card(FILM_ID)!!
        assertFalse("educational content is not rationed", documentary.consumesPlay)
        assertTrue("entertainment is watch-once", film.consumesPlay)
        assertTrue("nothing is watched yet", documentary.playable && film.playable)
    }

    @Test
    fun `a network failure is reported instead of crashing the catalog`() {
        server.shutdown()

        val (container, _) = pairedContainer()
        val viewModel = viewModel(container)
        awaitUntil("settings to load") { viewModel.settings.value.isPaired }

        viewModel.refreshCatalog()

        awaitUntil("the failure to surface") { viewModel.catalog.value.error != null }
        assertNull(viewModel.catalog.value.view)
    }

    @Test
    fun `selecting an item opens its detail screen`() {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"items":[${catalogItem(DOC_ID, "Inertia", "educational")}],"serverTime":"2025-03-01T12:00:00Z"}""",
            ),
        )

        val (container, _) = pairedContainer()
        val viewModel = viewModel(container)
        awaitUntil("settings to load") { viewModel.settings.value.isPaired }
        viewModel.refreshCatalog()
        awaitUntil("the catalog to load") { viewModel.catalog.value.view != null }

        viewModel.openDetail(DOC_ID)

        assertEquals(Destination.DETAIL, viewModel.destination.value)
        assertEquals("Inertia", viewModel.playingTitle())
        // Educational study material may be re-watched, so seeking is offered.
        assertTrue(viewModel.playbackControls().seekAllowed)
    }

    @Test
    fun `an unknown selection degrades to the most restrictive on-demand policy`() {
        val controls = viewModel(container()).playbackControls()
        assertFalse(controls.seekAllowed)
        assertTrue(controls.pauseAllowed)
    }

    @Test
    fun `the television ui mode picks the leanback shell and anything else the touch shell`() {
        assertEquals(FormFactor.TELEVISION, FormFactor.fromUiModeType(FormFactor.UI_MODE_TYPE_TELEVISION))
        assertEquals(FormFactor.TABLET, FormFactor.fromUiModeType(0))
        assertEquals(FormFactor.TABLET, FormFactor.fromUiModeType(1))
    }

    private fun catalogItem(id: String, title: String, mediaClass: String): String = """
    {
      "mediaItem": {
        "id": "$id",
        "jellyfinItemId": "jf-$id",
        "title": "$title",
        "sortTitle": "$title",
        "overview": "About $title.",
        "class": "$mediaClass",
        "runtimeSeconds": 1800,
        "subjectTags": ["science"],
        "standardCodes": ["TN.SCI.6.PS2.1"],
        "qualityNotes": "",
        "container": "mkv",
        "videoCodec": "h264",
        "audioCodec": "aac",
        "directPlayOk": true,
        "imageTag": "tag-1",
        "createdAt": "2025-02-01T00:00:00Z",
        "updatedAt": "2025-02-01T00:00:00Z"
      },
      "availabilityWindowId": "33333333-3333-3333-3333-333333333333",
      "windowEndsAt": "2025-03-08T12:00:00Z",
      "imageUrl": "/images/$id/Primary"
    }
    """.trimIndent()

    private companion object {
        const val DOC_ID = "11111111-1111-1111-1111-111111111111"
        const val FILM_ID = "22222222-2222-2222-2222-222222222222"
    }
}
