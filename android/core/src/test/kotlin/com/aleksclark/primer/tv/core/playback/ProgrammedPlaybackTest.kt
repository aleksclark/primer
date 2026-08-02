package com.aleksclark.primer.tv.core.playback

import com.aleksclark.primer.tv.core.data.ApiError
import com.aleksclark.primer.tv.core.data.StoredGrant
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.PlaybackMode
import com.aleksclark.primer.tv.core.testing.FakeGrantStore
import com.aleksclark.primer.tv.core.testing.T0
import com.aleksclark.primer.tv.core.testing.nowJson
import com.aleksclark.primer.tv.core.testing.problemJson
import com.aleksclark.primer.tv.core.testing.programmeJson
import com.aleksclark.primer.tv.core.testing.programmedGrantJson
import com.aleksclark.primer.tv.core.testing.repositoryFor
import com.aleksclark.primer.tv.core.testing.sessionJson
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
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

/** The programmed half of the playback lifecycle: joining, locking, re-syncing. */
@OptIn(ExperimentalCoroutinesApi::class)
class ProgrammedPlaybackTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer().also { it.start() }
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun json(body: String, code: Int = 200) = MockResponse()
        .setResponseCode(code)
        .setHeader("Content-Type", if (code >= 400) "application/problem+json" else "application/json")
        .setBody(body)

    /** Takes the next recorded request, failing rather than blocking forever. */
    private fun nextRequest(): RecordedRequest =
        server.takeRequest(5, TimeUnit.SECONDS) ?: throw AssertionError("expected a request but none arrived")

    /** Pumps the test dispatcher until [predicate] holds, with a wall-clock bound. */
    private fun TestScope.pumpUntil(reason: String, predicate: () -> Boolean) {
        val deadlineNanos = System.nanoTime() + TimeUnit.SECONDS.toNanos(5)
        while (System.nanoTime() < deadlineNanos) {
            runCurrent()
            if (predicate()) return
            Thread.sleep(5)
        }
        throw AssertionError("timed out waiting for $reason")
    }

    /** A probe whose playhead the test drives by hand. */
    private class FakeProbe(
        var positionMillis: Long = 0,
        var isPlaying: Boolean = true,
    ) : PlaybackSessionController.PlayerProbe {
        override fun sample() = PlaybackSessionController.PlayerProbe.Sample(positionMillis, isPlaying)
    }

    /**
     * A controller whose heartbeat loop lives in [TestScope.backgroundScope], so
     * it is cancelled with the test body instead of holding it open, and whose
     * tick is far enough out that these tests drive every beat by hand. An
     * automatic tick would otherwise fire while the test is suspended on a real
     * socket and reorder the requests being asserted on.
     */
    private fun TestScope.controller(store: FakeGrantStore = FakeGrantStore()) =
        PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = backgroundScope,
            clock = { T0 },
            heartbeatIntervalMillis = TimeUnit.HOURS.toMillis(1),
        )

    private suspend fun PlaybackSessionController.tuneIn(scheduleEntryId: String = ENTRY_ID) = start(
        mediaItemId = "media-1",
        mediaClass = MediaClass.EDUCATIONAL,
        runtimeSeconds = 1_800,
        mode = PlaybackMode.PROGRAMMED,
        scheduleEntryId = scheduleEntryId,
    )

    @Test
    fun `tuning in requests a programmed grant and joins at the server's offset`() = runTest {
        server.enqueue(json(programmedGrantJson(startOffsetSeconds = 600), code = 201))
        val store = FakeGrantStore()
        val controller = controller(store)

        controller.tuneIn()

        assertEquals("/api/v1/media/media-1/grant?mode=programmed", nextRequest().path)
        val state = controller.state.value as PlaybackState.Playable
        assertEquals(600, state.resumePositionSeconds)
        assertEquals("programmed", state.grant.mode)
        // The channel is locked down no matter what class the programme is.
        assertFalse(state.controls.seekAllowed)
        assertFalse(state.controls.pauseAllowed)
        assertTrue(state.controls.followsBroadcast)
        assertEquals("programmed", store.stored?.mode)
    }

    @Test
    fun `a programme that has gone off air is refused rather than caught up`() = runTest {
        server.enqueue(
            json(problemJson(403, "that programme is not airing now; the channel does not offer catch-up"), code = 403),
        )
        val controller = controller()

        controller.tuneIn()

        val failed = controller.state.value as PlaybackState.Failed
        assertTrue(failed.error is ApiError.Forbidden)
        assertFalse("asking again will not make it air", failed.recoverable)
    }

    @Test
    fun `a stored grant is never resumed into the channel`() = runTest {
        // The channel moved on while the app was gone, so a stored offset would
        // drop the student into a scene that is no longer airing.
        val store = FakeGrantStore(
            StoredGrant(
                grantId = "old-grant",
                mediaItemId = "media-1",
                streamUrl = "http://jellyfin.local/old",
                startOffsetSeconds = 60,
                mode = "programmed",
                expiresAtEpochSeconds = T0.plusSeconds(3_600).epochSecond,
                positionSeconds = 300,
                watchedSeconds = 240,
                redeemed = true,
            ),
        )
        server.enqueue(json(programmedGrantJson(startOffsetSeconds = 900), code = 201))
        val controller = controller(store)

        controller.tuneIn()

        assertEquals("/api/v1/media/media-1/grant?mode=programmed", nextRequest().path)
        val state = controller.state.value as PlaybackState.Playable
        assertEquals("the join offset comes from the fresh grant", 900, state.resumePositionSeconds)
        assertEquals("33333333-3333-3333-3333-333333333333", state.grant.grantId)
    }

    @Test
    fun `an on-demand grant stored earlier is not adopted by the channel`() = runTest {
        val store = FakeGrantStore(
            StoredGrant(
                grantId = "on-demand-grant",
                mediaItemId = "media-1",
                streamUrl = "http://jellyfin.local/on-demand",
                startOffsetSeconds = 0,
                mode = "on_demand",
                expiresAtEpochSeconds = T0.plusSeconds(3_600).epochSecond,
                positionSeconds = 120,
                watchedSeconds = 120,
                redeemed = true,
            ),
        )
        server.enqueue(json(programmedGrantJson(startOffsetSeconds = 300), code = 201))
        val controller = controller(store)

        controller.tuneIn()

        assertEquals("/api/v1/media/media-1/grant?mode=programmed", nextRequest().path)
        assertEquals(300, (controller.state.value as PlaybackState.Playable).resumePositionSeconds)
    }

    @Test
    fun `re-syncing pulls a lagging playhead up to the broadcast`() = runTest {
        server.enqueue(json(programmedGrantJson(startOffsetSeconds = 600), code = 201))
        server.enqueue(json(nowJson(offsetSeconds = 900, startOffsetSeconds = 900)))
        val controller = controller()
        controller.tuneIn()
        nextRequest()

        val probe = FakeProbe(positionMillis = 600_000)
        controller.attachPlayer(probe)

        val outcome = controller.resyncBroadcast()

        assertEquals("/api/v1/now", nextRequest().path)
        assertEquals(BroadcastSync.Corrected(900), outcome)
        // The player is told to jump: the local clock was never consulted.
        assertEquals(900, controller.broadcastSeek.value?.positionSeconds)
    }

    @Test
    fun `re-syncing leaves a playhead that is level alone`() = runTest {
        server.enqueue(json(programmedGrantJson(startOffsetSeconds = 600), code = 201))
        server.enqueue(json(nowJson(offsetSeconds = 603, startOffsetSeconds = 603)))
        val controller = controller()
        controller.tuneIn()
        nextRequest()
        controller.attachPlayer(FakeProbe(positionMillis = 600_000))

        assertEquals(BroadcastSync.InSync, controller.resyncBroadcast())
        assertNull("no needless seek", controller.broadcastSeek.value)
    }

    @Test
    fun `re-syncing reports that the channel has moved on to another programme`() = runTest {
        server.enqueue(json(programmedGrantJson(), code = 201))
        server.enqueue(
            json(
                nowJson(
                    onAir = programmeJson(scheduleEntryId = "another-entry", title = "Gravity"),
                    offsetSeconds = 120,
                    startOffsetSeconds = 120,
                ),
            ),
        )
        val controller = controller()
        controller.tuneIn()
        nextRequest()
        controller.attachPlayer(FakeProbe(positionMillis = 600_000))

        assertEquals(BroadcastSync.ProgrammeChanged, controller.resyncBroadcast())
        assertNull(controller.broadcastSeek.value)
    }

    @Test
    fun `re-syncing into a gap reports that the channel has moved on`() = runTest {
        server.enqueue(json(programmedGrantJson(), code = 201))
        server.enqueue(json(nowJson(onAir = null, offsetSeconds = 0, startOffsetSeconds = 0)))
        val controller = controller()
        controller.tuneIn()
        nextRequest()
        controller.attachPlayer(FakeProbe())

        assertEquals(BroadcastSync.ProgrammeChanged, controller.resyncBroadcast())
    }

    @Test
    fun `an unreachable channel does not stop playback`() = runTest {
        server.enqueue(json(programmedGrantJson(startOffsetSeconds = 600), code = 201))
        server.enqueue(MockResponse().setResponseCode(500).setBody("{}"))
        val controller = controller()
        controller.tuneIn()
        nextRequest()
        controller.attachPlayer(FakeProbe(positionMillis = 600_000))

        val outcome = controller.resyncBroadcast()

        assertTrue(outcome is BroadcastSync.Unavailable)
        assertNull(controller.broadcastSeek.value)
        assertTrue("the stream keeps running", controller.state.value is PlaybackState.Playable)
    }

    @Test
    fun `re-syncing an on-demand session does nothing`() = runTest {
        server.enqueue(json(programmedGrantJson(), code = 201))
        val controller = controller()
        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)
        nextRequest()

        assertEquals(BroadcastSync.NotProgrammed, controller.resyncBroadcast())
        assertEquals("the channel is not consulted", 0, server.requestCount - 1)
    }

    @Test
    fun `programmed viewing heartbeats and records a session`() = runTest {
        server.dispatcher = channelDispatcher()
        val store = FakeGrantStore()
        val controller = controller(store)
        controller.tuneIn()
        assertEquals("/api/v1/media/media-1/grant?mode=programmed", nextRequest().path)

        val probe = FakeProbe(positionMillis = 630_000)
        controller.attachPlayer(probe)
        controller.sampleAndBeat()
        pumpUntil("the heartbeat to land") { controller.lastProgress.value != null }

        probe.positionMillis = 1_800_000
        controller.finish(completed = true)
        pumpUntil("the session to close") { controller.state.value is PlaybackState.Finished }

        // Programmed viewing goes through the same session machinery as
        // on-demand, so phase 5 has hours to report for the channel too.
        val paths = drainPaths()
        assertTrue(
            "progress was reported: $paths",
            paths.any { it == "/api/v1/grants/$GRANT_ID/heartbeat" },
        )
        assertTrue(
            "the session was closed: $paths",
            paths.any { it == "/api/v1/grants/$GRANT_ID/complete" },
        )

        val finished = controller.state.value as PlaybackState.Finished
        assertTrue(finished.completed)
        // The channel is not rationed: the grid, not an availability window,
        // authorized this viewing.
        assertFalse(finished.playConsumed)
        assertNull("the finished grant is cleared", store.stored)
    }

    /**
     * Answers by path rather than from a queue.
     *
     * A queue runs dry if a heartbeat lands while the test is suspended on a
     * real socket, and a dropped connection is a different outcome from the one
     * under test.
     */
    private fun channelDispatcher() = object : Dispatcher() {
        override fun dispatch(request: RecordedRequest): MockResponse {
            val path = request.path.orEmpty()
            return when {
                path.endsWith("/grant") || path.contains("/grant?") ->
                    json(programmedGrantJson(startOffsetSeconds = 600), code = 201)
                else -> json(
                    sessionJson(
                        watchedSeconds = 1_200,
                        maxPositionSeconds = 1_800,
                        completed = path.endsWith("/complete"),
                    ),
                )
            }
        }
    }

    /** Every request the server has recorded so far. */
    private fun drainPaths(): List<String> = buildList {
        while (true) {
            val request = server.takeRequest(50, TimeUnit.MILLISECONDS) ?: return@buildList
            add(request.path.orEmpty())
        }
    }

    @Test
    fun `a channel session that cannot be closed is never charged a play`() = runTest {
        server.enqueue(json(programmedGrantJson(startOffsetSeconds = 0), code = 201))
        server.enqueue(MockResponse().setResponseCode(500).setBody("{}"))
        val controller = controller()
        controller.start(
            mediaItemId = "media-1",
            mediaClass = MediaClass.ENTERTAINMENT,
            runtimeSeconds = 1_800,
            mode = PlaybackMode.PROGRAMMED,
            scheduleEntryId = ENTRY_ID,
        )
        nextRequest()
        controller.attachPlayer(FakeProbe(positionMillis = 1_800_000))

        controller.finish(completed = true)
        pumpUntil("the session to close") { controller.state.value is PlaybackState.Finished }

        val finished = controller.state.value as PlaybackState.Finished
        assertFalse(
            "an entertainment programme watched on the channel still burns nothing",
            finished.playConsumed,
        )
    }

    private companion object {
        const val ENTRY_ID = "66666666-6666-6666-6666-666666666666"
        const val GRANT_ID = "33333333-3333-3333-3333-333333333333"
    }
}
