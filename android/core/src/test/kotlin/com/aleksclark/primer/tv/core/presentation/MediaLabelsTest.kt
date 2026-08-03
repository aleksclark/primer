package com.aleksclark.primer.tv.core.presentation

import org.junit.Assert.assertEquals
import org.junit.Test

class MediaLabelsTest {

    @Test
    fun `runtime formatting matches catalog presenter`() {
        assertEquals("Unknown length", MediaLabels.runtime(0))
        assertEquals("1m", MediaLabels.runtime(30))
        assertEquals("45m", MediaLabels.runtime(45 * 60))
        assertEquals("1h", MediaLabels.runtime(3600))
        assertEquals("1h 30m", MediaLabels.runtime(5400))
    }

    @Test
    fun `remaining live prefers minutes then hours`() {
        assertEquals("Ending now", MediaLabels.remainingLive(0))
        assertEquals("1m left", MediaLabels.remainingLive(15))
        assertEquals("12m left", MediaLabels.remainingLive(12 * 60))
        assertEquals("1h left", MediaLabels.remainingLive(3600))
        assertEquals("1h 5m left", MediaLabels.remainingLive(3900))
    }

    @Test
    fun `next starts in labels are scannable`() {
        assertEquals("Starting soon", MediaLabels.nextStartsIn(0))
        assertEquals("Starts in 5m", MediaLabels.nextStartsIn(5 * 60))
        assertEquals("Starts in 2h", MediaLabels.nextStartsIn(7200))
        assertEquals("Starts in 1h 10m", MediaLabels.nextStartsIn(4200))
    }

    @Test
    fun `primary action labels are plain language`() {
        assertEquals("Play", MediaLabels.PLAY)
        assertEquals("Resume", MediaLabels.RESUME)
        assertEquals("Watched", MediaLabels.WATCHED)
        assertEquals("One viewing", MediaLabels.ONE_VIEWING)
    }
}
