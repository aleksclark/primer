package com.aleksclark.primer.tv.core.domain

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PlaybackPolicyTest {

    @Test
    fun `the programmed player offers no transport at all`() {
        val controls = PlaybackPolicy.controlsFor(PlaybackMode.PROGRAMMED, MediaClass.EDUCATIONAL)

        assertFalse("no seeking on a broadcast", controls.seekAllowed)
        assertFalse("no fast-forward or rewind bar", controls.showSeekBar)
        assertFalse("pausing would leave the student behind the channel", controls.pauseAllowed)
        assertFalse("an empty control overlay still steals focus", controls.showTransportControls)
        assertTrue(controls.followsBroadcast)
    }

    @Test
    fun `the class does not loosen the programmed policy`() {
        for (mediaClass in MediaClass.entries) {
            val controls = PlaybackPolicy.controlsFor(PlaybackMode.PROGRAMMED, mediaClass)
            assertFalse("$mediaClass must not be seekable on the channel", controls.seekAllowed)
            assertFalse("$mediaClass must not be pausable on the channel", controls.pauseAllowed)
        }
    }

    @Test
    fun `on demand still follows the item's class`() {
        val study = PlaybackPolicy.controlsFor(PlaybackMode.ON_DEMAND, MediaClass.EDUCATIONAL)
        assertTrue(study.seekAllowed)
        assertTrue(study.pauseAllowed)
        assertFalse(study.followsBroadcast)

        val rationed = PlaybackPolicy.controlsFor(PlaybackMode.ON_DEMAND, MediaClass.ENTERTAINMENT)
        assertFalse("scrubbing to the credits would burn the play unwatched", rationed.seekAllowed)
        assertTrue(rationed.pauseAllowed)
        assertTrue(rationed.showTransportControls)
    }

    @Test
    fun `an unknown mode degrades to the locked-down policy`() {
        assertEquals(PlaybackMode.PROGRAMMED, PlaybackMode.fromWire("time_shifted"))
        assertEquals(PlaybackMode.PROGRAMMED, PlaybackMode.fromWire(""))
        assertEquals(PlaybackMode.ON_DEMAND, PlaybackMode.fromWire("on_demand"))
        assertEquals(PlaybackMode.PROGRAMMED, PlaybackMode.fromWire("programmed"))
    }

    @Test
    fun `a playhead level with the broadcast is left alone`() {
        assertNull(PlaybackPolicy.broadcastCorrectionSeconds(serverOffsetSeconds = 600, playheadSeconds = 600))
        assertNull(
            PlaybackPolicy.broadcastCorrectionSeconds(serverOffsetSeconds = 605, playheadSeconds = 600),
        )
    }

    @Test
    fun `a playhead that has fallen behind is pulled up to the broadcast`() {
        assertEquals(
            660,
            PlaybackPolicy.broadcastCorrectionSeconds(serverOffsetSeconds = 660, playheadSeconds = 600),
        )
    }

    @Test
    fun `a playhead that has run ahead is not rewound`() {
        // Showing a scene twice is worse than a few seconds of drift, and the
        // next re-sync catches up anyway.
        assertNull(
            PlaybackPolicy.broadcastCorrectionSeconds(serverOffsetSeconds = 600, playheadSeconds = 900),
        )
    }

    @Test
    fun `the channel never warns about burning a play`() {
        // A programmed grant is authorized by the grid, not by an availability
        // window, so nothing is rationed.
        assertFalse(WatchOnce.shouldWarnBeforePlay(MediaClass.ENTERTAINMENT, PlaybackMode.PROGRAMMED))
        assertTrue(WatchOnce.shouldWarnBeforePlay(MediaClass.ENTERTAINMENT, PlaybackMode.ON_DEMAND))
        assertFalse(WatchOnce.shouldWarnBeforePlay(MediaClass.EDUCATIONAL, PlaybackMode.ON_DEMAND))
    }

    @Test
    fun `on-demand resume starts 30 seconds before the furthest position`() {
        assertEquals(0, PlaybackPolicy.resumePositionSeconds(0))
        assertEquals(0, PlaybackPolicy.resumePositionSeconds(20))
        assertEquals(0, PlaybackPolicy.resumePositionSeconds(30))
        assertEquals(870, PlaybackPolicy.resumePositionSeconds(900))
    }

    @Test
    fun `on-demand seek window is five minutes behind the furthest mark`() {
        assertEquals(0..0, PlaybackPolicy.seekWindowSeconds(0))
        assertEquals(0..120, PlaybackPolicy.seekWindowSeconds(120))
        assertEquals(0..300, PlaybackPolicy.seekWindowSeconds(300))
        assertEquals(600..900, PlaybackPolicy.seekWindowSeconds(900))
    }

    @Test
    fun `on-demand seeks past the furthest mark are clamped back`() {
        // 15 minutes furthest; 5-minute rewind floor at 600s.
        val furthestMs = 900_000L
        assertEquals(
            900_000L,
            PlaybackPolicy.clampSeekPositionMs(1_200_000L, furthestMs),
        )
        assertEquals(
            600_000L,
            PlaybackPolicy.clampSeekPositionMs(0L, furthestMs),
        )
        assertEquals(
            750_000L,
            PlaybackPolicy.clampSeekPositionMs(750_000L, furthestMs),
        )
    }

    @Test
    fun `on-demand seek clamp respects a known duration`() {
        // Watermark past EOF should still not land past duration.
        assertEquals(
            100_000L,
            PlaybackPolicy.clampSeekPositionMs(
                requestedMs = 200_000L,
                furthestPositionMs = 500_000L,
                durationMs = 100_000L,
            ),
        )
    }
}

class ProgrammeTest {
    private val airsAt: Instant = Instant.parse("2025-03-01T11:50:00Z")

    private fun programme(runtimeSeconds: Int = 1_800, joinInProgress: Boolean = true) = Programme(
        scheduleEntryId = "entry-1",
        mediaItemId = "media-1",
        title = "Inertia",
        overview = "",
        mediaClass = MediaClass.EDUCATIONAL,
        subjectTags = emptyList(),
        runtimeSeconds = runtimeSeconds,
        airsAt = airsAt,
        endsAt = airsAt.plusSeconds(runtimeSeconds.toLong()),
        block = "morning",
        joinInProgress = joinInProgress,
        directPlayOk = true,
        imagePath = "/images/media-1/Primary",
    )

    @Test
    fun `the offset is clamped to the programme's own span`() {
        val p = programme()
        assertEquals(0, p.offsetSecondsAt(airsAt.minusSeconds(60)))
        assertEquals(0, p.offsetSecondsAt(airsAt))
        assertEquals(600, p.offsetSecondsAt(airsAt.plusSeconds(600)))
        assertEquals(1_800, p.offsetSecondsAt(airsAt.plusSeconds(9_000)))
    }

    @Test
    fun `the airing span is half-open`() {
        val p = programme()
        assertFalse(p.airingAt(airsAt.minusSeconds(1)))
        assertTrue(p.airingAt(airsAt))
        assertTrue(p.airingAt(airsAt.plusSeconds(1_799)))
        assertFalse("the end of the slot belongs to the next programme", p.airingAt(airsAt.plusSeconds(1_800)))
    }
}
