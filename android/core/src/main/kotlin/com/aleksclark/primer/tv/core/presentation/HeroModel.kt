package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.MediaClass

/** Primary action the hero offers. Screens map these to navigation/playback. */
enum class HeroAction {
    WATCH_LIVE,
    PLAY,
    VIEW_DETAILS,
    NONE,
}

/** Optional next-programme hint shown under gap or featured heroes. */
data class NextProgrammeHint(
    val mediaItemId: String,
    val title: String,
    val startsInSeconds: Int,
    val startsInLabel: String,
    val imagePath: String,
)

/**
 * Featured / on-now content at the top of Home.
 *
 * The screen supplies this model; [FeaturedHero] does not fetch.
 */
sealed interface HeroModel {
    val imagePath: String?
    val title: String
    val primaryAction: HeroAction

    /** Currently airing programme the student can join. */
    data class Live(
        val mediaItemId: String,
        val scheduleEntryId: String,
        override val title: String,
        val overview: String,
        val mediaClass: MediaClass,
        override val imagePath: String,
        val remainingSeconds: Int,
        val remainingLabel: String,
        val metadataLabel: String,
        val joinInProgress: Boolean,
        val tunable: Boolean,
        override val primaryAction: HeroAction = HeroAction.WATCH_LIVE,
    ) : HeroModel

    /** Curated on-demand title when the channel is in a gap or not loaded. */
    data class Featured(
        val card: MediaCardModel,
        val next: NextProgrammeHint? = null,
        override val primaryAction: HeroAction = HeroAction.PLAY,
    ) : HeroModel {
        override val imagePath: String get() = card.imagePath
        override val title: String get() = card.title
        val mediaItemId: String get() = card.mediaItemId
    }

    /**
     * Nothing on air and no catalog feature — still intentional, with an
     * optional next programme.
     */
    data class Empty(
        val message: UiText = UiText("Nothing is available right now"),
        val next: NextProgrammeHint? = null,
        override val primaryAction: HeroAction = HeroAction.NONE,
    ) : HeroModel {
        override val imagePath: String? get() = next?.imagePath
        override val title: String get() = next?.title ?: message.value
    }

    /** Structure-preserving skeleton while the first load is in flight. */
    data object Loading : HeroModel {
        override val imagePath: String? get() = null
        override val title: String get() = ""
        override val primaryAction: HeroAction get() = HeroAction.NONE
    }
}
