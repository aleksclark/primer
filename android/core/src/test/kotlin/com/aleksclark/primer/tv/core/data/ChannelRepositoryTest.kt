package com.aleksclark.primer.tv.core.data

import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.PlaybackMode
import com.aleksclark.primer.tv.core.testing.nowJson
import com.aleksclark.primer.tv.core.testing.problemJson
import com.aleksclark.primer.tv.core.testing.programmeJson
import com.aleksclark.primer.tv.core.testing.programmedGrantJson
import com.aleksclark.primer.tv.core.testing.repositoryFor
import com.aleksclark.primer.tv.core.testing.scheduleJson
import java.time.Instant
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** The channel half of the device API: `/now`, `/schedule`, programmed grants. */
class ChannelRepositoryTest {
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

    @Test
    fun `now maps the airing programme and the server's offset`() = runTest {
        server.enqueue(
            json(
                nowJson(
                    next = programmeJson(
                        scheduleEntryId = "77777777-7777-7777-7777-777777777777",
                        title = "Gravity",
                        airsAt = "2025-03-01T12:20:00Z",
                        endsAt = "2025-03-01T12:50:00Z",
                    ),
                    nextStartsInSeconds = 1_200,
                ),
            ),
        )

        val now = (repositoryFor(server).now() as ApiResult.Ok).value

        assertEquals("/api/v1/now", server.takeRequest().path)
        val onAir = requireNotNull(now.onAir)
        assertEquals("Inertia", onAir.title)
        assertEquals(MediaClass.EDUCATIONAL, onAir.mediaClass)
        assertEquals(listOf("science"), onAir.subjectTags)
        assertEquals(Instant.parse("2025-03-01T11:50:00Z"), onAir.airsAt)
        assertEquals(600, now.offsetSeconds)
        assertEquals(600, now.startOffsetSeconds)
        assertEquals(1_200, now.remainingSeconds)
        assertFalse(now.inGap)
        assertEquals("Gravity", now.next?.title)
        assertEquals(1_200, now.nextStartsInSeconds)
        assertEquals(Instant.parse("2025-03-01T12:00:00Z"), now.serverTime)
    }

    @Test
    fun `now reports a gap as an absent programme rather than a stale one`() = runTest {
        server.enqueue(
            json(
                nowJson(
                    onAir = null,
                    offsetSeconds = 0,
                    startOffsetSeconds = 0,
                    next = programmeJson(title = "Gravity"),
                    nextStartsInSeconds = 900,
                ),
            ),
        )

        val now = (repositoryFor(server).now() as ApiResult.Ok).value

        assertNull(now.onAir)
        assertTrue(now.inGap)
        assertEquals(0, now.remainingSeconds)
        assertEquals("Gravity", now.next?.title)
    }

    @Test
    fun `now on an empty grid has neither programme`() = runTest {
        server.enqueue(json(nowJson(onAir = null, offsetSeconds = 0, startOffsetSeconds = 0)))

        val now = (repositoryFor(server).now() as ApiResult.Ok).value

        assertNull(now.onAir)
        assertNull(now.next)
        assertTrue(now.inGap)
    }

    @Test
    fun `schedule asks the server for its own today and maps the grid`() = runTest {
        server.enqueue(
            json(
                scheduleJson(
                    programmeJson(title = "Inertia"),
                    programmeJson(
                        scheduleEntryId = "77777777-7777-7777-7777-777777777777",
                        title = "Gravity",
                        airsAt = "2025-03-01T12:20:00Z",
                        endsAt = "2025-03-01T12:50:00Z",
                    ),
                ),
            ),
        )

        val day = (repositoryFor(server).schedule() as ApiResult.Ok).value

        // No day parameter: the client does not know the channel's timezone, so
        // only the server can say what "today" is.
        assertEquals("/api/v1/schedule", server.takeRequest().path)
        assertEquals("2025-03-01", day.day)
        assertEquals("America/Chicago", day.timezone)
        assertEquals(listOf("Inertia", "Gravity"), day.programmes.map { it.title })
        assertFalse(day.isEmpty)
        assertEquals("Inertia", day.onAir()?.title)
    }

    @Test
    fun `schedule passes an explicit day through`() = runTest {
        server.enqueue(json(scheduleJson(day = "2025-03-02")))

        val day = (repositoryFor(server).schedule("2025-03-02") as ApiResult.Ok).value

        assertEquals("/api/v1/schedule?day=2025-03-02", server.takeRequest().path)
        assertTrue(day.isEmpty)
        assertNull(day.onAir())
    }

    @Test
    fun `a programmed grant asks for programmed mode and carries the join offset`() = runTest {
        server.enqueue(json(programmedGrantJson(startOffsetSeconds = 420), code = 201))

        val grant = (
            repositoryFor(server).grant("media-1", PlaybackMode.PROGRAMMED) as ApiResult.Ok
            ).value

        assertEquals("/api/v1/media/media-1/grant?mode=programmed", server.takeRequest().path)
        assertEquals(420, grant.startOffsetSeconds)
        assertEquals("programmed", grant.mode)
    }

    @Test
    fun `a refused programmed grant explains that the channel has no catch-up`() = runTest {
        server.enqueue(
            json(problemJson(403, "that programme is not airing now; the channel does not offer catch-up"), code = 403),
        )

        val result = repositoryFor(server).grant("media-1", PlaybackMode.PROGRAMMED)

        val error = (result as ApiResult.Err).error
        assertTrue(error is ApiError.Forbidden)
        assertEquals(
            "that programme is not airing now; the channel does not offer catch-up",
            error.message,
        )
    }

    @Test
    fun `a refusal with no detail still names the mode's own failure`() = runTest {
        server.enqueue(MockResponse().setResponseCode(403))

        val error = (repositoryFor(server).grant("media-1", PlaybackMode.PROGRAMMED) as ApiResult.Err).error

        assertTrue(error.message.contains("not airing"))
        assertTrue(error.message.contains("missed slot") || error.message.contains("caught up"))
    }
}
