package com.aleksclark.primer.student.tasks

import android.graphics.Bitmap
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.GlobalHistogramBinarizer
import com.google.zxing.common.HybridBinarizer
import java.util.EnumMap

/**
 * The packed frame handed to the QR decoder. [rowStride] and [pixelStride] are
 * deliberately part of this type: CameraX RGBA frames commonly have padding
 * at the end of every row and must not be treated as a tightly packed bitmap.
 *
 * [cropLeft]/[cropTop] are coordinates in the underlying CameraX buffer. The
 * width and height describe the crop that should be decoded.
 */
data class RgbaFrame(
    val bytes: ByteArray,
    val width: Int,
    val height: Int,
    val rowStride: Int,
    val pixelStride: Int = 4,
    val cropLeft: Int = 0,
    val cropTop: Int = 0,
)

/** Pure, Android-free seam for exercising the CameraX-to-ZXing conversion. */
object QrFrameDecoder {
    private val hints = EnumMap<DecodeHintType, Any>(DecodeHintType::class.java).apply {
        put(DecodeHintType.POSSIBLE_FORMATS, listOf(BarcodeFormat.QR_CODE))
        put(DecodeHintType.TRY_HARDER, true)
        put(DecodeHintType.ALSO_INVERTED, true)
    }

    /** Returns the QR text, or null when this frame does not contain a QR code. */
    fun decode(frame: RgbaFrame, rotationDegrees: Int = 0): String? {
        val normalizedRotation = ((rotationDegrees % 360) + 360) % 360
        return decodeRgb(
            rgb = frame.toRotatedRgb(normalizedRotation),
            width = frame.width,
            height = frame.height,
            rotationDegrees = normalizedRotation,
        )
    }

    /** Decodes a bounded Photo Picker bitmap with the same ZXing reader as CameraX. */
    fun decode(bitmap: Bitmap): String? {
        val width = bitmap.width
        val height = bitmap.height
        require(width > 0 && height > 0)
        require(width.toLong() * height.toLong() <= DecodePolicy.MAX_DECODED_PIXELS) {
            "Bitmap exceeds the QR decode bound"
        }
        val pixels = IntArray(width * height)
        return try {
            bitmap.getPixels(pixels, 0, width, 0, 0, width, height)
            decodeRgb(pixels, width, height, rotationDegrees = 0)
        } finally {
            // Do not leave image pixels in a heap buffer after decoding.
            pixels.fill(0)
        }
    }

    private fun decodeRgb(
        rgb: IntArray,
        width: Int,
        height: Int,
        rotationDegrees: Int,
    ): String? {
        val (decodedWidth, decodedHeight) = rotatedSize(width, height, rotationDegrees)
        val source = RGBLuminanceSource(decodedWidth, decodedHeight, rgb)
        val reader = MultiFormatReader().apply { setHints(hints) }
        val bitmap = BinaryBitmap(HybridBinarizer(source))
        return runCatching { reader.decode(bitmap).text }
            .recoverCatching {
                reader.reset()
                reader.decode(BinaryBitmap(GlobalHistogramBinarizer(source))).text
            }
            .getOrNull()
    }

    private fun RgbaFrame.toRotatedRgb(rotationDegrees: Int): IntArray {
        require(width > 0 && height > 0) { "Frame dimensions must be positive" }
        require(rowStride > 0 && pixelStride >= 3) { "Invalid RGBA stride" }
        val normalized = ((rotationDegrees % 360) + 360) % 360
        require(normalized == 0 || normalized == 90 || normalized == 180 || normalized == 270) {
            "Rotation must be 0, 90, 180, or 270 degrees"
        }
        val (outputWidth, outputHeight) = rotatedSize(width, height, normalized)
        val rgb = IntArray(outputWidth * outputHeight)
        for (outputY in 0 until outputHeight) {
            for (outputX in 0 until outputWidth) {
                val (inputX, inputY) = sourceCoordinates(outputX, outputY, width, height, normalized)
                val sourceOffset = (cropTop + inputY) * rowStride + (cropLeft + inputX) * pixelStride
                require(sourceOffset >= 0 && sourceOffset + 2 < bytes.size) { "RGBA frame is shorter than its stride" }
                val red = bytes[sourceOffset].toInt() and 0xff
                val green = bytes[sourceOffset + 1].toInt() and 0xff
                val blue = bytes[sourceOffset + 2].toInt() and 0xff
                rgb[outputY * outputWidth + outputX] = (red shl 16) or (green shl 8) or blue
            }
        }
        return rgb
    }

    private fun rotatedSize(width: Int, height: Int, degrees: Int): Pair<Int, Int> =
        if (degrees == 90 || degrees == 270) height to width else width to height

    /** Maps a pixel in the rotated output back to the unrotated input. */
    private fun sourceCoordinates(outputX: Int, outputY: Int, width: Int, height: Int, degrees: Int): Pair<Int, Int> = when (degrees) {
        90 -> outputY to (height - 1 - outputX)
        180 -> (width - 1 - outputX) to (height - 1 - outputY)
        270 -> (width - 1 - outputY) to outputX
        else -> outputX to outputY
    }
}
