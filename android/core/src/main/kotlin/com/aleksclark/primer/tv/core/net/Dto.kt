package com.aleksclark.primer.tv.core.net

import java.time.Instant
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder

/**
 * Reads and writes the RFC 3339 timestamps Go's `time.Time` produces. The
 * offset-aware parse accepts both the `Z` the TV server always sends and a
 * numeric offset, so a reverse proxy that rewrites timestamps cannot break the
 * client.
 */
object InstantSerializer : KSerializer<Instant> {
    override val descriptor: SerialDescriptor =
        PrimitiveSerialDescriptor("java.time.Instant", PrimitiveKind.STRING)

    override fun deserialize(decoder: Decoder): Instant =
        OffsetDateTime.parse(decoder.decodeString(), DateTimeFormatter.ISO_OFFSET_DATE_TIME).toInstant()

    override fun serialize(encoder: Encoder, value: Instant) {
        encoder.encodeString(DateTimeFormatter.ISO_INSTANT.format(value))
    }
}

private typealias ApiInstant = @Serializable(with = InstantSerializer::class) Instant

/** Body of `POST /devices/pair`. */
@Serializable
data class PairRequestDto(val code: String)

/** Response of `POST /devices/pair`. The token is returned exactly once. */
@Serializable
data class PairResponseDto(
    val token: String,
    val device: DeviceDto,
)

/** A paired client as the TV server describes it. */
@Serializable
data class DeviceDto(
    val id: String,
    val name: String,
    val kind: String,
    val pairingCode: String = "",
    val pairedAt: ApiInstant? = null,
    val lastSeenAt: ApiInstant? = null,
    val revokedAt: ApiInstant? = null,
    val createdAt: ApiInstant,
    val updatedAt: ApiInstant,
)

/** A curated Jellyfin entry. */
@Serializable
data class MediaItemDto(
    val id: String,
    val jellyfinItemId: String,
    val title: String,
    val sortTitle: String = "",
    val overview: String = "",
    @SerialName("class") val mediaClass: String,
    val runtimeSeconds: Int,
    val subjectTags: List<String>? = null,
    val standardCodes: List<String>? = null,
    val qualityNotes: String = "",
    val container: String = "",
    val videoCodec: String = "",
    val audioCodec: String = "",
    val directPlayOk: Boolean,
    val imageTag: String = "",
    val orphanedAt: ApiInstant? = null,
    val createdAt: ApiInstant,
    val updatedAt: ApiInstant,
)

/** One playable entry in `GET /catalog`. */
@Serializable
data class CatalogItemDto(
    val mediaItem: MediaItemDto,
    val availabilityWindowId: String,
    val windowEndsAt: ApiInstant,
    val imageUrl: String,
)

/** Response of `GET /catalog`. */
@Serializable
data class CatalogResponseDto(
    val items: List<CatalogItemDto>? = null,
    val serverTime: ApiInstant,
)

/** One airing of the programmed channel, from `GET /now` and `GET /schedule`. */
@Serializable
data class ProgrammeDto(
    val scheduleEntryId: String,
    val mediaItemId: String,
    val title: String,
    val overview: String = "",
    @SerialName("class") val mediaClass: String,
    val subjectTags: List<String>? = null,
    val runtimeSeconds: Int,
    val airsAt: ApiInstant,
    val endsAt: ApiInstant,
    val block: String = "",
    val joinInProgress: Boolean,
    val directPlayOk: Boolean,
    val imageUrl: String = "",
)

/**
 * Response of `GET /now`. Both programme fields are optional and independently
 * so: an empty grid has neither, a gap before the first slot has only [next].
 */
@Serializable
data class NowResponseDto(
    val onAir: ProgrammeDto? = null,
    val offsetSeconds: Int,
    val startOffsetSeconds: Int,
    val next: ProgrammeDto? = null,
    val nextStartsInSeconds: Int,
    val serverTime: ApiInstant,
)

/** Response of `GET /schedule`. */
@Serializable
data class ScheduleResponseDto(
    val day: String,
    val timezone: String,
    val dayStartsAt: ApiInstant,
    val dayEndsAt: ApiInstant,
    val programmes: List<ProgrammeDto>? = null,
    val serverTime: ApiInstant,
)

/** Response of `POST /media/{id}/grant`. */
@Serializable
data class GrantResponseDto(
    val grantId: String,
    val streamUrl: String,
    val startOffsetSeconds: Int,
    val mode: String,
    val expiresAt: ApiInstant,
    val serverTime: ApiInstant,
)

/** Body of `POST /grants/{id}/heartbeat`. */
@Serializable
data class HeartbeatRequestDto(
    val positionSeconds: Int,
    val watchedSeconds: Int,
)

/** Body of `POST /grants/{id}/complete`. */
@Serializable
data class CompleteRequestDto(
    val positionSeconds: Int,
    val watchedSeconds: Int,
    val completed: Boolean? = null,
)

/** Accumulated watch metrics for one redeemed grant. */
@Serializable
data class PlaybackSessionDto(
    val id: String,
    val grantId: String,
    val mediaItemId: String,
    val deviceId: String,
    val startedAt: ApiInstant,
    val endedAt: ApiInstant? = null,
    val watchedSeconds: Int,
    val maxPositionSeconds: Int,
    val completed: Boolean,
    val createdAt: ApiInstant,
    val updatedAt: ApiInstant,
)

/** Response of the heartbeat and complete operations. */
@Serializable
data class SessionResponseDto(
    val session: PlaybackSessionDto,
    val playConsumed: Boolean,
    val serverTime: ApiInstant,
)

/** One field-level problem inside [ProblemDto]. */
@Serializable
data class ProblemDetailDto(
    val location: String? = null,
    val message: String? = null,
)

/**
 * RFC 7807 body Huma returns for every error status. Only the fields worth
 * showing a student are modelled.
 */
@Serializable
data class ProblemDto(
    val status: Int? = null,
    val title: String? = null,
    val detail: String? = null,
    val errors: List<ProblemDetailDto>? = null,
)

/** Response of `GET /app/release`. */
@Serializable
data class AppReleaseDto(
    val available: Boolean = false,
    val versionCode: Int = 0,
    val sizeBytes: Long = 0,
    val sha256: String = "",
    val downloadUrl: String = "",
)
