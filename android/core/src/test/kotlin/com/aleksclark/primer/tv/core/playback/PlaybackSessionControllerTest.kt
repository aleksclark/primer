package com.aleksclark.primer.tv.core.playback

import com.aleksclark.primer.tv.core.data.ApiError
import com.aleksclark.primer.tv.core.data.StoredGrant
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.testing.FakeGrantStore
import com.aleksclark.primer.tv.core.testing.T0
import com.aleksclark.primer.tv.core.testing.grantJson
import com.aleksclark.primer.tv.core.testing.problemJson
import com.aleksclark.primer.tv.core.testing.repositoryFor
import com.aleksclark.primer.tv.core.testing.sessionJson
import java.time.Instant
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.mockwebserver.MockResponse
import java.util.concurrent.TimeUnit
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okhttp3.mockwebserver.SocketPolicy
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class PlaybackSessionControllerTest {
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

    /**
     * Takes the next recorded request, failing rather than blocking forever.
     *
     * A response that drops the connection is never recorded, so an unbounded
     * take would hang the whole suite instead of reporting the missing call.
     */
    private fun nextRequest(): RecordedRequest =
        server.takeRequest(5, TimeUnit.SECONDS) ?: throw AssertionError("expected a request but none arrived")

    /**
     * Pumps the test dispatcher until [predicate] holds.
     *
     * Heartbeats cross a real socket, so the controller suspends on OkHttp's
     * own threads and its continuation only runs when the test scheduler is
     * pumped again. Virtual time alone cannot advance past a call that is
     * genuinely in flight, so the test waits on wall-clock for the response and
     * fails rather than hanging if it never lands.
     */
    private fun TestScope.pumpUntil(reason: String, predicate: () -> Boolean) {
        val deadlineNanos = System.nanoTime() + TimeUnit.SECONDS.toNanos(5)
        while (System.nanoTime() < deadlineNanos) {
            runCurrent()
            if (predicate()) return
            Thread.sleep(5)
        }
        throw AssertionError("timed out waiting for $reason")
    }

    /** Waits for the controller to reach a state of type [T]. */
    private inline fun <reified T : PlaybackState> TestScope.awaitState(
        controller: PlaybackSessionController,
    ): T {
        pumpUntil(T::class.simpleName ?: "state") { controller.state.value is T }
        return controller.state.value as T
    }

    /** A probe whose playhead the test drives by hand. */
    private class FakeProbe(
        var positionMillis: Long = 0,
        var isPlaying: Boolean = true,
    ) : PlaybackSessionController.PlayerProbe {
        override fun sample() = PlaybackSessionController.PlayerProbe.Sample(positionMillis, isPlaying)
    }

    @Test
    fun `start requests a grant and becomes playable with the stream url`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        val store = FakeGrantStore()
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)

        val state = controller.state.value as PlaybackState.Playable
        assertTrue(state.grant.streamUrl.contains("static=true"))
        assertEquals(0, state.resumePositionSeconds)
        // Educational content is study material: rewinding is part of using it.
        assertTrue(state.controls.seekAllowed)
        assertTrue(state.controls.pauseAllowed)
        // The grant is persisted immediately so a crash before the first
        // heartbeat still leaves something to resume.
        assertEquals("33333333-3333-3333-3333-333333333333", store.stored?.grantId)
        assertFalse(store.stored!!.redeemed)
    }

    @Test
    fun `entertainment items are playable but not seekable`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.ENTERTAINMENT, runtimeSeconds = 5_400)

        val controls = (controller.state.value as PlaybackState.Playable).controls
        assertTrue(controls.pauseAllowed)
        assertFalse(controls.seekAllowed)
        assertFalse(controls.showSeekBar)
    }

    @Test
    fun `a refused grant fails unrecoverably so the UI does not offer a retry`() = runTest {
        server.enqueue(json(problemJson(403, "item is unavailable or its play was already consumed"), code = 403))
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.ENTERTAINMENT, runtimeSeconds = 5_400)

        val failed = controller.state.value as PlaybackState.Failed
        assertTrue(failed.error is ApiError.Forbidden)
        assertFalse(failed.recoverable)
    }

    @Test
    fun `a network failure while requesting a grant is recoverable`() = runTest {
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)

        val failed = controller.state.value as PlaybackState.Failed
        assertTrue(failed.error is ApiError.Network)
        assertTrue(failed.recoverable)
    }

    @Test
    fun `a temporary media source outage while requesting a grant is recoverable`() = runTest {
        server.enqueue(json(problemJson(503, "The media source is unavailable."), code = 503))
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)

        val failed = controller.state.value as PlaybackState.Failed
        assertTrue(failed.error is ApiError.Unavailable)
        assertTrue(failed.recoverable)
    }

    @Test
    fun `heartbeats fire on the interval and report accumulated watch time`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        server.enqueue(json(sessionJson(watchedSeconds = 30, maxPositionSeconds = 30)))
        server.enqueue(json(sessionJson(watchedSeconds = 60, maxPositionSeconds = 60)))

        var elapsed = 0L
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
            elapsedRealtime = { elapsed },
        )
        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)
        nextRequest()

        val probe = FakeProbe()
        controller.attachPlayer(probe)

        // First tick establishes the baseline; no watch time is credited yet
        // because there is no previous sample to measure against.
        elapsed = 30_000
        probe.positionMillis = 30_000
        advanceTimeBy(HEARTBEAT_INTERVAL_MILLIS)
        runCurrent()

        val first = Json.parseToJsonElement(nextRequest().body.readUtf8()) as JsonObject
        assertEquals("30", first["positionSeconds"]?.jsonPrimitive?.content)
        assertEquals("0", first["watchedSeconds"]?.jsonPrimitive?.content)
        pumpUntil("the first heartbeat to be acknowledged") { controller.lastProgress.value != null }

        elapsed = 60_000
        probe.positionMillis = 60_000
        advanceTimeBy(HEARTBEAT_INTERVAL_MILLIS)
        runCurrent()

        val second = Json.parseToJsonElement(nextRequest().body.readUtf8()) as JsonObject
        assertEquals("60", second["positionSeconds"]?.jsonPrimitive?.content)
        assertEquals("30", second["watchedSeconds"]?.jsonPrimitive?.content)

        controller.detachPlayer()
    }

    @Test
    fun `a failed heartbeat does not interrupt playback and the next one still reports`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))
        server.enqueue(json(sessionJson(watchedSeconds = 30, maxPositionSeconds = 60)))

        var elapsed = 0L
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
            elapsedRealtime = { elapsed },
        )
        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)
        nextRequest()

        val probe = FakeProbe()
        controller.attachPlayer(probe)

        elapsed = 30_000
        probe.positionMillis = 30_000
        advanceTimeBy(HEARTBEAT_INTERVAL_MILLIS)
        pumpUntil("the dropped beat to be attempted") { server.requestCount >= 2 }

        // The Wi-Fi blip must not stop the film.
        assertTrue(controller.state.value is PlaybackState.Playable)

        elapsed = 60_000
        probe.positionMillis = 60_000
        advanceTimeBy(HEARTBEAT_INTERVAL_MILLIS)
        pumpUntil("the retry to be acknowledged") { controller.lastProgress.value != null }

        // Counters are cumulative, so the dropped beat costs nothing: the beat
        // the server never answered is still recorded, so the retry is the one
        // after it.
        nextRequest()
        val recovered = Json.parseToJsonElement(nextRequest().body.readUtf8()) as JsonObject
        assertEquals("60", recovered["positionSeconds"]?.jsonPrimitive?.content)
        assertTrue(controller.state.value is PlaybackState.Playable)

        controller.detachPlayer()
    }

    @Test
    fun `a revoked token during playback stops the session and drops the grant`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        server.enqueue(json(problemJson(401, "unknown device token"), code = 401))

        val store = FakeGrantStore()
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = this,
            clock = { T0 },
        )
        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)
        controller.attachPlayer(FakeProbe(positionMillis = 10_000))

        advanceTimeBy(HEARTBEAT_INTERVAL_MILLIS)

        val failed = awaitState<PlaybackState.Failed>(controller)
        assertTrue(failed.error is ApiError.Unauthenticated)
        assertFalse(failed.recoverable)
        assertNull(store.stored)
    }

    @Test
    fun `a heartbeat that consumes the play surfaces it without ending playback`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        server.enqueue(json(sessionJson(watchedSeconds = 4_400, maxPositionSeconds = 4_400, playConsumed = true)))

        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )
        controller.start("media-1", MediaClass.ENTERTAINMENT, runtimeSeconds = 5_400)
        controller.attachPlayer(FakeProbe(positionMillis = 4_400_000))

        advanceTimeBy(HEARTBEAT_INTERVAL_MILLIS)
        pumpUntil("the server's watch-once verdict") { controller.playConsumed }

        // The play is spent, but the student keeps watching to the end.
        val state = controller.state.value as PlaybackState.Playable
        assertTrue(state.playConsumed)
        assertTrue(controller.playConsumed)

        controller.detachPlayer()
    }

    @Test
    fun `finish closes the session with the server's watch-once verdict`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        server.enqueue(json(sessionJson(completed = true, playConsumed = true)))

        val store = FakeGrantStore()
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = this,
            clock = { T0 },
        )
        controller.start("media-1", MediaClass.ENTERTAINMENT, runtimeSeconds = 5_400)
        nextRequest()
        controller.attachPlayer(FakeProbe(positionMillis = 5_400_000))

        controller.finish(completed = true)

        assertEquals("/api/v1/grants/33333333-3333-3333-3333-333333333333/complete", nextRequest().path)
        val finished = controller.state.value as PlaybackState.Finished
        assertTrue(finished.playConsumed)
        assertTrue(finished.completed)
        // The session is over, so nothing should be left to resume.
        assertNull(store.stored)
    }

    @Test
    fun `exiting early reports not-completed so the play is not charged`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        server.enqueue(json(sessionJson(completed = false, playConsumed = false)))

        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )
        controller.start("media-1", MediaClass.ENTERTAINMENT, runtimeSeconds = 5_400)
        nextRequest()
        controller.attachPlayer(FakeProbe(positionMillis = 300_000))

        controller.finish(completed = false)

        val sent = Json.parseToJsonElement(nextRequest().body.readUtf8()) as JsonObject
        assertEquals("false", sent["completed"]?.jsonPrimitive?.content)
        assertFalse((controller.state.value as PlaybackState.Finished).playConsumed)
    }

    @Test
    fun `a resumable stored grant is reused instead of spending a new play`() = runTest {
        // Only the completion call is enqueued: reusing the stored grant must not
        // hit the grant endpoint at all.
        server.enqueue(json(sessionJson(completed = true, playConsumed = true)))

        val store = FakeGrantStore(
            StoredGrant(
                grantId = "grant-resume",
                mediaItemId = "media-1",
                streamUrl = "http://jellyfin.local/Videos/jf-1/stream?static=true",
                startOffsetSeconds = 0,
                mode = "on_demand",
                expiresAtEpochSeconds = T0.plusSeconds(120).epochSecond,
                positionSeconds = 900,
                watchedSeconds = 880,
                redeemed = true,
            ),
        )
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.ENTERTAINMENT, runtimeSeconds = 5_400)

        val state = controller.state.value as PlaybackState.Playable
        assertEquals("grant-resume", state.grant.grantId)
        // Resume starts 30s before the furthest mark so the student hears context.
        assertEquals(870, state.resumePositionSeconds)
        assertEquals(900, state.furthestPositionSeconds)

        controller.finish(completed = true)

        // The prior watch time is carried forward, not restarted at zero.
        val sent = Json.parseToJsonElement(nextRequest().body.readUtf8()) as JsonObject
        assertEquals("880", sent["watchedSeconds"]?.jsonPrimitive?.content)
        assertEquals("900", sent["positionSeconds"]?.jsonPrimitive?.content)
    }

    @Test
    fun `notePosition raises the on-demand furthest watermark`() = runTest {
        server.enqueue(json(grantJson(furthestPositionSeconds = 100, startOffsetSeconds = 70), code = 201))

        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )
        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 3_600)

        val before = controller.state.value as PlaybackState.Playable
        assertEquals(100, before.furthestPositionSeconds)
        assertEquals(70, before.resumePositionSeconds)

        controller.notePosition(250_000L)
        val after = controller.state.value as PlaybackState.Playable
        assertEquals(250, after.furthestPositionSeconds)
        assertEquals(250, controller.furthestPositionSeconds())
    }

    @Test
    fun `a redeemed grant stays resumable past its expiry`() = runTest {
        // A feature film outlives the grant TTL, and the server keeps accepting
        // progress for a redeemed grant, so expiry alone must not discard it.
        server.enqueue(json(sessionJson()))

        val store = FakeGrantStore(
            StoredGrant(
                grantId = "grant-long-film",
                mediaItemId = "media-1",
                streamUrl = "http://jellyfin.local/Videos/jf-1/stream?static=true",
                startOffsetSeconds = 0,
                mode = "on_demand",
                expiresAtEpochSeconds = T0.minusSeconds(3_600).epochSecond,
                positionSeconds = 4_000,
                watchedSeconds = 4_000,
                redeemed = true,
            ),
        )
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.MIXED, runtimeSeconds = 7_200)

        assertEquals(
            "grant-long-film",
            (controller.state.value as PlaybackState.Playable).grant.grantId,
        )
    }

    @Test
    fun `an unredeemed expired grant is discarded and a fresh one requested`() = runTest {
        server.enqueue(json(grantJson(grantId = "grant-fresh"), code = 201))

        val store = FakeGrantStore(
            StoredGrant(
                grantId = "grant-stale",
                mediaItemId = "media-1",
                streamUrl = "http://jellyfin.local/old",
                startOffsetSeconds = 0,
                mode = "on_demand",
                expiresAtEpochSeconds = T0.minusSeconds(60).epochSecond,
                positionSeconds = 0,
                watchedSeconds = 0,
                redeemed = false,
            ),
        )
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)

        assertEquals("/api/v1/media/media-1/grant?mode=on_demand", nextRequest().path)
        assertEquals("grant-fresh", (controller.state.value as PlaybackState.Playable).grant.grantId)
    }

    @Test
    fun `a stored grant for a different item is discarded`() = runTest {
        server.enqueue(json(grantJson(grantId = "grant-other"), code = 201))

        val store = FakeGrantStore(
            StoredGrant(
                grantId = "grant-for-media-1",
                mediaItemId = "media-1",
                streamUrl = "http://jellyfin.local/one",
                startOffsetSeconds = 0,
                mode = "on_demand",
                expiresAtEpochSeconds = T0.plusSeconds(300).epochSecond,
                positionSeconds = 100,
                watchedSeconds = 100,
                redeemed = true,
            ),
        )
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = this,
            clock = { T0 },
        )

        controller.start("media-2", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)

        assertEquals("/api/v1/media/media-2/grant?mode=on_demand", nextRequest().path)
        val state = controller.state.value as PlaybackState.Playable
        assertEquals("grant-other", state.grant.grantId)
        // Nothing from the abandoned session leaks into the new one.
        assertEquals(0, state.resumePositionSeconds)
    }

    @Test
    fun `detach flushes progress but keeps the grant so playback can resume`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        server.enqueue(json(sessionJson(watchedSeconds = 0, maxPositionSeconds = 45)))

        val store = FakeGrantStore()
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = store,
            scope = this,
            clock = { T0 },
        )
        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)
        nextRequest()
        controller.attachPlayer(FakeProbe(positionMillis = 45_000))

        controller.detachPlayer()

        assertEquals("/api/v1/grants/33333333-3333-3333-3333-333333333333/heartbeat", nextRequest().path)
        // Backgrounding is not finishing: the grant survives, now marked redeemed.
        val stored = assertNotNull(store.stored).let { store.stored!! }
        assertEquals(45, stored.positionSeconds)
        assertTrue(stored.redeemed)
        assertTrue(controller.state.value is PlaybackState.Playable)
    }

    @Test
    fun `a failed completion still reports a locally predicted verdict`() = runTest {
        server.enqueue(json(grantJson(), code = 201))
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))

        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )
        controller.start("media-1", MediaClass.ENTERTAINMENT, runtimeSeconds = 5_400)
        controller.attachPlayer(FakeProbe(positionMillis = 5_400_000))

        controller.finish(completed = true)

        // The server never answered, so the client mirrors its rule to avoid
        // telling the student the play is still available when it is not.
        val finished = controller.state.value as PlaybackState.Finished
        assertTrue(finished.playConsumed)
    }

    @Test
    fun `finishing without a grant does not crash`() = runTest {
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )

        controller.finish(completed = false)

        assertEquals(PlaybackState.Finished(playConsumed = false, completed = false), controller.state.value)
    }

    @Test
    fun `a grant startable window is judged against the server clock`() = runTest {
        server.enqueue(json(grantJson(expiresAt = "2025-03-01T12:05:00Z"), code = 201))
        val controller = PlaybackSessionController(
            repository = repositoryFor(server),
            grantStore = FakeGrantStore(),
            scope = this,
            clock = { T0 },
        )

        controller.start("media-1", MediaClass.EDUCATIONAL, runtimeSeconds = 1_800)

        val grant = (controller.state.value as PlaybackState.Playable).grant
        assertTrue(grant.startableAt(T0))
        assertFalse(grant.startableAt(Instant.parse("2025-03-01T12:06:00Z")))
    }
}
