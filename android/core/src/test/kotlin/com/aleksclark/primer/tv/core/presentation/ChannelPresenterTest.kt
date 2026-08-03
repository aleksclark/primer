package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.ChannelNow
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.Programme
import com.aleksclark.primer.tv.core.testing.T0
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.ZoneOffset

class ChannelPresenterTest {

    private val zone = ZoneOffset.UTC

    @Test
    fun `loading with no snapshot yields a skeleton hero`() {
        val model = ChannelPresenter.present(now = null, loading = true)

        assertTrue(model.showSkeletons)
        assertEquals(OnNowHeroModel.Loading, model.hero)
        assertFalse(model.tunable)
    }

    @Test
    fun `on-air programme offers Watch Live with remaining and progress`() {
        val now = onAirNow(offsetSeconds = 600, remainingRuntime = 1800)
        val model = ChannelPresenter.present(now = now, zoneId = zone)

        val hero = model.hero as OnNowHeroModel.OnAir
        assertEquals("Inertia", hero.title)
        assertTrue(hero.tunable)
        assertTrue(hero.joinInProgress)
        assertEquals(MediaLabels.remainingLive(1200), hero.remainingLabel)
        assertEquals("10 min in · 20 min left", hero.progressLabel)
        assertEquals(OnNowHeroModel.JOIN_IN_PROGRESS_HINT, hero.joinHint)
        assertNull(hero.notPlayableReason)
        assertTrue(model.tunable)
        assertFalse(model.inGap)
    }

    @Test
    fun `zero offset omits the mid-broadcast progress line`() {
        val now = onAirNow(offsetSeconds = 0, remainingRuntime = 1800)
        val hero = ChannelPresenter.present(now = now, zoneId = zone).hero as OnNowHeroModel.OnAir

        assertNull(hero.progressLabel)
        assertEquals(MediaLabels.remainingLive(1800), hero.remainingLabel)
    }

    @Test
    fun `non-direct-play on-air is visible but not tunable`() {
        val now = onAirNow(offsetSeconds = 60, remainingRuntime = 1800, directPlayOk = false)
        val model = ChannelPresenter.present(now = now, zoneId = zone)
        val hero = model.hero as OnNowHeroModel.OnAir

        assertFalse(hero.tunable)
        assertFalse(model.tunable)
        assertEquals(OnNowHeroModel.CANNOT_PLAY, hero.notPlayableReason)
    }

    @Test
    fun `gap surfaces next title and disables Watch Live`() {
        val now = gapNow(nextTitle = "Gravity", startsInSeconds = 1800)
        val model = ChannelPresenter.present(now = now, zoneId = zone)

        assertTrue(model.inGap)
        assertFalse(model.tunable)
        val hero = model.hero as OnNowHeroModel.Gap
        assertEquals(OnNowHeroModel.NOTHING_ON_NOW, hero.message.value)
        assertNotNull(hero.next)
        assertEquals("Gravity", hero.next?.title)
        assertEquals(MediaLabels.nextStartsIn(1800), hero.next?.startsInLabel)
    }

    @Test
    fun `gap with no next is still a gap`() {
        val now = ChannelNow(
            onAir = null,
            offsetSeconds = 0,
            startOffsetSeconds = 0,
            next = null,
            nextStartsInSeconds = 0,
            serverTime = T0,
        )
        val hero = ChannelPresenter.present(now = now).hero as OnNowHeroModel.Gap
        assertNull(hero.next)
    }

    @Test
    fun `hard error without snapshot becomes an error hero`() {
        val model = ChannelPresenter.present(now = null, error = "network down")

        val hero = model.hero as OnNowHeroModel.Error
        assertEquals("network down", hero.message.value)
        assertEquals("network down", model.error?.value)
    }

    @Test
    fun `refresh failure keeps prior on-air and attaches a non-blocking error`() {
        val now = onAirNow()
        val model = ChannelPresenter.present(now = now, loading = false, error = "stale refresh")

        assertTrue(model.hero is OnNowHeroModel.OnAir)
        assertEquals("stale refresh", model.error?.value)
    }

    @Test
    fun `up next is attached under an on-air hero when known`() {
        val next = programme(id = "n1", title = "Next Up", airsIn = 1200, runtime = 1800)
        val onAir = programme(id = "a1", title = "On Air", airsIn = -600, runtime = 1800)
        val now = ChannelNow(
            onAir = onAir,
            offsetSeconds = 600,
            startOffsetSeconds = 600,
            next = next,
            nextStartsInSeconds = 1200,
            serverTime = T0,
        )
        val hero = ChannelPresenter.present(now = now, zoneId = zone).hero as OnNowHeroModel.OnAir
        assertEquals("Next Up", hero.next?.title)
        assertEquals(MediaLabels.nextStartsIn(1200), hero.next?.startsInLabel)
    }

    private fun onAirNow(
        offsetSeconds: Int = 600,
        remainingRuntime: Int = 1800,
        directPlayOk: Boolean = true,
    ): ChannelNow {
        val programme = programme(
            id = "a1",
            title = "Inertia",
            airsIn = -offsetSeconds.toLong(),
            runtime = remainingRuntime,
            directPlayOk = directPlayOk,
            joinInProgress = offsetSeconds > 0,
        )
        return ChannelNow(
            onAir = programme,
            offsetSeconds = offsetSeconds,
            startOffsetSeconds = offsetSeconds,
            next = null,
            nextStartsInSeconds = 0,
            serverTime = T0,
        )
    }

    private fun gapNow(nextTitle: String, startsInSeconds: Int): ChannelNow {
        val next = programme(
            id = "n1",
            title = nextTitle,
            airsIn = startsInSeconds.toLong(),
            runtime = 2400,
        )
        return ChannelNow(
            onAir = null,
            offsetSeconds = 0,
            startOffsetSeconds = 0,
            next = next,
            nextStartsInSeconds = startsInSeconds,
            serverTime = T0,
        )
    }

    private fun programme(
        id: String,
        title: String,
        airsIn: Long,
        runtime: Int,
        directPlayOk: Boolean = true,
        joinInProgress: Boolean = true,
    ): Programme = Programme(
        scheduleEntryId = "sched-$id",
        mediaItemId = "media-$id",
        title = title,
        overview = "Overview of $title",
        mediaClass = MediaClass.EDUCATIONAL,
        subjectTags = emptyList(),
        runtimeSeconds = runtime,
        airsAt = T0.plusSeconds(airsIn),
        endsAt = T0.plusSeconds(airsIn + runtime),
        block = "morning",
        joinInProgress = joinInProgress,
        directPlayOk = directPlayOk,
        imagePath = "/images/$id/Primary",
    )
}
