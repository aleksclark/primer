package com.aleksclark.primer.tv.core.presentation

/**
 * Coherent Home model combining catalog rails, channel hero input, and load
 * flags. The UI receives this single state rather than stitching catalog and
 * channel flows itself.
 */
data class HomeUiState(
    val hero: HeroModel = HeroModel.Loading,
    val rails: List<RailModel> = emptyList(),
    val loading: Boolean = false,
    val refreshing: Boolean = false,
    val error: UiText? = null,
) {
    val hasContent: Boolean
        get() = hero !is HeroModel.Loading && (hero !is HeroModel.Empty || rails.isNotEmpty())

    val showSkeletons: Boolean
        get() = loading && !hasContent

    val isEmpty: Boolean
        get() = !loading && error == null && hero is HeroModel.Empty && rails.isEmpty()

    /** Stable media IDs currently shown, used to decide refresh preservation. */
    fun contentIds(): Set<String> = buildSet {
        when (val h = hero) {
            is HeroModel.Live -> add(h.mediaItemId)
            is HeroModel.Featured -> add(h.mediaItemId)
            is HeroModel.Empty -> h.next?.mediaItemId?.let(::add)
            HeroModel.Loading -> Unit
        }
        rails.forEach { rail -> rail.items.forEach { add(it.mediaItemId) } }
    }
}
