package com.aleksclark.primer.tv.app.player

import androidx.media3.exoplayer.DefaultRenderersFactory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit-level contract for ExoPlayer wiring.
 *
 * Platform MediaCodec stays preferred (EXTENSION_RENDERER_MODE_ON, not PREFER)
 * so AAC/MP3 keep hardware paths; FFmpeg only covers formats the SoC lacks.
 */
class PlayerConfigurationTest {

    @Test
    fun `extension renderer mode is ON so FFmpeg is fallback not preferred`() {
        assertEquals(
            DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON,
            PlayerConfiguration.EXTENSION_RENDERER_MODE,
        )
        // Guard against accidentally flipping to PREFER or OFF via a wrong constant.
        assertTrue(PlayerConfiguration.EXTENSION_RENDERER_MODE == 1)
        assertTrue(
            "ON must differ from OFF and PREFER",
            PlayerConfiguration.EXTENSION_RENDERER_MODE !=
                DefaultRenderersFactory.EXTENSION_RENDERER_MODE_OFF &&
                PlayerConfiguration.EXTENSION_RENDERER_MODE !=
                DefaultRenderersFactory.EXTENSION_RENDERER_MODE_PREFER,
        )
    }

    @Test
    fun `player error log includes error code type and message`() {
        val formatted = PlayerConfiguration.formatPlayerError(
            errorCode = 4003,
            errorCodeName = "ERROR_CODE_DECODING_FAILED",
            message = "Audio codec audio/ac3 not supported",
            cause = "FfmpegDecoderException: init failed",
        )
        assertTrue(formatted.contains("4003"))
        assertTrue(formatted.contains("ERROR_CODE_DECODING_FAILED"))
        assertTrue(formatted.contains("audio/ac3"))
        assertTrue(formatted.contains("FfmpegDecoderException"))
        assertTrue(formatted.startsWith("ExoPlayer error"))
    }
}
