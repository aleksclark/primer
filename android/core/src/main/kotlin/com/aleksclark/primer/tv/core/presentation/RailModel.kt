package com.aleksclark.primer.tv.core.presentation

/**
 * Stable rail identity used for LazyListState keys and TV focus restoration.
 * Catalog refresh must not invent new IDs when the same logical rail remains.
 */
enum class RailId {
    LEARN,
    WORTH_WATCHING,
    ENTERTAINMENT,
    UNCLASSIFIED,
    LEAVING_SOON,
}

/** One titled horizontal row on Home. Empty rails are omitted by the presenter. */
data class RailModel(
    val id: RailId,
    val title: String,
    val items: List<MediaCardModel>,
) {
    val isEmpty: Boolean get() = items.isEmpty()

    fun itemIds(): List<String> = items.map { it.mediaItemId }
}

/** Maps catalog rail titles produced by [com.aleksclark.primer.tv.core.domain.CatalogPresenter]. */
fun railIdForCatalogTitle(title: String): RailId? = when (title) {
    "Learn" -> RailId.LEARN
    "Worth Watching" -> RailId.WORTH_WATCHING
    "Entertainment" -> RailId.ENTERTAINMENT
    "Unclassified" -> RailId.UNCLASSIFIED
    "Leaving Soon" -> RailId.LEAVING_SOON
    else -> null
}

fun RailId.defaultTitle(): String = when (this) {
    RailId.LEARN -> "Learn"
    RailId.WORTH_WATCHING -> "Worth Watching"
    RailId.ENTERTAINMENT -> "Entertainment"
    RailId.UNCLASSIFIED -> "Unclassified"
    RailId.LEAVING_SOON -> "Leaving Soon"
}
