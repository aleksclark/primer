package com.aleksclark.primer.tv.core.testing

import com.aleksclark.primer.tv.core.data.DeviceSettings
import com.aleksclark.primer.tv.core.data.GrantStore
import com.aleksclark.primer.tv.core.data.SettingsStore
import com.aleksclark.primer.tv.core.data.StoredGrant
import com.aleksclark.primer.tv.core.data.TvRepository
import com.aleksclark.primer.tv.core.domain.Catalog
import com.aleksclark.primer.tv.core.domain.CatalogEntry
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.net.buildTvApi
import java.time.Instant
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockWebServer

/** A fixed instant so time-dependent assertions stay deterministic. */
val T0: Instant = Instant.parse("2025-03-01T12:00:00Z")

/** Builds a repository pointed at a [MockWebServer], mirroring production wiring. */
fun repositoryFor(server: MockWebServer, token: String? = "device-token"): TvRepository {
    val client = OkHttpClient.Builder()
        .addInterceptor(com.aleksclark.primer.tv.core.net.DeviceTokenInterceptor { token })
        .build()
    return TvRepository(buildTvApi(server.url("/").toString(), client))
}

/** A JSON catalog item body. Runtime and class are what the tests vary. */
fun catalogItemJson(
    id: String = "11111111-1111-1111-1111-111111111111",
    title: String = "Inertia",
    mediaClass: String = "educational",
    runtimeSeconds: Int = 1_800,
    directPlayOk: Boolean = true,
    windowEndsAt: String = "2025-03-08T12:00:00Z",
    subjectTags: String = """["science"]""",
): String = """
{
  "mediaItem": {
    "id": "$id",
    "jellyfinItemId": "jf-$id",
    "title": "$title",
    "sortTitle": "$title",
    "overview": "A film about $title.",
    "class": "$mediaClass",
    "runtimeSeconds": $runtimeSeconds,
    "subjectTags": $subjectTags,
    "standardCodes": ["TN.SCI.6.PS2.1"],
    "qualityNotes": "",
    "container": "mkv",
    "videoCodec": "h264",
    "audioCodec": "aac",
    "directPlayOk": $directPlayOk,
    "imageTag": "tag-1",
    "createdAt": "2025-02-01T00:00:00Z",
    "updatedAt": "2025-02-01T00:00:00Z"
  },
  "availabilityWindowId": "22222222-2222-2222-2222-222222222222",
  "windowEndsAt": "$windowEndsAt",
  "imageUrl": "/images/$id/Primary"
}
""".trimIndent()

/** Wraps catalog items in the response envelope. */
fun catalogJson(vararg items: String, serverTime: String = "2025-03-01T12:00:00Z"): String =
    """{"items":[${items.joinToString(",")}],"serverTime":"$serverTime"}"""

/** A grant response body. */
fun grantJson(
    grantId: String = "33333333-3333-3333-3333-333333333333",
    streamUrl: String = "http://jellyfin.local/Videos/jf-1/stream?static=true&api_key=k",
    startOffsetSeconds: Int = 0,
    expiresAt: String = "2025-03-01T12:05:00Z",
): String = """
{
  "grantId": "$grantId",
  "streamUrl": "$streamUrl",
  "startOffsetSeconds": $startOffsetSeconds,
  "mode": "on_demand",
  "expiresAt": "$expiresAt",
  "serverTime": "2025-03-01T12:00:00Z"
}
""".trimIndent()

/** A programme body, as `/now` and `/schedule` embed it. */
fun programmeJson(
    scheduleEntryId: String = "66666666-6666-6666-6666-666666666666",
    mediaItemId: String = "11111111-1111-1111-1111-111111111111",
    title: String = "Inertia",
    mediaClass: String = "educational",
    runtimeSeconds: Int = 1_800,
    airsAt: String = "2025-03-01T11:50:00Z",
    endsAt: String = "2025-03-01T12:20:00Z",
    joinInProgress: Boolean = true,
    directPlayOk: Boolean = true,
): String = """
{
  "scheduleEntryId": "$scheduleEntryId",
  "mediaItemId": "$mediaItemId",
  "title": "$title",
  "overview": "A film about $title.",
  "class": "$mediaClass",
  "subjectTags": ["science"],
  "runtimeSeconds": $runtimeSeconds,
  "airsAt": "$airsAt",
  "endsAt": "$endsAt",
  "block": "morning",
  "joinInProgress": $joinInProgress,
  "directPlayOk": $directPlayOk,
  "imageUrl": "/images/$mediaItemId/Primary"
}
""".trimIndent()

/**
 * A `/now` body. [onAir] and [next] are raw programme JSON, or null for the gap
 * and end-of-grid cases the server reports by omitting them.
 */
fun nowJson(
    onAir: String? = programmeJson(),
    offsetSeconds: Int = 600,
    startOffsetSeconds: Int = 600,
    next: String? = null,
    nextStartsInSeconds: Int = 0,
    serverTime: String = "2025-03-01T12:00:00Z",
): String {
    val fields = buildList {
        if (onAir != null) add(field("onAir", onAir))
        if (next != null) add(field("next", next))
        add(field("offsetSeconds", offsetSeconds.toString()))
        add(field("startOffsetSeconds", startOffsetSeconds.toString()))
        add(field("nextStartsInSeconds", nextStartsInSeconds.toString()))
        add(field("serverTime", quote(serverTime)))
    }
    return fields.joinToString(",", prefix = "{", postfix = "}")
}

/** field renders one JSON member whose value is already JSON. */
private fun field(name: String, rawValue: String): String = "\"$name\":$rawValue"

/** quote wraps a value as a JSON string. */
private fun quote(value: String): String = "\"$value\""

/** A `/schedule` body. */
fun scheduleJson(
    vararg programmes: String,
    day: String = "2025-03-01",
    timezone: String = "America/Chicago",
    serverTime: String = "2025-03-01T12:00:00Z",
): String = """
{
  "day": "$day",
  "timezone": "$timezone",
  "dayStartsAt": "${day}T06:00:00Z",
  "dayEndsAt": "${day}T06:00:00Z",
  "programmes": [${programmes.joinToString(",")}],
  "serverTime": "$serverTime"
}
""".trimIndent()

/** A grant response body for the programmed channel. */
fun programmedGrantJson(
    grantId: String = "33333333-3333-3333-3333-333333333333",
    startOffsetSeconds: Int = 600,
    expiresAt: String = "2025-03-01T12:05:00Z",
): String = """
{
  "grantId": "$grantId",
  "streamUrl": "http://jellyfin.local/Videos/jf-1/stream?static=true&api_key=k",
  "startOffsetSeconds": $startOffsetSeconds,
  "mode": "programmed",
  "expiresAt": "$expiresAt",
  "serverTime": "2025-03-01T12:00:00Z"
}
""".trimIndent()

/** A session response body for heartbeat and complete. */
fun sessionJson(
    watchedSeconds: Int = 30,
    maxPositionSeconds: Int = 30,
    completed: Boolean = false,
    playConsumed: Boolean = false,
): String = """
{
  "session": {
    "id": "44444444-4444-4444-4444-444444444444",
    "grantId": "33333333-3333-3333-3333-333333333333",
    "mediaItemId": "11111111-1111-1111-1111-111111111111",
    "deviceId": "55555555-5555-5555-5555-555555555555",
    "startedAt": "2025-03-01T12:00:00Z",
    "watchedSeconds": $watchedSeconds,
    "maxPositionSeconds": $maxPositionSeconds,
    "completed": $completed,
    "createdAt": "2025-03-01T12:00:00Z",
    "updatedAt": "2025-03-01T12:00:30Z"
  },
  "playConsumed": $playConsumed,
  "serverTime": "2025-03-01T12:00:30Z"
}
""".trimIndent()

/** An RFC 7807 problem body as Huma emits it. */
fun problemJson(status: Int, detail: String, title: String = "Error"): String =
    """{"status":$status,"title":"$title","detail":"$detail"}"""

/** A domain catalog entry, for tests that skip the wire format. */
fun entry(
    id: String,
    title: String = id,
    mediaClass: MediaClass = MediaClass.EDUCATIONAL,
    runtimeSeconds: Int = 1_800,
    windowEndsAt: Instant = T0.plusSeconds(7 * 24 * 3600),
): CatalogEntry = CatalogEntry(
    mediaItemId = id,
    title = title,
    overview = "",
    mediaClass = mediaClass,
    runtimeSeconds = runtimeSeconds,
    subjectTags = emptyList(),
    standardCodes = emptyList(),
    availabilityWindowId = "window-$id",
    windowEndsAt = windowEndsAt,
    imagePath = "/images/$id/Primary",
    directPlayOk = true,
    container = "mkv",
    videoCodec = "h264",
    audioCodec = "aac",
)

/** A catalog of domain entries stamped at [T0]. */
fun catalog(vararg entries: CatalogEntry, serverTime: Instant = T0) =
    Catalog(entries = entries.toList(), serverTime = serverTime)

/** In-memory [GrantStore]. */
class FakeGrantStore(initial: StoredGrant? = null) : GrantStore {
    var stored: StoredGrant? = initial
        private set
    var clearCount: Int = 0
        private set

    override suspend fun load(): StoredGrant? = stored

    override suspend fun save(grant: StoredGrant) {
        stored = grant
    }

    override suspend fun clear() {
        stored = null
        clearCount++
    }
}

/** In-memory [SettingsStore]. */
class FakeSettingsStore(initial: DeviceSettings = DeviceSettings()) : SettingsStore {
    private val state = MutableStateFlow(initial)

    override val settings: Flow<DeviceSettings> = state

    override suspend fun current(): DeviceSettings = state.first()

    override suspend fun setBaseUrl(baseUrl: String) {
        state.value = state.value.copy(baseUrl = baseUrl)
    }

    override suspend fun savePairing(token: String, deviceId: String, deviceName: String, deviceKind: String) {
        state.value = state.value.copy(
            token = token,
            deviceId = deviceId,
            deviceName = deviceName,
            deviceKind = deviceKind,
        )
    }

    override suspend fun clearPairing() {
        state.value = state.value.copy(token = null, deviceId = null, deviceName = null, deviceKind = null)
    }
}
