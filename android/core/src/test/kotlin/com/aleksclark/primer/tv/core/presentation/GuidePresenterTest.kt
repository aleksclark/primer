package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.ChannelDay
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.Programme
import com.aleksclark.primer.tv.core.testing.T0
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.ZoneOffset

class GuidePresenterTest {

    private val zone = ZoneOffset.UTC

    @Test
    fun `rows are labeled past current and future from server time`() {
        val day = dayWithPastCurrentFuture()
        val guide = GuidePresenter.presentDay(day, zone)

        assertEquals(3, guide.rows.size)
        assertEquals(ProgrammeTemporalState.PAST, guide.rows[0].temporal)
        assertEquals(GuidePresenter.FINISHED, guide.rows[0].statusLabel)
        assertEquals(ProgrammeTemporalState.CURRENT, guide.rows[1].temporal)
        assertEquals(GuidePresenter.ON_NOW, guide.rows[1].statusLabel)
        assertEquals(ProgrammeTemporalState.FUTURE, guide.rows[2].temporal)
        assertNull(guide.rows[2].statusLabel)
    }

    @Test
    fun `focus lands on the current programme when one is airing`() {
        val day = dayWithPastCurrentFuture()
        val index = GuidePresenter.focusIndex(day.programmes, day.serverTime)

        assertEquals(1, index)
        val guide = GuidePresenter.presentDay(day, zone)
        assertEquals(1, guide.focusIndex)
        assertEquals("sched-current", guide.focusScheduleEntryId)
        assertTrue(guide.rows[guide.focusIndex].isCurrent)
    }

    @Test
    fun `focus lands on the next future programme during a gap`() {
        val past = programme("past", airsIn = -7200, runtime = 3600)
        val next = programme("next", airsIn = 1800, runtime = 3600)
        val later = programme("later", airsIn = 7200, runtime = 3600)
        val day = ChannelDay(
            day = "2025-03-01",
            timezone = "UTC",
            programmes = listOf(past, next, later),
            serverTime = T0,
        )

        assertEquals(1, GuidePresenter.focusIndex(day.programmes, day.serverTime))
        val guide = GuidePresenter.presentDay(day, zone)
        assertEquals("sched-next", guide.focusScheduleEntryId)
        assertEquals(ProgrammeTemporalState.FUTURE, guide.rows[guide.focusIndex].temporal)
    }

    @Test
    fun `focus falls back to the last row when the whole day is past`() {
        val a = programme("a", airsIn = -10_800, runtime = 3600)
        val b = programme("b", airsIn = -7200, runtime = 3600)
        val day = ChannelDay(
            day = "2025-03-01",
            timezone = "UTC",
            programmes = listOf(a, b),
            serverTime = T0,
        )

        assertEquals(1, GuidePresenter.focusIndex(day.programmes, day.serverTime))
    }

    @Test
    fun `empty schedule reports empty without a focus target`() {
        val day = ChannelDay(
            day = "2025-03-01",
            timezone = "UTC",
            programmes = emptyList(),
            serverTime = T0,
        )
        val model = GuidePresenter.present(day = day, zoneId = zone)

        assertTrue(model.isEmpty)
        assertTrue(model.guide!!.isEmpty)
        assertEquals(-1, GuidePresenter.focusIndex(emptyList(), T0))
        assertEquals(-1, model.guide!!.focusIndex)
        assertNull(model.guide!!.focusScheduleEntryId)
        assertEquals(GuidePresenter.EMPTY_MESSAGE, "Nothing scheduled today")
    }

    @Test
    fun `present prefers the household timezone stamped on the day`() {
        // T0 is 12:00 UTC; America/Chicago is UTC-6 in March → 6:00 AM.
        val day = ChannelDay(
            day = "2025-03-01",
            timezone = "America/Chicago",
            programmes = listOf(programme("local", airsIn = 0, runtime = 1800)),
            serverTime = T0,
        )
        val guide = GuidePresenter.present(day = day, zoneId = ZoneOffset.UTC).guide!!

        assertEquals("6:00 AM", guide.rows.single().timeLabel)
    }

    @Test
    fun `loading without data shows skeletons`() {
        val model = GuidePresenter.present(day = null, loading = true)
        assertTrue(model.showSkeletons)
        assertNull(model.guide)
    }

    @Test
    fun `error without data surfaces the message`() {
        val model = GuidePresenter.present(day = null, error = "offline")
        assertEquals("offline", model.error?.value)
        assertFalse(model.showSkeletons)
    }

    @Test
    fun `metadata includes runtime classification and temporal status`() {
        val day = dayWithPastCurrentFuture()
        val current = GuidePresenter.presentDay(day, zone).rows[1]

        assertTrue(current.metadataLabel.contains(MediaLabels.runtime(3600)))
        assertTrue(current.metadataLabel.contains(MediaLabels.classification(MediaClass.EDUCATIONAL)))
        assertTrue(current.metadataLabel.contains(GuidePresenter.ON_NOW))
        assertEquals(GuidePresenter.ON_NOW, current.statusLabel)
    }

    @Test
    fun `clock labels use household-local zone`() {
        // T0 is 12:00 UTC; America/New_York is UTC-5 in March → 7:00 AM.
        val ny = java.time.ZoneId.of("America/New_York")
        val programme = programme("local", airsIn = 0, runtime = 1800)
        val row = GuidePresenter.row(programme, serverTime = T0, zoneId = ny)
        assertEquals("7:00 AM", row.timeLabel)
    }

    private fun dayWithPastCurrentFuture(): ChannelDay {
        val past = programme("past", airsIn = -7200, runtime = 3600)
        val current = programme("current", airsIn = -600, runtime = 3600)
        val future = programme("future", airsIn = 3600, runtime = 3600)
        return ChannelDay(
            day = "2025-03-01",
            timezone = "UTC",
            programmes = listOf(past, current, future),
            serverTime = T0,
        )
    }

    private fun programme(
        id: String,
        airsIn: Long,
        runtime: Int,
    ): Programme = Programme(
        scheduleEntryId = "sched-$id",
        mediaItemId = "media-$id",
        title = id.replaceFirstChar { it.uppercase() },
        overview = "",
        mediaClass = MediaClass.EDUCATIONAL,
        subjectTags = emptyList(),
        runtimeSeconds = runtime,
        airsAt = T0.plusSeconds(airsIn),
        endsAt = T0.plusSeconds(airsIn + runtime),
        block = "day",
        joinInProgress = true,
        directPlayOk = true,
        imagePath = "/images/$id/Primary",
    )
}
