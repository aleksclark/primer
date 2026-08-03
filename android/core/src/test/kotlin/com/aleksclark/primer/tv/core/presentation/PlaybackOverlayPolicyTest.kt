package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.data.ApiError
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.PlaybackControls
import com.aleksclark.primer.tv.core.domain.PlaybackMode
import com.aleksclark.primer.tv.core.domain.PlaybackPolicy
import com.aleksclark.primer.tv.core.playback.PlaybackState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PlaybackOverlayPolicyTest {

    @Test
    fun `educational on-demand gets full interactive transport`() {
        val overlay = PlaybackOverlayPolicy.forPlayback(
            mode = PlaybackMode.ON_DEMAND,
            mediaClass = MediaClass.EDUCATIONAL,
            title = "Inertia",
        )

        assertEquals(PlaybackOverlayKind.FULL_TRANSPORT, overlay.kind)
        assertTrue(overlay.showTransportControls)
        assertTrue(overlay.seekInteractive)
        assertTrue(overlay.pauseAllowed)
        assertFalse(overlay.showLiveBadge)
        assertEquals("Inertia", overlay.title)
    }

    @Test
    fun `mixed on-demand matches educational transport`() {
        val overlay = PlaybackOverlayPolicy.forPlayback(
            mode = PlaybackMode.ON_DEMAND,
            mediaClass = MediaClass.MIXED,
        )
        assertEquals(PlaybackOverlayKind.FULL_TRANSPORT, overlay.kind)
        assertTrue(overlay.seekInteractive)
    }

    @Test
    fun `entertainment keeps pause but non-interactive progress`() {
        val overlay = PlaybackOverlayPolicy.forPlayback(
            mode = PlaybackMode.ON_DEMAND,
            mediaClass = MediaClass.ENTERTAINMENT,
        )

        assertEquals(PlaybackOverlayKind.PAUSE_PROGRESS, overlay.kind)
        assertTrue(overlay.showTransportControls)
        assertTrue(overlay.pauseAllowed)
        assertFalse("seek UI must not be offered for watch-once", overlay.seekInteractive)
        assertFalse(overlay.showLiveBadge)
    }

    @Test
    fun `unknown class is treated like entertainment`() {
        val overlay = PlaybackOverlayPolicy.forPlayback(
            mode = PlaybackMode.ON_DEMAND,
            mediaClass = MediaClass.UNKNOWN,
        )
        assertEquals(PlaybackOverlayKind.PAUSE_PROGRESS, overlay.kind)
        assertFalse(overlay.seekInteractive)
    }

    @Test
    fun `programmed playback is live-minimal regardless of class`() {
        for (mediaClass in MediaClass.entries) {
            val overlay = PlaybackOverlayPolicy.forPlayback(
                mode = PlaybackMode.PROGRAMMED,
                mediaClass = mediaClass,
                title = "On air",
            )
            assertEquals("$mediaClass must stay live-minimal", PlaybackOverlayKind.LIVE_MINIMAL, overlay.kind)
            assertTrue(overlay.showLiveBadge)
            assertFalse(overlay.showTransportControls)
            assertFalse(overlay.seekInteractive)
            assertFalse(overlay.pauseAllowed)
            assertTrue(overlay.followsBroadcast)
        }
    }

    @Test
    fun `overlay kind tracks controls without weakening them`() {
        val rationed = PlaybackControls(pauseAllowed = true, seekAllowed = false)
        val overlay = PlaybackOverlayPolicy.forControls(rationed)
        assertEquals(PlaybackOverlayKind.PAUSE_PROGRESS, overlay.kind)

        val programmed = PlaybackPolicy.PROGRAMMED_CONTROLS
        val live = PlaybackOverlayPolicy.forControls(programmed, title = "Now")
        assertEquals(PlaybackOverlayKind.LIVE_MINIMAL, live.kind)
        assertEquals("Now", live.title)
    }

    @Test
    fun `finished consumed entertainment explains the one viewing`() {
        val message = PlaybackOverlayPolicy.messageFor(
            PlaybackState.Finished(playConsumed = true, completed = true),
        )!!
        assertTrue(message.body.contains("one viewing", ignoreCase = true))
        assertEquals("Back", message.primaryLabel)
        assertFalse(message.canRetry)
        assertFalse(message.isError)
    }

    @Test
    fun `finished without consumption is a plain stop`() {
        val message = PlaybackOverlayPolicy.messageFor(
            PlaybackState.Finished(playConsumed = false, completed = false),
        )!!
        assertEquals("Stopped.", message.body)
        assertFalse(message.isError)
    }

    @Test
    fun `failed grant surfaces error copy`() {
        val message = PlaybackOverlayPolicy.messageFor(
            PlaybackState.Failed(
                error = ApiError.Forbidden("Not available right now."),
                recoverable = false,
            ),
        )!!
        assertTrue(message.isError)
        assertEquals("Not available right now.", message.body)
        assertFalse(message.canRetry)
    }

    @Test
    fun `recoverable failure offers try again`() {
        val message = PlaybackOverlayPolicy.messageFor(
            PlaybackState.Failed(
                error = ApiError.Network("Connection lost"),
                recoverable = true,
            ),
        )!!
        assertTrue(message.canRetry)
        assertEquals("Try again", message.primaryLabel)
    }

    @Test
    fun `starting state is non-error wait copy`() {
        val message = PlaybackOverlayPolicy.messageFor(PlaybackState.RequestingGrant)!!
        assertEquals("Starting…", message.title)
        assertFalse(message.isError)

        val idle = PlaybackOverlayPolicy.messageFor(PlaybackState.Idle)!!
        assertEquals("Starting…", idle.title)
    }
}
