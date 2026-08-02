package com.aleksclark.primer.tv.core.data

import com.aleksclark.primer.tv.core.domain.AppRelease
import com.aleksclark.primer.tv.core.domain.Catalog
import com.aleksclark.primer.tv.core.domain.CatalogEntry
import com.aleksclark.primer.tv.core.domain.ChannelDay
import com.aleksclark.primer.tv.core.domain.ChannelNow
import com.aleksclark.primer.tv.core.domain.Device
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.Pairing
import com.aleksclark.primer.tv.core.domain.PlayGrant
import com.aleksclark.primer.tv.core.domain.PlaybackMode
import com.aleksclark.primer.tv.core.domain.Programme
import com.aleksclark.primer.tv.core.domain.SessionProgress
import com.aleksclark.primer.tv.core.net.CatalogItemDto
import com.aleksclark.primer.tv.core.net.CompleteRequestDto
import com.aleksclark.primer.tv.core.net.GrantResponseDto
import com.aleksclark.primer.tv.core.net.HeartbeatRequestDto
import com.aleksclark.primer.tv.core.net.PairRequestDto
import com.aleksclark.primer.tv.core.net.ProblemDto
import com.aleksclark.primer.tv.core.net.ProgrammeDto
import com.aleksclark.primer.tv.core.net.SessionResponseDto
import com.aleksclark.primer.tv.core.net.TvApi
import com.aleksclark.primer.tv.core.net.tvJson
import java.io.IOException
import retrofit2.Response

/**
 * The device-facing half of the TV server, expressed in domain terms.
 *
 * All failures arrive as [ApiResult.Err]; nothing here throws for an expected
 * condition, because "your one play is used up" is a normal outcome the UI has
 * to render, not an exception.
 */
class TvRepository(private val api: TvApi) {

    /** Exchanges a pairing code for a device token. */
    suspend fun pair(code: String): ApiResult<Pairing> = call(
        request = { api.pair(PairRequestDto(code = code.trim())) },
        forbidden = "That pairing code is invalid, expired, or already used. Generate a new one in the admin UI.",
        map = { body ->
            Pairing(
                token = body.token,
                device = Device(id = body.device.id, name = body.device.name, kind = body.device.kind),
            )
        },
    )

    /**
     * Lists what may be played right now.
     *
     * The server's catalog query (`repo.Catalog`) filters out items whose play
     * has already been consumed and items that cannot direct-play, so a consumed
     * entertainment item simply stops appearing. The client therefore never
     * fabricates an "already watched" row from an empty response — it can only
     * mark an item watched within this session, from the server's own
     * `playConsumed` flag.
     */
    suspend fun catalog(): ApiResult<Catalog> = call(
        request = { api.catalog() },
        map = { body ->
            Catalog(
                entries = body.items.orEmpty().map(::toEntry),
                serverTime = body.serverTime,
            )
        },
    )

    /**
     * Resolves what the channel is airing, and how far into it.
     *
     * The offset comes from the server's clock, not the box's: a cheap RTC
     * drifts, and the channel is only a channel if every device agrees on where
     * in the programme it is.
     */
    suspend fun now(): ApiResult<ChannelNow> = call(
        request = { api.now() },
        map = { body ->
            ChannelNow(
                onAir = body.onAir?.let(::toProgramme),
                offsetSeconds = body.offsetSeconds,
                startOffsetSeconds = body.startOffsetSeconds,
                next = body.next?.let(::toProgramme),
                nextStartsInSeconds = body.nextStartsInSeconds,
                serverTime = body.serverTime,
            )
        },
    )

    /**
     * Loads one day of the grid.
     *
     * @param day calendar day as `YYYY-MM-DD` in the channel's timezone, or null
     *   for the server's today. The client does not know the channel's zone, so
     *   null is the only honest way to ask for "today".
     */
    suspend fun schedule(day: String? = null): ApiResult<ChannelDay> = call(
        request = { api.schedule(day) },
        map = { body ->
            ChannelDay(
                day = body.day,
                timezone = body.timezone,
                programmes = body.programmes.orEmpty().map(::toProgramme),
                serverTime = body.serverTime,
            )
        },
    )

    /**
     * Requests a single-use playback authorization.
     *
     * In [PlaybackMode.PROGRAMMED] the server refuses anything that is not
     * airing at this instant — the channel has no catch-up — and stamps the
     * grant with the offset to join at.
     */
    suspend fun grant(
        mediaItemId: String,
        mode: PlaybackMode = PlaybackMode.ON_DEMAND,
    ): ApiResult<PlayGrant> = call(
        request = { api.grant(mediaItemId, mode.wire) },
        forbidden = when (mode) {
            PlaybackMode.PROGRAMMED ->
                "That programme is not airing right now. The channel plays live, so a missed slot cannot be caught up."
            PlaybackMode.ON_DEMAND ->
                "This item is not available right now. Its viewing may already be used up, or its availability window has closed."
        },
        map = { body -> toGrant(mediaItemId, body) },
    )

    /** Reports progress, redeeming the grant on the first call. */
    suspend fun heartbeat(grantId: String, positionSeconds: Int, watchedSeconds: Int): ApiResult<SessionProgress> = call(
        request = {
            api.heartbeat(
                grantId,
                HeartbeatRequestDto(
                    positionSeconds = positionSeconds.coerceAtLeast(0),
                    watchedSeconds = watchedSeconds.coerceAtLeast(0),
                ),
            )
        },
        forbidden = "This viewing authorization expired before playback started.",
        map = ::toProgress,
    )

    /**
     * Reads the APK the server publishes for sideloading.
     *
     * A server that has never had a build uploaded answers "nothing
     * available", which is a normal state rather than a failure: the box then
     * simply keeps running what it has.
     */
    suspend fun appRelease(): ApiResult<AppRelease> = call(
        request = { api.appRelease() },
        map = { body ->
            AppRelease(
                available = body.available,
                versionCode = body.versionCode,
                sizeBytes = body.sizeBytes,
                sha256 = body.sha256,
                downloadPath = body.downloadUrl,
            )
        },
    )

    /** Closes the session out. */
    suspend fun complete(
        grantId: String,
        positionSeconds: Int,
        watchedSeconds: Int,
        completed: Boolean,
    ): ApiResult<SessionProgress> = call(
        request = {
            api.complete(
                grantId,
                CompleteRequestDto(
                    positionSeconds = positionSeconds.coerceAtLeast(0),
                    watchedSeconds = watchedSeconds.coerceAtLeast(0),
                    completed = completed,
                ),
            )
        },
        forbidden = "This viewing authorization expired before playback started.",
        map = ::toProgress,
    )

    /**
     * Runs one call, turning transport faults and error statuses into
     * [ApiError]. A 403 means something different on every endpoint, so callers
     * supply the wording.
     */
    private suspend fun <D, T> call(
        request: suspend () -> Response<D>,
        forbidden: String = "The server refused this request.",
        map: (D) -> T,
    ): ApiResult<T> {
        val response = try {
            request()
        } catch (e: IOException) {
            return ApiResult.Err(ApiError.Network(e.message?.takeIf { it.isNotBlank() } ?: "Cannot reach the server."))
        } catch (e: RuntimeException) {
            // Retrofit wraps a malformed body in a RuntimeException; treat it as
            // an unexpected server response rather than crashing the player.
            return ApiResult.Err(ApiError.Unexpected(e.message ?: "Unreadable server response."))
        }

        if (response.isSuccessful) {
            val body = response.body()
                ?: return ApiResult.Err(ApiError.Unexpected("The server returned an empty response.", response.code()))
            return ApiResult.Ok(map(body))
        }

        val detail = problemDetail(response)
        return ApiResult.Err(
            when (response.code()) {
                401 -> ApiError.Unauthenticated(detail ?: "This device is no longer paired with the server.")
                403 -> ApiError.Forbidden(detail ?: forbidden)
                404 -> ApiError.NotFound(detail ?: "The server has no record of this item.")
                503 -> ApiError.Unavailable(detail ?: "The media source is unavailable.")
                else -> ApiError.Unexpected(detail ?: "The server returned an error.", response.code())
            },
        )
    }

    /** Pulls Huma's RFC 7807 `detail` out of an error body, if it has one. */
    private fun problemDetail(response: Response<*>): String? {
        val raw = runCatching { response.errorBody()?.string() }.getOrNull()
        if (raw.isNullOrBlank()) return null
        val problem = runCatching { tvJson.decodeFromString<ProblemDto>(raw) }.getOrNull() ?: return null
        return problem.detail?.takeIf { it.isNotBlank() } ?: problem.title?.takeIf { it.isNotBlank() }
    }

    private companion object {
        fun toEntry(dto: CatalogItemDto): CatalogEntry {
            val item = dto.mediaItem
            return CatalogEntry(
                mediaItemId = item.id,
                title = item.title,
                overview = item.overview,
                mediaClass = MediaClass.fromWire(item.mediaClass),
                runtimeSeconds = item.runtimeSeconds,
                subjectTags = item.subjectTags.orEmpty(),
                standardCodes = item.standardCodes.orEmpty(),
                availabilityWindowId = dto.availabilityWindowId,
                windowEndsAt = dto.windowEndsAt,
                imagePath = dto.imageUrl,
                directPlayOk = item.directPlayOk,
                container = item.container,
                videoCodec = item.videoCodec,
                audioCodec = item.audioCodec,
            )
        }

        fun toProgramme(dto: ProgrammeDto) = Programme(
            scheduleEntryId = dto.scheduleEntryId,
            mediaItemId = dto.mediaItemId,
            title = dto.title,
            overview = dto.overview,
            mediaClass = MediaClass.fromWire(dto.mediaClass),
            subjectTags = dto.subjectTags.orEmpty(),
            runtimeSeconds = dto.runtimeSeconds,
            airsAt = dto.airsAt,
            endsAt = dto.endsAt,
            block = dto.block,
            joinInProgress = dto.joinInProgress,
            directPlayOk = dto.directPlayOk,
            imagePath = dto.imageUrl,
        )

        fun toGrant(mediaItemId: String, dto: GrantResponseDto) = PlayGrant(
            grantId = dto.grantId,
            mediaItemId = mediaItemId,
            streamUrl = dto.streamUrl,
            startOffsetSeconds = dto.startOffsetSeconds,
            mode = dto.mode,
            expiresAt = dto.expiresAt,
            serverTime = dto.serverTime,
        )

        fun toProgress(dto: SessionResponseDto) = SessionProgress(
            sessionId = dto.session.id,
            watchedSeconds = dto.session.watchedSeconds,
            maxPositionSeconds = dto.session.maxPositionSeconds,
            completed = dto.session.completed,
            playConsumed = dto.playConsumed,
            serverTime = dto.serverTime,
        )
    }
}
