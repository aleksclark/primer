package com.aleksclark.primer.ui

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.common.BitMatrix
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel

/**
 * Payload-only QR mark. Callers pass opaque bytes-as-text; this component does
 * not parse pairing URLs, enrollment DTOs, or product schemes.
 */
object PrimerQrMark {
    const val QUIET_ZONE_MODULES = 4
    const val MIN_PAYLOAD_BYTES = 1
    const val MAX_PAYLOAD_BYTES = 1024

    fun encodeMatrix(payload: String): BitMatrix {
        val bytes = payload.toByteArray(Charsets.UTF_8)
        require(bytes.size in MIN_PAYLOAD_BYTES..MAX_PAYLOAD_BYTES) {
            "QR payload must be between $MIN_PAYLOAD_BYTES and $MAX_PAYLOAD_BYTES bytes"
        }
        return QRCodeWriter().encode(
            payload,
            BarcodeFormat.QR_CODE,
            0,
            0,
            mapOf(
                EncodeHintType.MARGIN to QUIET_ZONE_MODULES,
                EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M,
                EncodeHintType.CHARACTER_SET to "UTF-8",
            ),
        )
    }

    fun encode(payload: String, modulePx: Int = 8): Bitmap {
        val matrix = encodeMatrix(payload)
        val width = matrix.width
        val height = matrix.height
        val scale = modulePx.coerceAtLeast(1)
        val bitmap = Bitmap.createBitmap(width * scale, height * scale, Bitmap.Config.ARGB_8888)
        for (y in 0 until height) {
            for (x in 0 until width) {
                val color = if (matrix.get(x, y)) 0xFF111111.toInt() else 0xFFFFFFFF.toInt()
                for (dy in 0 until scale) {
                    for (dx in 0 until scale) {
                        bitmap.setPixel(x * scale + dx, y * scale + dy, color)
                    }
                }
            }
        }
        return bitmap
    }
}

@Composable
fun PrimerQrMark(
    payload: String,
    contentDescription: String,
    modifier: Modifier = Modifier,
    size: Dp = 220.dp,
) {
    val bitmap = remember(payload) { runCatching { PrimerQrMark.encode(payload) }.getOrNull() }
    Box(
        modifier
            .background(Color.White)
            .padding((size.value / 16f).dp)
            .semantics { this.contentDescription = contentDescription },
    ) {
        if (bitmap != null) {
            Image(
                bitmap = bitmap.asImageBitmap(),
                contentDescription = null,
                modifier = Modifier.size(size),
                contentScale = ContentScale.FillBounds,
                filterQuality = FilterQuality.None,
            )
        }
    }
}
