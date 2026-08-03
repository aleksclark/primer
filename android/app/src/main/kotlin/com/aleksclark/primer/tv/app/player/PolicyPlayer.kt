package com.aleksclark.primer.tv.app.player

import androidx.media3.common.ForwardingPlayer
import androidx.media3.common.Player
import com.aleksclark.primer.tv.core.domain.PlaybackControls

/**
 * A player that enforces a [PlaybackControls] policy.
 *
 * Withdrawing the commands, rather than merely hiding buttons, is what makes
 * the rule stick: the transport controls, the D-pad, and any media session all
 * read the player's advertised command set, so nothing in the UI can route
 * around it. The overrides are belt and braces for callers that hold the player
 * directly and never consult the command set.
 *
 * Seeking is withheld from entertainment because it is rationed by the server's
 * watch-once ledger, and scrubbing to the final minute would spend the single
 * viewing without watching it. Both seeking *and* pausing are withheld from the
 * programmed channel: a pause that resumed where it left off would leave the
 * student behind the broadcast, watching a scene the server says already aired.
 */
class PolicyPlayer(
    player: Player,
    private val controls: PlaybackControls,
) : ForwardingPlayer(player) {

    /** The commands this policy withdraws. */
    private val blocked: IntArray = buildList {
        if (!controls.seekAllowed) addAll(SEEK_COMMANDS.toList())
        if (!controls.pauseAllowed) add(Player.COMMAND_PLAY_PAUSE)
    }.toIntArray()

    override fun getAvailableCommands(): Player.Commands =
        super.getAvailableCommands().buildUpon().removeAll(*blocked).build()

    override fun isCommandAvailable(command: Int): Boolean =
        !blocks(command) && super.isCommandAvailable(command)

    override fun setPlayWhenReady(playWhenReady: Boolean) {
        // A programmed stream must never stop advancing: it is a broadcast, and
        // the position it resumed at would no longer be the one on air.
        if (!playWhenReady && !controls.pauseAllowed) return
        super.setPlayWhenReady(playWhenReady)
    }

    override fun pause() {
        if (!controls.pauseAllowed) return
        super.pause()
    }

    override fun seekTo(positionMs: Long) {
        if (!controls.seekAllowed) return
        super.seekTo(positionMs)
    }

    override fun seekTo(mediaItemIndex: Int, positionMs: Long) {
        if (!controls.seekAllowed) return
        super.seekTo(mediaItemIndex, positionMs)
    }

    override fun seekToDefaultPosition() {
        if (!controls.seekAllowed) return
        super.seekToDefaultPosition()
    }

    override fun seekToDefaultPosition(mediaItemIndex: Int) {
        if (!controls.seekAllowed) return
        super.seekToDefaultPosition(mediaItemIndex)
    }

    override fun seekForward() {
        if (!controls.seekAllowed) return
        super.seekForward()
    }

    override fun seekBack() {
        if (!controls.seekAllowed) return
        super.seekBack()
    }

    override fun seekToNext() {
        if (!controls.seekAllowed) return
        super.seekToNext()
    }

    override fun seekToPrevious() {
        if (!controls.seekAllowed) return
        super.seekToPrevious()
    }

    override fun seekToNextMediaItem() {
        if (!controls.seekAllowed) return
        super.seekToNextMediaItem()
    }

    override fun seekToPreviousMediaItem() {
        if (!controls.seekAllowed) return
        super.seekToPreviousMediaItem()
    }

    @Deprecated("Deprecated in Media3")
    override fun seekToNextWindow() {
        if (!controls.seekAllowed) return
        @Suppress("DEPRECATION")
        super.seekToNextWindow()
    }

    @Deprecated("Deprecated in Media3")
    override fun seekToPreviousWindow() {
        if (!controls.seekAllowed) return
        @Suppress("DEPRECATION")
        super.seekToPreviousWindow()
    }

    private fun blocks(command: Int): Boolean = blocked.any { it == command }

    private companion object {
        /**
         * Every Media3 seek command. Withdrawing only a subset leaves D-pad /
         * media-session / controller paths that can still scrub entertainment
         * or programmed playback.
         */
        val SEEK_COMMANDS = intArrayOf(
            Player.COMMAND_SEEK_TO_DEFAULT_POSITION,
            Player.COMMAND_SEEK_IN_CURRENT_MEDIA_ITEM,
            Player.COMMAND_SEEK_TO_PREVIOUS_MEDIA_ITEM,
            Player.COMMAND_SEEK_TO_PREVIOUS,
            Player.COMMAND_SEEK_TO_NEXT_MEDIA_ITEM,
            Player.COMMAND_SEEK_TO_NEXT,
            Player.COMMAND_SEEK_TO_MEDIA_ITEM,
            Player.COMMAND_SEEK_BACK,
            Player.COMMAND_SEEK_FORWARD,
        )
    }
}
