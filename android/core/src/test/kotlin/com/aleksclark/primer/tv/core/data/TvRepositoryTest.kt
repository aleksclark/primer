package com.aleksclark.primer.tv.core.data

import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.testing.catalogItemJson
import com.aleksclark.primer.tv.core.testing.catalogJson
import com.aleksclark.primer.tv.core.testing.grantJson
import com.aleksclark.primer.tv.core.testing.problemJson
import com.aleksclark.primer.tv.core.testing.repositoryFor
import com.aleksclark.primer.tv.core.testing.sessionJson
import java.time.Instant
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
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

class TvRepositoryTest {
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
    fun `pair posts the code to the device pairing path and maps the token`() = runTest {
        server.enqueue(
            json(
                """{"token":"tok-abc","device":{"id":"dev-1","name":"Den TV","kind":"tv_box","pairingCode":"","createdAt":"2025-03-01T12:00:00Z","updatedAt":"2025-03-01T12:00:00Z"}}""",
                code = 201,
            ),
        )

        val result = repositoryFor(server, token = null).pair(" 4821 ")

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/devices/pair", request.path)
        // The code is trimmed so a stray space from a D-pad keyboard does not
        // fail an otherwise valid pairing.
        val sent = Json.parseToJsonElement(request.body.readUtf8()) as JsonObject
        assertEquals("4821", sent["code"]?.jsonPrimitive?.content)

        val pairing = (result as ApiResult.Ok).value
        assertEquals("tok-abc", pairing.token)
        assertEquals("Den TV", pairing.device.name)
        assertEquals("tv_box", pairing.device.kind)
    }

    @Test
    fun `pair maps a rejected code to a forbidden error carrying the server detail`() = runTest {
        server.enqueue(
            json(problemJson(403, "pairing code is invalid, expired, or already used"), code = 403),
        )

        val result = repositoryFor(server, token = null).pair("0000")

        val error = (result as ApiResult.Err).error
        assertTrue(error is ApiError.Forbidden)
        assertEquals("pairing code is invalid, expired, or already used", error.message)
    }

    @Test
    fun `catalog sends the device bearer token and maps items into domain entries`() = runTest {
        server.enqueue(json(catalogJson(catalogItemJson(mediaClass = "entertainment", runtimeSeconds = 5_400))))

        val result = repositoryFor(server).catalog()

        val request = server.takeRequest()
        assertEquals("/api/v1/catalog", request.path)
        assertEquals("Bearer device-token", request.getHeader("Authorization"))

        val catalog = (result as ApiResult.Ok).value
        assertEquals(Instant.parse("2025-03-01T12:00:00Z"), catalog.serverTime)
        val entry = catalog.entries.single()
        assertEquals("Inertia", entry.title)
        assertEquals(MediaClass.ENTERTAINMENT, entry.mediaClass)
        assertEquals(5_400, entry.runtimeSeconds)
        assertEquals(listOf("science"), entry.subjectTags)
        assertEquals("/images/11111111-1111-1111-1111-111111111111/Primary", entry.imagePath)
        assertEquals(Instant.parse("2025-03-08T12:00:00Z"), entry.windowEndsAt)
        assertTrue(entry.directPlayOk)
    }

    @Test
    fun `catalog maps a null items array to an empty catalog`() = runTest {
        // Go marshals an empty slice as null, so the client must not treat a
        // missing array as a parse failure.
        server.enqueue(json("""{"items":null,"serverTime":"2025-03-01T12:00:00Z"}"""))

        val catalog = (repositoryFor(server).catalog() as ApiResult.Ok).value

        assertTrue(catalog.entries.isEmpty())
    }

    @Test
    fun `an unrecognised media class degrades to the rationed default`() = runTest {
        server.enqueue(json(catalogJson(catalogItemJson(mediaClass = "documentary"))))

        val catalog = (repositoryFor(server).catalog() as ApiResult.Ok).value

        assertEquals(MediaClass.UNKNOWN, catalog.entries.single().mediaClass)
        assertTrue(catalog.entries.single().mediaClass.consumesPlay)
    }

    @Test
    fun `catalog maps a rejected token to unauthenticated`() = runTest {
        server.enqueue(json(problemJson(401, "unknown device token", title = "Unauthorized"), code = 401))

        val error = (repositoryFor(server).catalog() as ApiResult.Err).error

        assertTrue(error is ApiError.Unauthenticated)
        assertEquals("unknown device token", error.message)
    }

    @Test
    fun `grant maps the stream url and offset`() = runTest {
        server.enqueue(json(grantJson(startOffsetSeconds = 0), code = 201))

        val grant = (repositoryFor(server).grant("media-1") as ApiResult.Ok).value

        assertEquals("/api/v1/media/media-1/grant?mode=on_demand", server.takeRequest().path)
        assertEquals("33333333-3333-3333-3333-333333333333", grant.grantId)
        assertEquals("media-1", grant.mediaItemId)
        assertTrue(grant.streamUrl.contains("static=true"))
        assertEquals("on_demand", grant.mode)
        assertEquals(Instant.parse("2025-03-01T12:05:00Z"), grant.expiresAt)
    }

    @Test
    fun `grant maps a consumed item to forbidden with the server explanation`() = runTest {
        server.enqueue(
            json(problemJson(403, "item is unavailable or its play was already consumed"), code = 403),
        )

        val error = (repositoryFor(server).grant("media-1") as ApiResult.Err).error

        assertTrue(error is ApiError.Forbidden)
        assertEquals("item is unavailable or its play was already consumed", error.message)
    }

    @Test
    fun `grant falls back to its own wording when the server sends no problem body`() = runTest {
        server.enqueue(MockResponse().setResponseCode(403))

        val error = (repositoryFor(server).grant("media-1") as ApiResult.Err).error

        assertTrue(error is ApiError.Forbidden)
        assertTrue(error.message.contains("not available"))
    }

    @Test
    fun `grant maps an unconfigured media source to unavailable`() = runTest {
        server.enqueue(json(problemJson(503, "media source is not configured"), code = 503))

        val error = (repositoryFor(server).grant("media-1") as ApiResult.Err).error

        assertTrue(error is ApiError.Unavailable)
    }

    @Test
    fun `heartbeat posts position and watched seconds and maps the session`() = runTest {
        server.enqueue(json(sessionJson(watchedSeconds = 120, maxPositionSeconds = 150)))

        val progress = (
            repositoryFor(server).heartbeat("grant-1", positionSeconds = 150, watchedSeconds = 120)
                as ApiResult.Ok
            ).value

        val request = server.takeRequest()
        assertEquals("/api/v1/grants/grant-1/heartbeat", request.path)
        val sent = Json.parseToJsonElement(request.body.readUtf8()) as JsonObject
        assertEquals("150", sent["positionSeconds"]?.jsonPrimitive?.content)
        assertEquals("120", sent["watchedSeconds"]?.jsonPrimitive?.content)

        assertEquals(120, progress.watchedSeconds)
        assertEquals(150, progress.maxPositionSeconds)
        assertFalse(progress.playConsumed)
    }

    @Test
    fun `heartbeat clamps negative counters the server would reject`() = runTest {
        // Huma validates minimum 0, so a player reporting a negative position
        // during a seek must not turn into a 422.
        server.enqueue(json(sessionJson()))

        repositoryFor(server).heartbeat("grant-1", positionSeconds = -5, watchedSeconds = -9)

        val sent = Json.parseToJsonElement(server.takeRequest().body.readUtf8()) as JsonObject
        assertEquals("0", sent["positionSeconds"]?.jsonPrimitive?.content)
        assertEquals("0", sent["watchedSeconds"]?.jsonPrimitive?.content)
    }

    @Test
    fun `heartbeat maps an expired unredeemed grant to forbidden`() = runTest {
        server.enqueue(json(problemJson(403, "grant expired before playback started"), code = 403))

        val error = (
            repositoryFor(server).heartbeat("grant-1", positionSeconds = 1, watchedSeconds = 1)
                as ApiResult.Err
            ).error

        assertTrue(error is ApiError.Forbidden)
        assertEquals("grant expired before playback started", error.message)
    }

    @Test
    fun `heartbeat maps an unknown grant to not found`() = runTest {
        server.enqueue(json(problemJson(404, "grant not found", title = "Not Found"), code = 404))

        val error = (
            repositoryFor(server).heartbeat("nope", positionSeconds = 1, watchedSeconds = 1)
                as ApiResult.Err
            ).error

        assertTrue(error is ApiError.NotFound)
    }

    @Test
    fun `complete reports the completion flag and surfaces the consumed play`() = runTest {
        server.enqueue(json(sessionJson(completed = true, playConsumed = true)))

        val progress = (
            repositoryFor(server).complete("grant-1", positionSeconds = 5_400, watchedSeconds = 5_400, completed = true)
                as ApiResult.Ok
            ).value

        val request = server.takeRequest()
        assertEquals("/api/v1/grants/grant-1/complete", request.path)
        val sent = Json.parseToJsonElement(request.body.readUtf8()) as JsonObject
        assertEquals("true", sent["completed"]?.jsonPrimitive?.content)

        assertTrue(progress.completed)
        assertTrue(progress.playConsumed)
    }

    @Test
    fun `a dropped connection maps to a network error rather than throwing`() = runTest {
        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))

        val result = repositoryFor(server).catalog()

        assertTrue((result as ApiResult.Err).error is ApiError.Network)
    }

    @Test
    fun `a malformed body maps to an unexpected error rather than throwing`() = runTest {
        server.enqueue(json("{ this is not json"))

        val result = repositoryFor(server).catalog()

        assertTrue((result as ApiResult.Err).error is ApiError.Unexpected)
    }

    @Test
    fun `a server error maps to unexpected and keeps the status`() = runTest {
        server.enqueue(json(problemJson(500, "issue device token"), code = 500))

        val error = (repositoryFor(server).catalog() as ApiResult.Err).error

        assertEquals(500, (error as ApiError.Unexpected).status)
    }

    @Test
    fun `no authorization header is sent before pairing`() = runTest {
        server.enqueue(json("""{"items":[],"serverTime":"2025-03-01T12:00:00Z"}"""))

        repositoryFor(server, token = null).catalog()

        assertNull(server.takeRequest().getHeader("Authorization"))
    }
}
