package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.CatalogPresenter
import com.aleksclark.primer.tv.core.domain.MediaClass
import java.time.Duration
import java.time.Instant

/**
 * Plain-language labels shared by cards, hero, and (later) details.
 *
 * Classification and one-viewing status are always text — never color alone.
 */
object MediaLabels {
    const val ONE_VIEWING = "One viewing"
    const val WATCHED = "Watched"
    const val LEAVING_SOON = "Leaving soon"
    const val LIVE = "Live"
    const val WATCH_LIVE = "Watch Live"
    const val PLAY = "Play"
    const val RESUME = "Resume"
    const val VIEW_DETAILS = "View details"

    fun runtime(seconds: Int): String = CatalogPresenter.formatRuntime(seconds)

    fun classification(mediaClass: MediaClass): String = mediaClass.label

    /**
     * Metadata line used under titles: `Educational · 30m`.
     * Optional trailing status fragments are appended by callers.
     */
    fun metadataLine(
        mediaClass: MediaClass,
        runtimeSeconds: Int,
        vararg trailing: String,
    ): String = buildString {
        append(classification(mediaClass))
        append(" · ")
        append(runtime(runtimeSeconds))
        trailing.filter { it.isNotBlank() }.forEach { part ->
            append(" · ")
            append(part)
        }
    }

    /**
     * Status chips for a catalog card, in display order.
     * Watched replaces One viewing once the session has consumed the play.
     */
    fun statusLabels(
        oneViewing: Boolean,
        watched: Boolean,
        leavingSoon: Boolean,
    ): List<String> = buildList {
        when {
            watched -> add(WATCHED)
            oneViewing -> add(ONE_VIEWING)
        }
        if (leavingSoon) add(LEAVING_SOON)
    }

    /** Compact status string for poster cards. */
    fun statusLine(
        oneViewing: Boolean,
        watched: Boolean,
        leavingSoon: Boolean,
    ): String? = statusLabels(oneViewing, watched, leavingSoon)
        .takeIf { it.isNotEmpty() }
        ?.joinToString(" · ")

    fun remainingLive(remainingSeconds: Int): String {
        if (remainingSeconds <= 0) return "Ending now"
        val minutes = (remainingSeconds + 59) / 60
        return when {
            minutes >= 60 -> {
                val hours = minutes / 60
                val mins = minutes % 60
                if (mins == 0) "${hours}h left" else "${hours}h ${mins}m left"
            }
            else -> "${minutes}m left"
        }
    }

    fun nextStartsIn(seconds: Int, at: Instant? = null, now: Instant? = null): String {
        val remaining = when {
            seconds > 0 -> seconds
            at != null && now != null -> Duration.between(now, at).seconds.toInt().coerceAtLeast(0)
            else -> 0
        }
        if (remaining <= 0) return "Starting soon"
        val minutes = (remaining + 59) / 60
        return when {
            minutes >= 60 -> {
                val hours = minutes / 60
                val mins = minutes % 60
                if (mins == 0) "Starts in ${hours}h" else "Starts in ${hours}h ${mins}m"
            }
            else -> "Starts in ${minutes}m"
        }
    }
}
