package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.MediaClass

/**
 * Channel destination presentation: what is on now, a gap before the next
 * programme, loading, or an error that preserves chrome outside the hero.
 */
sealed interface OnNowHeroModel {
    /** First paint / explicit reload with no prior snapshot. */
    data object Loading : OnNowHeroModel

    /** Something is airing; Watch Live may or may not be enabled. */
    data class OnAir(
        val mediaItemId: String,
        val scheduleEntryId: String,
        val title: String,
        val overview: String,
        val mediaClass: MediaClass,
        val imagePath: String,
        val runtimeSeconds: Int,
        val airsAtLabel: String,
        val endsAtLabel: String,
        val runtimeLabel: String,
        val metadataLabel: String,
        val offsetSeconds: Int,
        val remainingSeconds: Int,
        val progressLabel: String?,
        val remainingLabel: String,
        val joinInProgress: Boolean,
        val tunable: Boolean,
        val notPlayableReason: String?,
        val joinHint: String,
        val next: NextProgrammeHint?,
    ) : OnNowHeroModel

    /** Between programmes — Watch Live is unavailable. */
    data class Gap(
        val message: UiText = UiText(NOTHING_ON_NOW),
        val next: NextProgrammeHint?,
    ) : OnNowHeroModel

    /** Hard failure with no prior on-now data. */
    data class Error(
        val message: UiText,
    ) : OnNowHeroModel

    companion object {
        const val NOTHING_ON_NOW = "Nothing is on the channel right now."
        const val JOIN_IN_PROGRESS_HINT =
            "Tuning in joins the broadcast where it is now. There is no rewind."
        const val STARTS_FROM_BEGINNING_HINT = "This programme starts from the beginning."
        const val CANNOT_PLAY = "This programme cannot play on this device."
    }
}

/** Full Channel screen model derived from [com.aleksclark.primer.tv.app.ui.ChannelUiState] inputs. */
data class ChannelScreenModel(
    val hero: OnNowHeroModel,
    val loading: Boolean = false,
    val refreshing: Boolean = false,
    val error: UiText? = null,
) {
    val tunable: Boolean
        get() = (hero as? OnNowHeroModel.OnAir)?.tunable == true

    val inGap: Boolean
        get() = hero is OnNowHeroModel.Gap

    val showSkeletons: Boolean
        get() = loading && hero is OnNowHeroModel.Loading
}
