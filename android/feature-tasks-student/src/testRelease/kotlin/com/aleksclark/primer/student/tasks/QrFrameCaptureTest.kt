package com.aleksclark.primer.student.tasks

import java.io.File
import java.nio.file.Files
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class QrFrameCaptureTest {
    @Test
    fun releaseCaptureIsInertAndNeverWritesAnArtifact() {
        assertFalse(BuildConfig.DEBUG)
        val directory = Files.createTempDirectory("primer-qr-capture-release").toFile()
        try {
            val output = File(directory, "capture.json")
            val frame = RgbaFrame(
                bytes = byteArrayOf(0x01, 0x23, 0x45, 0x67),
                width = 12,
                height = 8,
                rowStride = 52,
                pixelStride = 4,
                cropLeft = 3,
                cropTop = 5,
            )
            DebugQrFrameCapture(output).capture(
                frame = frame,
                imageWidth = 640,
                imageHeight = 480,
                rotationDegrees = 90,
            )
            DebugQrFrameCapture(output).capture(
                frame = RgbaFrame(byteArrayOf(0x7f), width = 1, height = 1, rowStride = 4),
                imageWidth = 1,
                imageHeight = 1,
                rotationDegrees = 0,
            )
            NoOpQrFrameCapture.capture(
                frame = frame,
                imageWidth = 640,
                imageHeight = 480,
                rotationDegrees = 90,
            )

            assertFalse(output.exists())
            assertEquals(0, directory.listFiles()?.size ?: 0)
        } finally {
            directory.deleteRecursively()
        }
    }
}
