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
        val matrix = PrimerQrMark.matrix(payload)
        val scale = 8
        val width = matrix.width * scale
        val height = matrix.height * scale
        val pixels = IntArray(width * height)
        for (y in 0 until matrix.height) {
            for (x in 0 until matrix.width) {
                val color = if (matrix.get(x, y)) 0xFF000000.toInt() else 0xFFFFFFFF.toInt()
                for (dy in 0 until scale) {
                    val row = (y * scale + dy) * width
                    for (dx in 0 until scale) pixels[row + x * scale + dx] = color
                }
            }
        }
        val source = RGBLuminanceSource(width, height, pixels)
        val hints = mapOf(
            DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
            DecodeHintType.CHARACTER_SET to "UTF-8",
            DecodeHintType.TRY_HARDER to true,
        )
        val decoded = QRCodeReader().decode(BinaryBitmap(HybridBinarizer(source)), hints)
        assertEquals(payload, decoded.text)
    }

    @Test
    fun quietZoneIsAtLeastFourModules() {
        val matrix = PrimerQrMark.matrix("abc")
        assertTrue((0 until matrix.width).all { x -> !matrix.get(x, 0) })
        val firstDark = (0 until matrix.width).indexOfFirst { x -> matrix.get(x, PrimerQrMark.QUIET_ZONE_MODULES) }
        assertTrue(firstDark >= PrimerQrMark.QUIET_ZONE_MODULES)
    }

    @Test
    fun emptyAndOversizedPayloadsAreRejected() {
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) { PrimerQrMark.matrix("") }
        org.junit.Assert.assertThrows(IllegalArgumentException::class.java) {
            PrimerQrMark.matrix("x".repeat(PrimerQrMark.MAX_PAYLOAD_BYTES + 1))
        }
    }

    @Test
    fun encodeFailureMessageIsVisibleToCallers() {
        val error = PrimerQrMark.encodeError("")
        org.junit.Assert.assertTrue(error!!.contains("QR payload"))
        org.junit.Assert.assertNull(PrimerQrMark.encodeError("abc"))
    }
}
