package com.aleksclark.primer.tv.app.player

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.File
import java.util.jar.JarFile
import java.util.zip.ZipFile

/**
 * Guards the vendored Media3 FFmpeg extension AAR.
 *
 * The T9 / RK3318 box has no AC3/EAC3/DTS MediaCodec decoders, so the app
 * ships an LGPL-compatible FFmpeg build as a software fallback. These checks
 * fail the unit-test suite if the artifact is missing, stripped of a required
 * ABI, missing [FfmpegAudioRenderer], missing required decoders, or carrying
 * forbidden extra decoders (e.g. a full Jellyfin GPL artifact rename).
 */
class FfmpegExtensionArtifactTest {

    @Test
    fun `vendored AAR includes arm ABIs FfmpegAudioRenderer and exact decoder scope`() {
        val aar = locateAar()
        assertTrue("AAR must be a non-empty file: ${aar.absolutePath}", aar.isFile && aar.length() > 0)

        ZipFile(aar).use { zip ->
            val names = zip.entries().asSequence().map { it.name }.toSet()

            for (abi in REQUIRED_ABIS) {
                val so = "jni/$abi/libffmpegJNI.so"
                assertTrue("missing $so in ${aar.name}", names.contains(so))
                val entry = zip.getEntry(so)
                    ?: error("missing entry $so")
                assertTrue("$so must not be empty", entry.size > 0)
            }

            val classes = zip.getEntry("classes.jar")
                ?: error("classes.jar missing from ${aar.name}")
            val classBytes = zip.getInputStream(classes).use { it.readBytes() }
            val tmpJar = File.createTempFile("ffmpeg-classes", ".jar").apply {
                writeBytes(classBytes)
                deleteOnExit()
            }
            JarFile(tmpJar).use { jar ->
                val classNames = jar.entries().asSequence().map { it.name }.toSet()
                assertTrue(
                    "FfmpegAudioRenderer.class missing from classes.jar",
                    classNames.contains(FFMPEG_AUDIO_RENDERER_CLASS),
                )
            }

            // Exact decoder scope is compiled into the native library. Require
            // ac3/eac3/dca and reject the common Jellyfin full-set symbols.
            val arm64 = zip.getEntry("jni/arm64-v8a/libffmpegJNI.so")
                ?: error("arm64 lib missing")
            val soBytes = zip.getInputStream(arm64).use { it.readBytes() }
            val haystack = soBytes.toString(Charsets.ISO_8859_1)

            for (symbol in REQUIRED_DECODER_SYMBOLS) {
                assertTrue(
                    "arm64 libffmpegJNI.so missing decoder symbol '$symbol'",
                    haystack.contains(symbol),
                )
            }
            for (symbol in FORBIDDEN_DECODER_SYMBOLS) {
                assertFalse(
                    "arm64 libffmpegJNI.so must not include forbidden decoder '$symbol' " +
                        "(LGPL ac3/eac3/dca-only build; not Jellyfin full set)",
                    haystack.contains(symbol),
                )
            }

            val found = DECODER_SYMBOL_REGEX.findAll(haystack).map { it.value }.toSortedSet()
            assertEquals(
                "arm64 libffmpegJNI.so decoder symbol set must be exactly ac3/eac3/dca",
                REQUIRED_DECODER_SYMBOLS.toSortedSet(),
                found,
            )
        }
    }

    private fun locateAar(): File {
        // tests run with cwd = android/ or android/app/ depending on Gradle task
        val candidates = listOf(
            File("app/libs/$AAR_NAME"),
            File("libs/$AAR_NAME"),
            File("../app/libs/$AAR_NAME"),
        )
        return candidates.firstOrNull { it.isFile }
            ?: run {
                fail(
                    "Missing vendored AAR '$AAR_NAME'. Expected under android/app/libs/. " +
                        "Rebuild with android/third_party/media3-ffmpeg/build-media3-ffmpeg.sh",
                )
                error("unreachable")
            }
    }

    companion object {
        const val AAR_NAME = "media3-ffmpeg-decoder-1.4.1.aar"
        val REQUIRED_ABIS = listOf("arm64-v8a", "armeabi-v7a")
        const val FFMPEG_AUDIO_RENDERER_CLASS =
            "androidx/media3/decoder/ffmpeg/FfmpegAudioRenderer.class"
        // Native symbols emitted by FFmpeg when the decoder is compiled in.
        val REQUIRED_DECODER_SYMBOLS = listOf(
            "ff_ac3_decoder",
            "ff_eac3_decoder",
            "ff_dca_decoder",
        )
        // Minimum reject list for accidental GPL / Jellyfin full-artifact swaps.
        val FORBIDDEN_DECODER_SYMBOLS = listOf(
            "ff_aac_decoder",
            "ff_aac_latm_decoder",
            "ff_alac_decoder",
            "ff_flac_decoder",
            "ff_mp3_decoder",
            "ff_mlp_decoder",
            "ff_truehd_decoder",
            "ff_pcm_alaw_decoder",
            "ff_pcm_mulaw_decoder",
        )
        private val DECODER_SYMBOL_REGEX = Regex("ff_[a-z0-9_]+_decoder")
    }
}
