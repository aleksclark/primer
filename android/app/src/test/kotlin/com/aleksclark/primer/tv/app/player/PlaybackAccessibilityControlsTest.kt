package com.aleksclark.primer.tv.app.player

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PlaybackAccessibilityControlsTest {
    @Test
    fun `subtitle options omit unsupported tracks and retain selection coordinates`() {
        val options = subtitleOptions(
            listOf(
                SubtitleTrackDescriptor(0, 0, "English", "en", supported = true, selected = true),
                SubtitleTrackDescriptor(0, 1, "Commentary", "en", supported = false, selected = false),
                SubtitleTrackDescriptor(2, 3, null, "es", supported = true, selected = false),
            ),
        )

        assertEquals(
            listOf(
                SubtitleOption(0, 0, "English", selected = true),
                SubtitleOption(2, 3, "es", selected = false),
            ),
            options,
        )
    }

    @Test
    fun `audio options create distinct labels for duplicate unnamed tracks`() {
        val options = audioOptions(
            listOf(
                AudioTrackDescriptor(0, 0, null, null, supported = true, selected = true),
                AudioTrackDescriptor(1, 0, " ", " ", supported = true, selected = false),
            ),
        )

        assertEquals(listOf("Audio 1", "Audio 2"), options.map(AudioOption::label))
        assertEquals(true, options.first().selected)
    }

    @Test
    fun `volume percent clamps invalid stream values`() {
        assertEquals(50, volumePercent(current = 5, maximum = 10))
        assertEquals(100, volumePercent(current = 12, maximum = 10))
        assertEquals(0, volumePercent(current = -1, maximum = 10))
        assertEquals(0, volumePercent(current = 1, maximum = 0))
    }

    @Test
    fun `media controls only appear while the timed overlay is visible`() {
        assertEquals(true, mediaControlsVisible(overlayVisible = true))
        assertEquals(false, mediaControlsVisible(overlayVisible = false))
    }

    @Test
    fun `live card refresh waits one minute past projected end`() {
        assertEquals(160L, liveCardRefreshDelaySeconds(remainingSeconds = 100, nextStartsInSeconds = null))
        assertEquals(75L, liveCardRefreshDelaySeconds(remainingSeconds = null, nextStartsInSeconds = 15))
        assertEquals(120L, liveCardRefreshDelaySeconds(remainingSeconds = 60, nextStartsInSeconds = 5))
        assertNull(liveCardRefreshDelaySeconds(remainingSeconds = 0, nextStartsInSeconds = 0))
        assertNull(liveCardRefreshDelaySeconds(remainingSeconds = null, nextStartsInSeconds = null))
    }
}
