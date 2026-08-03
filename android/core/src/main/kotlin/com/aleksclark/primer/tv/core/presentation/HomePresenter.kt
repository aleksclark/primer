package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.Catalog
import com.aleksclark.primer.tv.core.domain.CatalogPresenter
import com.aleksclark.primer.tv.core.domain.CatalogView
import com.aleksclark.primer.tv.core.domain.ChannelNow
import com.aleksclark.primer.tv.core.domain.Programme

/**
 * Pure reducer for Home. Combines catalog rails, on-air channel state, and
 * session-consumed IDs into one [HomeUiState].
 *
 * No Android types — unit-tested on the JVM.
 */
object HomePresenter {

    /**
     * Builds Home from domain inputs.
     *
     * Hero priority:
     * 1. Tunable on-air programme (direct-play OK).
     * 2. First curated catalog card (Learn → Worth Watching → Entertainment).
     * 3. Empty/gap hero with optional next programme.
     */
    fun present(
        catalog: Catalog?,
        channelNow: ChannelNow?,
        consumedMediaItemIds: Set<String> = emptySet(),
        loading: Boolean = false,
        refreshing: Boolean = false,
        error: String? = null,
    ): HomeUiState {
        val view = catalog?.let { CatalogPresenter.present(it, consumedMediaItemIds) }
        return presentView(
            catalogView = view,
            channelNow = channelNow,
            loading = loading,
            refreshing = refreshing,
            error = error,
        )
    }

    fun presentView(
        catalogView: CatalogView?,
        channelNow: ChannelNow?,
        loading: Boolean = false,
        refreshing: Boolean = false,
        error: String? = null,
    ): HomeUiState {
        if (loading && catalogView == null && channelNow == null) {
            return HomeUiState(
                hero = HeroModel.Loading,
                rails = skeletonRails(),
                loading = true,
                refreshing = false,
                error = error?.let(UiText::of),
            )
        }

        val rails = buildRails(catalogView)
        val hero = selectHero(catalogView = catalogView, channelNow = channelNow, rails = rails)

        return HomeUiState(
            hero = hero,
            rails = rails,
            loading = loading && catalogView == null,
            refreshing = refreshing,
            error = error?.let(UiText::of),
        )
    }

    /**
     * Applies a successful reload while preserving the previous structure when
     * media IDs are unchanged. Callers keep LazyListState keyed by [RailId].
     */
    fun applyRefreshSuccess(
        previous: HomeUiState,
        catalog: Catalog?,
        channelNow: ChannelNow?,
        consumedMediaItemIds: Set<String> = emptySet(),
    ): HomeUiState = present(
        catalog = catalog,
        channelNow = channelNow,
        consumedMediaItemIds = consumedMediaItemIds,
        loading = false,
        refreshing = false,
        error = null,
    ).let { next ->
        // Structural equality of IDs is enough for scroll/focus preservation;
        // models still update labels (watched, leaving soon) in place.
        next
    }

    /**
     * Non-destructive refresh failure: keep existing content, surface a
     * non-blocking error, clear the refreshing flag.
     */
    fun applyRefreshFailure(previous: HomeUiState, message: String): HomeUiState {
        if (!previous.hasContent && previous.hero is HeroModel.Loading) {
            return HomeUiState(
                hero = HeroModel.Empty(message = UiText("Couldn't load Home")),
                rails = emptyList(),
                loading = false,
                refreshing = false,
                error = UiText(message),
            )
        }
        return previous.copy(
            loading = false,
            refreshing = false,
            error = UiText(message),
        )
    }

    /** Marks refresh-in-flight without replacing content already on screen. */
    fun beginRefresh(previous: HomeUiState): HomeUiState {
        if (!previous.hasContent) {
            return previous.copy(
                hero = HeroModel.Loading,
                rails = if (previous.rails.isEmpty()) skeletonRails() else previous.rails,
                loading = true,
                refreshing = false,
                error = null,
            )
        }
        return previous.copy(refreshing = true, error = null)
    }

    fun selectHero(
        catalogView: CatalogView?,
        channelNow: ChannelNow?,
        rails: List<RailModel> = buildRails(catalogView),
    ): HeroModel {
        val onAir = channelNow?.onAir
        if (onAir != null && onAir.directPlayOk) {
            return liveHero(onAir, channelNow)
        }

        val featured = firstCuratedCard(rails)
        if (featured != null) {
            val action = when {
                featured.watched -> HeroAction.VIEW_DETAILS
                featured.playable -> HeroAction.PLAY
                else -> HeroAction.VIEW_DETAILS
            }
            return HeroModel.Featured(
                card = featured,
                next = nextHint(channelNow),
                primaryAction = action,
            )
        }

        return HeroModel.Empty(
            message = UiText("Nothing is available right now"),
            next = nextHint(channelNow),
        )
    }

    fun buildRails(catalogView: CatalogView?): List<RailModel> {
        if (catalogView == null) return emptyList()

        val primary = catalogView.rails.mapNotNull { rail ->
            val id = railIdForCatalogTitle(rail.title) ?: return@mapNotNull null
            val items = rail.cards.map { it.toMediaCardModel() }
            if (items.isEmpty()) null else RailModel(id = id, title = rail.title, items = items)
        }

        val leaving = leavingSoonRail(primary)
        return if (leaving == null) primary else primary + leaving
    }

    /**
     * Optional cross-listed rail of titles leaving within the expiring window.
     * Deduplicates by media ID while preserving Learn → Entertainment order.
     */
    fun leavingSoonRail(rails: List<RailModel>): RailModel? {
        val seen = linkedSetOf<String>()
        val items = buildList {
            for (rail in rails) {
                if (rail.id == RailId.LEAVING_SOON) continue
                for (item in rail.items) {
                    if (!item.leavingSoon || item.watched) continue
                    if (!seen.add(item.mediaItemId)) continue
                    add(item)
                }
            }
        }
        if (items.isEmpty()) return null
        return RailModel(
            id = RailId.LEAVING_SOON,
            title = RailId.LEAVING_SOON.defaultTitle(),
            items = items,
        )
    }

    fun firstCuratedCard(rails: List<RailModel>): MediaCardModel? {
        val order = listOf(
            RailId.LEARN,
            RailId.WORTH_WATCHING,
            RailId.ENTERTAINMENT,
            RailId.UNCLASSIFIED,
        )
        for (id in order) {
            val card = rails.firstOrNull { it.id == id }?.items?.firstOrNull { !it.watched }
                ?: rails.firstOrNull { it.id == id }?.items?.firstOrNull()
            if (card != null) return card
        }
        return null
    }

    private fun liveHero(programme: Programme, channelNow: ChannelNow): HeroModel.Live {
        val remaining = channelNow.remainingSeconds
        return HeroModel.Live(
            mediaItemId = programme.mediaItemId,
            scheduleEntryId = programme.scheduleEntryId,
            title = programme.title,
            overview = programme.overview,
            mediaClass = programme.mediaClass,
            imagePath = programme.imagePath,
            remainingSeconds = remaining,
            remainingLabel = MediaLabels.remainingLive(remaining),
            metadataLabel = MediaLabels.metadataLine(
                mediaClass = programme.mediaClass,
                runtimeSeconds = programme.runtimeSeconds,
                MediaLabels.LIVE,
                MediaLabels.remainingLive(remaining),
            ),
            joinInProgress = programme.joinInProgress,
            tunable = programme.directPlayOk,
            primaryAction = HeroAction.WATCH_LIVE,
        )
    }

    private fun nextHint(channelNow: ChannelNow?): NextProgrammeHint? {
        val next = channelNow?.next ?: return null
        val startsIn = channelNow.nextStartsInSeconds
        return NextProgrammeHint(
            mediaItemId = next.mediaItemId,
            title = next.title,
            startsInSeconds = startsIn,
            startsInLabel = MediaLabels.nextStartsIn(startsIn),
            imagePath = next.imagePath,
        )
    }

    /** Placeholder rail shells so first paint keeps Home structure. */
    fun skeletonRails(): List<RailModel> = listOf(
        RailModel(RailId.LEARN, RailId.LEARN.defaultTitle(), emptyList()),
        RailModel(RailId.WORTH_WATCHING, RailId.WORTH_WATCHING.defaultTitle(), emptyList()),
        RailModel(RailId.ENTERTAINMENT, RailId.ENTERTAINMENT.defaultTitle(), emptyList()),
    )
}
