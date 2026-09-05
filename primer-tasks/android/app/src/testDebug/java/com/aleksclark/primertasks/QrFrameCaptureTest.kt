package com.aleksclark.primertasks

import java.io.File
import java.nio.file.Files
import java.security.MessageDigest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class QrFrameCaptureTest {
    @Test
    fun debugCaptureWritesOneRedactedArtifactAndIgnoresLaterFrames() {
        assertTrue(BuildConfig.DEBUG)
        val directory = Files.createTempDirectory("primer-qr-capture-debug").toFile()
        try {
            val output = File(directory, "capture.json")
            val capture = DebugQrFrameCapture(output)
            val firstBytes = byteArrayOf(0x01, 0x23, 0x45, 0x67)
            capture.capture(
                frame = RgbaFrame(
                    bytes = firstBytes,
                    width = 12,
                    height = 8,
                    rowStride = 52,
                    pixelStride = 4,
                    cropLeft = 3,
                    cropTop = 5,
                ),
                imageWidth = 640,
                imageHeight = 480,
                rotationDegrees = 90,
            )
            capture.capture(
                frame = RgbaFrame(byteArrayOf(0x7f), width = 1, height = 1, rowStride = 4),
                imageWidth = 1,
                imageHeight = 1,
                rotationDegrees = 0,
            )

            val artifact = output.readText()
            assertEquals(1, directory.listFiles()!!.size)
            assertTrue(artifact.contains("\"redacted\":true"))
            assertTrue(artifact.contains("\"imageWidth\":640"))
            assertTrue(artifact.contains("\"sha256\":\"${firstBytes.sha256()}\""))
            assertFalse(artifact.contains("raw"))
            assertFalse(artifact.contains("sample"))
            assertFalse(artifact.contains("01234567"))
        } finally {
            directory.deleteRecursively()
        }
    }

    private fun ByteArray.sha256(): String = MessageDigest.getInstance("SHA-256")
        .digest(this)
        .joinToString(separator = "") { "%02x".format(it.toInt() and 0xff) }
}
