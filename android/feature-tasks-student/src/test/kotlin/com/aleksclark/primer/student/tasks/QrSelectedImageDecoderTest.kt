package com.aleksclark.primer.student.tasks

import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel
import java.util.zip.GZIPInputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class QrSelectedImageDecoderTest {
    // 176-character pairing JSON matches the live parent web QR density.
    private val densePayload =
        """{"v":1,"api":"/api","origin":"https://tasks.example.test","code":"ABC123ABC123ABC123ABC123ABC123AB","pairingId":"11111111-1111-1111-1111-11111111","exp":"2026-09-06T12:00:00Z"}"""

    @Test
    fun selectedImageDecodesDenseRenderedPairingQr() {
        val (rgb, width, height) = renderDenseQr(densePayload, size = 240, margin = 1)
        val decoded = QrFrameDecoder.decodeSelectedRgb(rgb, width, height)
        assertEquals(densePayload, decoded)
        assertEquals(176, densePayload.length)
    }

    @Test
    fun cameraPathDoesNotUsePureBarcodeFallback() {
        val (rgb, width, height) = renderDenseQr(densePayload, size = 240, margin = 1)
        val frame = rgbToFrame(rgb, width, height)
        val cameraDecoded = QrFrameDecoder.decode(frame)
        val selectedDecoded = QrFrameDecoder.decodeSelectedRgb(rgb, width, height)
        assertEquals(densePayload, selectedDecoded)
        if (cameraDecoded != null) {
            assertEquals(densePayload, cameraDecoded)
        }
    }

    @Test
    fun selectedImageStillDecodesTheBrowserRenderedCrop() {
        val crop = javaClass.classLoader!!
            .getResourceAsStream("browser-rendered-qr-crop.rgb.gz")!!
            .let(::GZIPInputStream)
            .use { it.readBytes() }
        val width = 400
        val height = 400
        val rgb = IntArray(width * height)
        for (i in rgb.indices) {
            val offset = i * 3
            rgb[i] = ((crop[offset].toInt() and 0xff) shl 16) or
                ((crop[offset + 1].toInt() and 0xff) shl 8) or
                (crop[offset + 2].toInt() and 0xff)
        }
        val expected = "{\"api\":\"/api\",\"code\":\"3332596D4D\",\"exp\":\"2026-08-19T02:43:22Z\",\"origin\":\"http://127.0.0.1:37574\",\"pairingId\":\"172b7dd8-dbf6-48b8-8010-5d0c13bb9a2b\",\"v\":1}"
        assertEquals(expected, QrFrameDecoder.decodeSelectedRgb(rgb, width, height))
    }

    @Test
    fun emptyLumaIsNotAQr() {
        val rgb = IntArray(32 * 32) { 0xFFFFFF }
        assertNull(QrFrameDecoder.decodeSelectedRgb(rgb, 32, 32))
    }

    private fun renderDenseQr(payload: String, size: Int, margin: Int): Triple<IntArray, Int, Int> {
        val matrix = QRCodeWriter().encode(
            payload,
            BarcodeFormat.QR_CODE,
            size,
            size,
            mapOf(
                EncodeHintType.MARGIN to margin,
                EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M,
                EncodeHintType.CHARACTER_SET to "UTF-8",
            ),
        )
        val rgb = IntArray(matrix.width * matrix.height)
        for (y in 0 until matrix.height) {
            for (x in 0 until matrix.width) {
                rgb[y * matrix.width + x] = if (matrix.get(x, y)) 0x000000 else 0xFFFFFF
            }
        }
        assertNotNull(matrix)
        return Triple(rgb, matrix.width, matrix.height)
    }

    private fun rgbToFrame(rgb: IntArray, width: Int, height: Int): RgbaFrame {
        val bytes = ByteArray(width * height * 4)
        for (i in rgb.indices) {
            val pixel = rgb[i]
            val offset = i * 4
            bytes[offset] = ((pixel shr 16) and 0xff).toByte()
            bytes[offset + 1] = ((pixel shr 8) and 0xff).toByte()
            bytes[offset + 2] = (pixel and 0xff).toByte()
            bytes[offset + 3] = 0xFF.toByte()
        }
        return RgbaFrame(bytes, width, height, width * 4, 4, 0, 0)
    }
}
