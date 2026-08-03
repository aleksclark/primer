package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.ChannelNow
import com.aleksclark.primer.tv.core.domain.Programme
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * Pure reducer for the Channel destination. Maps server [ChannelNow] into an
 * [OnNowHeroModel] the UI can render without domain branching.
 */
object ChannelPresenter {

    private val clockFormatter: DateTimeFormatter = DateTimeFormatter.ofPattern("h:mm a")

    fun present(
        now: ChannelNow?,
        loading: Boolean = false,
        error: String? = null,
        zoneId: ZoneId = ZoneId.systemDefault(),
    ): ChannelScreenModel {
        if (loading && now == null && error == null) {
            return ChannelScreenModel(hero = OnNowHeroModel.Loading, loading = true)
        }
        if (error != null && now == null) {
            return ChannelScreenModel(
                hero = OnNowHeroModel.Error(UiText(error)),
                loading = false,
                error = UiText(error),
            )
        }

        val onAir = now?.onAir
        val hero = when {
            now == null -> OnNowHeroModel.Loading
            onAir != null -> onAirHero(onAir, now, zoneId)
            else -> gapHero(now)
        }

        return ChannelScreenModel(
            hero = hero,
            loading = loading && now == null,
            refreshing = loading && now != null,
            error = error?.let(UiText::of),
        )
    }

    fun onAirHero(
        programme: Programme,
        channelNow: ChannelNow,
        zoneId: ZoneId = ZoneId.systemDefault(),
    ): OnNowHeroModel.OnAir {
        val remaining = channelNow.remainingSeconds
        val remainingLabel = MediaLabels.remainingLive(remaining)
        val progressLabel = progressLabel(
            offsetSeconds = channelNow.offsetSeconds,
            remainingSeconds = remaining,
        )
        val tunable = programme.directPlayOk
        return OnNowHeroModel.OnAir(
            mediaItemId = programme.mediaItemId,
            scheduleEntryId = programme.scheduleEntryId,
            title = programme.title,
            overview = programme.overview,
            mediaClass = programme.mediaClass,
            imagePath = programme.imagePath,
            runtimeSeconds = programme.runtimeSeconds,
            airsAtLabel = clockTime(programme.airsAt, zoneId),
            endsAtLabel = clockTime(programme.endsAt, zoneId),
            runtimeLabel = MediaLabels.runtime(programme.runtimeSeconds),
            metadataLabel = MediaLabels.metadataLine(
                mediaClass = programme.mediaClass,
                runtimeSeconds = programme.runtimeSeconds,
                MediaLabels.LIVE,
                remainingLabel,
            ),
            offsetSeconds = channelNow.offsetSeconds,
            remainingSeconds = remaining,
            progressLabel = progressLabel,
            remainingLabel = remainingLabel,
            joinInProgress = programme.joinInProgress,
            tunable = tunable,
            notPlayableReason = if (!tunable) OnNowHeroModel.CANNOT_PLAY else null,
            joinHint = if (programme.joinInProgress) {
                OnNowHeroModel.JOIN_IN_PROGRESS_HINT
            } else {
                OnNowHeroModel.STARTS_FROM_BEGINNING_HINT
            },
            next = nextHint(channelNow),
        )
    }

    fun gapHero(channelNow: ChannelNow): OnNowHeroModel.Gap = OnNowHeroModel.Gap(
        message = UiText(OnNowHeroModel.NOTHING_ON_NOW),
        next = nextHint(channelNow),
    )

    fun nextHint(channelNow: ChannelNow?): NextProgrammeHint? {
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

    /**
     * "15 min in · 30 min left" when the student would join mid-broadcast;
     * null at the very start so the UI does not shout zero progress.
     */
    fun progressLabel(offsetSeconds: Int, remainingSeconds: Int): String? {
        if (offsetSeconds <= 0) return null
        val inMin = offsetSeconds / 60
        val leftMin = (remainingSeconds / 60).coerceAtLeast(1)
        return "$inMin min in · $leftMin min left"
    }

    fun clockTime(instant: java.time.Instant, zoneId: ZoneId = ZoneId.systemDefault()): String =
        clockFormatter.format(instant.atZone(zoneId))
}
