package com.aleksclark.primer.ui

import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeReader
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PrimerQrMarkTest {
    @Test
    fun encodedMatrixRoundTripsOpaquePayload() {
        val payload = "primer-pairing:opaque-token-not-a-url"
        val matrix = PrimerQrMark.encodeMatrix(payload)
        val scale = 4
        val width = matrix.width * scale
        val height = matrix.height * scale
        val pixels = IntArray(width * height)
        for (y in 0 until matrix.height) {
            for (x in 0 until matrix.width) {
                val color = if (matrix.get(x, y)) 0xFF111111.toInt() else 0xFFFFFFFF.toInt()
                for (dy in 0 until scale) {
                    for (dx in 0 until scale) {
                        pixels[(y * scale + dy) * width + (x * scale + dx)] = color
                    }
                }
            }
        }
        val source = RGBLuminanceSource(width, height, pixels)
        val hints = mapOf(
            DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
            DecodeHintType.CHARACTER_SET to "UTF-8",
        )
        val decoded = QRCodeReader().decode(BinaryBitmap(HybridBinarizer(source)), hints)
        assertEquals(payload, decoded.text)
    }

    @Test
    fun quietZoneIsAtLeastFourModules() {
        val matrix = PrimerQrMark.encodeMatrix("abc")
        val edge = (0 until matrix.width).count { x -> !matrix.get(x, 0) }
        assertTrue("quiet zone should occupy the first row", edge == matrix.width)
        val inset = (0 until matrix.width).indexOfFirst { x -> matrix.get(x, PrimerQrMark.QUIET_ZONE_MODULES) }
        assertTrue(inset >= PrimerQrMark.QUIET_ZONE_MODULES)
    }

    @Test
    fun emptyAndOversizedPayloadsAreRejected() {
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) { PrimerQrMark.encodeMatrix("") }
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) {
            PrimerQrMark.encodeMatrix("x".repeat(PrimerQrMark.MAX_PAYLOAD_BYTES + 1))
        }
    }
}
