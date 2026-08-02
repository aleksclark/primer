package com.aleksclark.primer.tv.app.ui

import com.aleksclark.primer.tv.app.FakeGrantStore
import com.aleksclark.primer.tv.app.FakeSettingsStore
import com.aleksclark.primer.tv.app.data.AppContainer
import com.aleksclark.primer.tv.core.data.DeviceSettings
import com.aleksclark.primer.tv.core.playback.PlaybackState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Channel behaviour at the view-model level, against a real HTTP stack.
 *
 * Like the catalog tests these run on a real dispatcher and poll, because the
 * repository suspends on a socket that virtual time cannot advance past.
 */
class ChannelViewModelTest {

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

    private fun json(body: String, code: Int = 200) = MockResponse()
        .setResponseCode(code)
        .setHeader("Content-Type", if (code >= 400) "application/problem+json" else "application/json")
        .setBody(body)

    /** Waits for an asynchronous outcome, failing the test rather than hanging. */
    private fun awaitUntil(reason: String, predicate: () -> Boolean) = runBlocking {
        runCatching {
            withTimeout(5_000) { while (!predicate()) delay(10) }
        }.onFailure { throw AssertionError("timed out waiting for $reason") }
        Unit
    }

    private fun pairedViewModel(): TvViewModel {
        val settings = FakeSettingsStore(
            DeviceSettings(baseUrl = server.url("/").toString(), token = "secret"),
        )
        val viewModel = TvViewModel(AppContainer(settings, FakeGrantStore()), scope)
        awaitUntil("settings to load") { viewModel.settings.value.isPaired }
        return viewModel
    }

    @Test
    fun `opening the channel loads what is on now`() {
        server.enqueue(json(nowBody(offsetSeconds = 900)))

        val viewModel = pairedViewModel()
        viewModel.openChannel()
        awaitUntil("the channel to load") { viewModel.channel.value.now != null }

        assertEquals(Destination.CHANNEL, viewModel.destination.value)
        val state = viewModel.channel.value
        assertEquals("Inertia", state.onAir?.title)
        assertEquals(900, state.now?.offsetSeconds)
        assertTrue(state.tunable)
        assertFalse(state.inGap)
    }

    @Test
    fun `a gap is surfaced as nothing on air`() {
        server.enqueue(json(gapBody()))

        val viewModel = pairedViewModel()
        viewModel.openChannel()
        awaitUntil("the channel to load") { viewModel.channel.value.now != null }

        val state = viewModel.channel.value
        assertNull(state.onAir)
        assertTrue(state.inGap)
        assertFalse("there is nothing to tune in to", state.tunable)
        assertEquals("Gravity", state.now?.next?.title)
    }

    @Test
    fun `a programme the box cannot decode is not tunable`() {
        server.enqueue(json(nowBody(directPlayOk = false)))

        val viewModel = pairedViewModel()
        viewModel.openChannel()
        awaitUntil("the channel to load") { viewModel.channel.value.now != null }

        assertNotNull(viewModel.channel.value.onAir)
        assertFalse(viewModel.channel.value.tunable)
    }

    @Test
    fun `the epg screen loads the server's own today`() {
        server.enqueue(json(scheduleBody()))

        val viewModel = pairedViewModel()
        viewModel.openEpg()
        awaitUntil("the grid to load") { viewModel.epg.value.day != null }

        assertEquals(Destination.EPG, viewModel.destination.value)
        assertEquals("/api/v1/schedule", server.takeRequest().path)
        assertEquals(listOf("Inertia", "Gravity"), viewModel.epg.value.day?.programmes?.map { it.title })
        assertFalse(viewModel.epg.value.isEmpty)
    }

    @Test
    fun `tuning in joins at the server's offset with the channel locked down`() {
        server.dispatcher = channelDispatcher(startOffsetSeconds = 900)

        val viewModel = pairedViewModel()
        viewModel.openChannel()
        awaitUntil("the channel to load") { viewModel.channel.value.now != null }

        viewModel.tuneIn()
        awaitUntil("playback to start") { viewModel.playback.value is PlaybackState.Playable }

        assertEquals(Destination.PLAYER, viewModel.destination.value)
        val playable = viewModel.playback.value as PlaybackState.Playable
        assertEquals(900, playable.resumePositionSeconds)
        assertFalse(playable.controls.seekAllowed)
        assertFalse(playable.controls.pauseAllowed)
        assertTrue(playable.controls.followsBroadcast)
        assertEquals("Inertia", viewModel.playingTitle())
        assertFalse(viewModel.playbackControls().showTransportControls)
    }

    @Test
    fun `tuning in with nothing on air does nothing`() {
        server.enqueue(json(gapBody()))

        val viewModel = pairedViewModel()
        viewModel.openChannel()
        awaitUntil("the channel to load") { viewModel.channel.value.now != null }

        viewModel.tuneIn()

        assertEquals(Destination.CHANNEL, viewModel.destination.value)
        assertTrue(viewModel.playback.value is PlaybackState.Idle)
    }

    @Test
    fun `back leaves the channel player for the channel, not a catalog detail`() {
        server.dispatcher = channelDispatcher()

        val viewModel = pairedViewModel()
        viewModel.openChannel()
        awaitUntil("the channel to load") { viewModel.channel.value.now != null }
        viewModel.tuneIn()
        awaitUntil("playback to start") { viewModel.playback.value is PlaybackState.Playable }

        assertTrue("back is the whole of the programmed transport", viewModel.back())

        assertEquals(Destination.CHANNEL, viewModel.destination.value)
    }

    @Test
    fun `back walks the epg to the channel and the channel to the catalog`() {
        server.dispatcher = channelDispatcher()
        val viewModel = pairedViewModel()

        viewModel.openEpg()
        assertTrue(viewModel.back())
        assertEquals(Destination.CHANNEL, viewModel.destination.value)

        assertTrue(viewModel.back())
        assertEquals(Destination.CATALOG, viewModel.destination.value)
    }

    @Test
    fun `re-syncing after the channel has moved on closes the session`() {
        var onAirEntry = ENTRY_ID
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.path.orEmpty()
                return when {
                    path.endsWith("/grant") || path.contains("/grant?") ->
                        json(grantBody(startOffsetSeconds = 60), code = 201)
                    path.startsWith("/api/v1/now") -> json(nowBody(scheduleEntryId = onAirEntry))
                    else -> json(sessionBody())
                }
            }
        }

        val viewModel = pairedViewModel()
        viewModel.openChannel()
        awaitUntil("the channel to load") { viewModel.channel.value.now != null }
        viewModel.tuneIn()
        awaitUntil("playback to start") { viewModel.playback.value is PlaybackState.Playable }

        // The channel has moved on while the app was away.
        onAirEntry = "88888888-8888-8888-8888-888888888888"
        viewModel.resyncBroadcast()

        awaitUntil("the session to be dropped") { viewModel.destination.value == Destination.CHANNEL }
        awaitUntil("playback to reset") { viewModel.playback.value is PlaybackState.Idle }
    }

    /** Answers by path, so a heartbeat landing mid-test cannot drain a queue. */
    private fun channelDispatcher(startOffsetSeconds: Int = 600) = object : Dispatcher() {
        override fun dispatch(request: RecordedRequest): MockResponse {
            val path = request.path.orEmpty()
            return when {
                path.endsWith("/grant") || path.contains("/grant?") ->
                    json(grantBody(startOffsetSeconds), code = 201)
                path.startsWith("/api/v1/now") -> json(nowBody(offsetSeconds = startOffsetSeconds))
                path.startsWith("/api/v1/schedule") -> json(scheduleBody())
                else -> json(sessionBody())
            }
        }
    }

    private fun programmeBody(
        scheduleEntryId: String = ENTRY_ID,
        title: String = "Inertia",
        airsAt: String = "2025-03-01T11:45:00Z",
        endsAt: String = "2025-03-01T12:15:00Z",
        directPlayOk: Boolean = true,
    ): String = """
    {
      "scheduleEntryId": "$scheduleEntryId",
      "mediaItemId": "$MEDIA_ID",
      "title": "$title",
      "overview": "About $title.",
      "class": "educational",
      "subjectTags": ["science"],
      "runtimeSeconds": 1800,
      "airsAt": "$airsAt",
      "endsAt": "$endsAt",
      "block": "morning",
      "joinInProgress": true,
      "directPlayOk": $directPlayOk,
      "imageUrl": "/images/$MEDIA_ID/Primary"
    }
    """.trimIndent()

    private fun nowBody(
        scheduleEntryId: String = ENTRY_ID,
        offsetSeconds: Int = 600,
        directPlayOk: Boolean = true,
    ): String = """
    {
      "onAir": ${programmeBody(scheduleEntryId = scheduleEntryId, directPlayOk = directPlayOk)},
      "offsetSeconds": $offsetSeconds,
      "startOffsetSeconds": $offsetSeconds,
      "nextStartsInSeconds": 900,
      "serverTime": "2025-03-01T12:00:00Z"
    }
    """.trimIndent()

    private fun gapBody(): String = """
    {
      "next": ${programmeBody(title = "Gravity", airsAt = "2025-03-01T12:15:00Z", endsAt = "2025-03-01T12:45:00Z")},
      "offsetSeconds": 0,
      "startOffsetSeconds": 0,
      "nextStartsInSeconds": 900,
      "serverTime": "2025-03-01T12:00:00Z"
    }
    """.trimIndent()

    private fun scheduleBody(): String = """
    {
      "day": "2025-03-01",
      "timezone": "America/Chicago",
      "dayStartsAt": "2025-03-01T06:00:00Z",
      "dayEndsAt": "2025-03-02T06:00:00Z",
      "programmes": [
        ${programmeBody()},
        ${programmeBody(
        scheduleEntryId = "77777777-7777-7777-7777-777777777777",
        title = "Gravity",
        airsAt = "2025-03-01T12:15:00Z",
        endsAt = "2025-03-01T12:45:00Z",
    )}
      ],
      "serverTime": "2025-03-01T12:00:00Z"
    }
    """.trimIndent()

    private fun grantBody(startOffsetSeconds: Int = 600): String = """
    {
      "grantId": "99999999-9999-9999-9999-999999999999",
      "streamUrl": "http://jellyfin.local/Videos/jf-1/stream?static=true",
      "startOffsetSeconds": $startOffsetSeconds,
      "mode": "programmed",
      "expiresAt": "2099-01-01T00:00:00Z",
      "serverTime": "2025-03-01T12:00:00Z"
    }
    """.trimIndent()

    private fun sessionBody(): String = """
    {
      "session": {
        "id": "44444444-4444-4444-4444-444444444444",
        "grantId": "99999999-9999-9999-9999-999999999999",
        "mediaItemId": "$MEDIA_ID",
        "deviceId": "55555555-5555-5555-5555-555555555555",
        "startedAt": "2025-03-01T12:00:00Z",
        "watchedSeconds": 60,
        "maxPositionSeconds": 660,
        "completed": false,
        "createdAt": "2025-03-01T12:00:00Z",
        "updatedAt": "2025-03-01T12:01:00Z"
      },
      "playConsumed": false,
      "serverTime": "2025-03-01T12:01:00Z"
    }
    """.trimIndent()

    private companion object {
        const val ENTRY_ID = "66666666-6666-6666-6666-666666666666"
        const val MEDIA_ID = "11111111-1111-1111-1111-111111111111"
    }
}
