package com.aleksclark.primer.tv.app.player

/**
 * ExoPlayer wiring constants and helpers that must stay unit-testable without
 * an Android device. [PlayerHost] consumes these when building the player.
 */
object PlayerConfiguration {
    /**
     * Prefer platform MediaCodec (AAC/MP3 hardware) and fall back to the
     * Media3 FFmpeg extension for AC3/EAC3/DTS. Value matches
     * [androidx.media3.exoplayer.DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON].
     */
    const val EXTENSION_RENDERER_MODE: Int = 1 // EXTENSION_RENDERER_MODE_ON

    /** Compact one-line summary for logcat when ExoPlayer fails. */
    fun formatPlayerError(
        errorCode: Int,
        errorCodeName: String,
        message: String?,
        cause: String?,
    ): String {
        val parts = buildList {
            add("ExoPlayer error")
            add("code=$errorCode")
            add("name=$errorCodeName")
            if (!message.isNullOrBlank()) add("message=$message")
            if (!cause.isNullOrBlank()) add("cause=$cause")
        }
        return parts.joinToString(" ")
    }
}
