package com.aleksclark.primer.tv.core.domain

import java.time.Duration
import java.time.Instant

/** A catalog entry as the home screen should render it. */
data class CatalogCard(
    val entry: CatalogEntry,
    /**
     * Whether this item's single play was used up during this app session.
     *
     * The server's catalog query drops consumed items, so this is only ever true
     * for an item finished since the last refresh — it exists so the row the
     * student just watched greys out immediately instead of vanishing mid-gesture.
     */
    val alreadyWatched: Boolean,
    /** Whether finishing this item will use up its single viewing. */
    val consumesPlay: Boolean,
    /** Whether the window closes soon enough to be worth flagging. */
    val expiringSoon: Boolean,
) {
    /** Whether a play action should be offered. */
    val playable: Boolean get() = !alreadyWatched
}

/** One titled group of cards on the home screen. */
data class CatalogRail(
    val title: String,
    val cards: List<CatalogCard>,
)

/** The home screen's full content. */
data class CatalogView(
    val rails: List<CatalogRail>,
    val serverTime: Instant,
) {
    val isEmpty: Boolean get() = rails.all { it.cards.isEmpty() }
    val cards: List<CatalogCard> get() = rails.flatMap { it.cards }
}

/**
 * Turns a catalog into the rails the home screen shows.
 *
 * The grouping is by class rather than subject because class is what governs
 * player behaviour and rationing, so it is the distinction the student needs to
 * see. Educational content leads, since that is the point of the channel.
 */
object CatalogPresenter {
    /** Windows closing inside this span are flagged to the student. */
    val EXPIRING_SOON: Duration = Duration.ofHours(24)

    private val RAIL_ORDER = listOf(
        MediaClass.EDUCATIONAL to "Learn",
        MediaClass.MIXED to "Worth Watching",
        MediaClass.ENTERTAINMENT to "Entertainment",
        MediaClass.UNKNOWN to "Unclassified",
    )

    /**
     * @param consumedMediaItemIds items whose play the server reported consumed
     *   during this app session, which stay visible but greyed until the next
     *   catalog refresh drops them.
     */
    fun present(catalog: Catalog, consumedMediaItemIds: Set<String> = emptySet()): CatalogView {
        val cards = catalog.entries.map { entry ->
            CatalogCard(
                entry = entry,
                alreadyWatched = entry.mediaItemId in consumedMediaItemIds,
                consumesPlay = entry.mediaClass.consumesPlay,
                expiringSoon = !entry.windowEndsAt.isAfter(catalog.serverTime.plus(EXPIRING_SOON)),
            )
        }

        val byClass = cards.groupBy { it.entry.mediaClass }
        val rails = RAIL_ORDER.mapNotNull { (mediaClass, title) ->
            val group = byClass[mediaClass]?.sortedWith(
                // Watched items sink to the bottom of their rail; the rest sort
                // by the title the admin curated for shelf order.
                compareBy({ it.alreadyWatched }, { it.entry.title.lowercase() }),
            )
            if (group.isNullOrEmpty()) null else CatalogRail(title = title, cards = group)
        }

        return CatalogView(rails = rails, serverTime = catalog.serverTime)
    }

    /** Formats a runtime for the catalog and detail screens. */
    fun formatRuntime(seconds: Int): String {
        if (seconds <= 0) return "Unknown length"
        val hours = seconds / 3600
        val minutes = (seconds % 3600) / 60
        return when {
            hours > 0 && minutes > 0 -> "${hours}h ${minutes}m"
            hours > 0 -> "${hours}h"
            else -> "${minutes.coerceAtLeast(1)}m"
        }
    }
}
