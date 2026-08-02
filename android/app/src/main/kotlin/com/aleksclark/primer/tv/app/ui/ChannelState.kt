package com.aleksclark.primer.tv.app.ui

import com.aleksclark.primer.tv.core.domain.ChannelDay
import com.aleksclark.primer.tv.core.domain.ChannelNow
import com.aleksclark.primer.tv.core.domain.Programme

/** State of the channel screen: what is on now and what follows. */
data class ChannelUiState(
    val loading: Boolean = false,
    val now: ChannelNow? = null,
    val error: String? = null,
) {
    /** The programme a tune-in would join, if there is one. */
    val onAir: Programme? get() = now?.onAir

    /** Whether tuning in is possible: something is airing and the box can decode it. */
    val tunable: Boolean get() = onAir?.directPlayOk == true

    /** Whether the channel is between programmes. */
    val inGap: Boolean get() = now != null && now.inGap
}

/** State of the EPG screen. */
data class EpgUiState(
    val loading: Boolean = false,
    val day: ChannelDay? = null,
    val error: String? = null,
) {
    val isEmpty: Boolean get() = !loading && error == null && (day == null || day.isEmpty)
}
