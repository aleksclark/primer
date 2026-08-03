package com.aleksclark.primer.tv.core.domain

import java.time.Instant

/**
 * How a media item is classified, which decides both what the player allows
 * and whether finishing it burns an availability window.
 *
 * [UNKNOWN] exists because the server's enum may grow: an unrecognised class is
 * treated as the most restrictive known option rather than crashing the client.
 */
enum class MediaClass(val wire: String) {
    EDUCATIONAL("educational"),
    ENTERTAINMENT("entertainment"),
    MIXED("mixed"),
    UNKNOWN("");

    /** Whether finishing this item consumes its availability window. */
    val consumesPlay: Boolean
        get() = this == ENTERTAINMENT || this == UNKNOWN

    /** Human-facing label for the detail screen. */
    val label: String
        get() = when (this) {
            EDUCATIONAL -> "Educational"
            ENTERTAINMENT -> "Entertainment"
            MIXED -> "Mixed"
            UNKNOWN -> "Unclassified"
        }

    companion object {
        fun fromWire(value: String): MediaClass =
            entries.firstOrNull { it.wire == value } ?: UNKNOWN
    }
}

/** A media item the student may play, joined with the window that offers it. */
data class CatalogEntry(
    val mediaItemId: String,
    val title: String,
    val overview: String,
    val mediaClass: MediaClass,
    val runtimeSeconds: Int,
    val subjectTags: List<String>,
    val standardCodes: List<String>,
    val availabilityWindowId: String,
    val windowEndsAt: Instant,
    val imagePath: String,
    val directPlayOk: Boolean,
    val container: String,
    val videoCodec: String,
    val audioCodec: String,
)

/** The catalog as of one server-stamped instant. */
data class Catalog(
    val entries: List<CatalogEntry>,
    val serverTime: Instant,
)

/** A playback authorization issued by `POST /media/{id}/grant`. */
data class PlayGrant(
    val grantId: String,
    val mediaItemId: String,
    val streamUrl: String,
    val startOffsetSeconds: Int,
    /**
     * Furthest playhead position this device has reached on the item. On-demand
     * seek ceiling and resume seed; zero for a fresh title or programmed grants.
     */
    val furthestPositionSeconds: Int = 0,
    val mode: String,
    val expiresAt: Instant,
    val serverTime: Instant,
) {
    /**
     * Whether the grant can still *start* playback. Once redeemed the server
     * keeps accepting progress for the session regardless of this, so it is only
     * consulted before the first frame.
     */
    fun startableAt(now: Instant): Boolean = now.isBefore(expiresAt)
}

/** Server-side state of a playback session after a heartbeat or completion. */
data class SessionProgress(
    val sessionId: String,
    val watchedSeconds: Int,
    val maxPositionSeconds: Int,
    val completed: Boolean,
    val playConsumed: Boolean,
    val serverTime: Instant,
)

/** A paired device. */
data class Device(
    val id: String,
    val name: String,
    val kind: String,
)

/** Credentials and identity persisted after a successful pairing. */
data class Pairing(
    val token: String,
    val device: Device,
)
