package com.aleksclark.primer.tv.app.player

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
 * ABI, missing [FfmpegAudioRenderer], or built without the required decoders.
 */
class FfmpegExtensionArtifactTest {

    @Test
    fun `vendored AAR includes arm ABIs FfmpegAudioRenderer and required decoders`() {
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

            // Decoder presence is compiled into the native library; scan the
            // arm64 binary for the FFmpeg decoder symbols the box needs.
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
    }
}
