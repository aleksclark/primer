package com.aleksclark.primertasks

import java.util.zip.GZIPInputStream
import org.junit.Assert.assertEquals
import org.junit.Test

class QrFrameDecoderTest {
    private val expected = "{\"api\":\"/api\",\"code\":\"3332596D4D\",\"exp\":\"2026-08-19T02:43:22Z\",\"origin\":\"http://127.0.0.1:37574\",\"pairingId\":\"172b7dd8-dbf6-48b8-8010-5d0c13bb9a2b\",\"v\":1}"

    @Test
    fun decodesTheExactBrowserRenderedQrAcrossRotationAndStrideMatrix() {
        val browserCrop = loadBrowserCrop()
        for (rotation in listOf(0, 90, 180, 270)) {
            for (padding in listOf(0, 5, 19)) {
                assertEquals(
                    "rotation=$rotation padding=$padding",
                    expected,
                    QrFrameDecoder.decode(frameFor(browserCrop, 400, 400, rotation, padding), rotation),
                )
            }
        }
    }

    @Test
    fun honorsCropOriginAndPixelStrideInsteadOfReadingTheRowAsTightlyPacked() {
        val browserCrop = loadBrowserCrop()
        val frame = frameFor(browserCrop, 400, 400, rotation = 0, padding = 11, cropLeft = 7, cropTop = 3, pixelStride = 5)
        assertEquals(expected, QrFrameDecoder.decode(frame))
    }

    private fun loadBrowserCrop(): ByteArray = javaClass.classLoader!!
        .getResourceAsStream("browser-rendered-qr-crop.rgb.gz")!!
        .let(::GZIPInputStream)
        .use { it.readBytes() }

    private fun frameFor(
        image: ByteArray,
        sourceWidth: Int,
        sourceHeight: Int,
        rotation: Int,
        padding: Int,
        cropLeft: Int = 2,
        cropTop: Int = 4,
        pixelStride: Int = 4,
    ): RgbaFrame {
        val source = rotateCounterClockwise(image, sourceWidth, sourceHeight, rotation)
        val width = if (rotation == 90 || rotation == 270) sourceHeight else sourceWidth
        val height = if (rotation == 90 || rotation == 270) sourceWidth else sourceHeight
        val underlyingWidth = cropLeft + width + 3
        val rowStride = underlyingWidth * pixelStride + padding
        val bytes = ByteArray((cropTop + height + 2) * rowStride) { 0x55 }
        for (y in 0 until height) {
            for (x in 0 until width) {
                val sourceOffset = (y * width + x) * 3
                val destinationOffset = (cropTop + y) * rowStride + (cropLeft + x) * pixelStride
                bytes[destinationOffset] = source[sourceOffset]
                bytes[destinationOffset + 1] = source[sourceOffset + 1]
                bytes[destinationOffset + 2] = source[sourceOffset + 2]
                if (pixelStride >= 4) bytes[destinationOffset + 3] = 0xFF.toByte()
            }
        }
        return RgbaFrame(bytes, width, height, rowStride, pixelStride, cropLeft, cropTop)
    }

    /** Build the camera's counter-clockwise view so rotation metadata must be honored. */
    private fun rotateCounterClockwise(input: ByteArray, width: Int, height: Int, degrees: Int): ByteArray {
        if (degrees == 0) return input
        val outputWidth = height
        val outputHeight = width
        val output = ByteArray(input.size)
        for (y in 0 until outputHeight) {
            for (x in 0 until outputWidth) {
                val sourceX: Int
                val sourceY: Int
                if (degrees == 90) {
                    sourceX = width - 1 - y
                    sourceY = x
                } else if (degrees == 270) {
                    sourceX = y
                    sourceY = height - 1 - x
                } else {
                    sourceX = width - 1 - x
                    sourceY = height - 1 - y
                }
                val sourceOffset = (sourceY * width + sourceX) * 3
                val outputOffset = (y * outputWidth + x) * 3
                input.copyInto(output, outputOffset, sourceOffset, sourceOffset + 3)
            }
        }
        return output
    }
}
