package com.aleksclark.primer.student.tasks

import android.content.ContentResolver
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import java.io.InputStream

/**
 * Decodes a user-selected image without retaining the URI, compressed bytes, or
 * decoded bitmap. The bounds pass is important: Photo Picker can return images
 * from outside the app with dimensions much larger than a QR needs.
 */
class QrImageImporter internal constructor(private val source: QrImageSource) {
    constructor(resolver: ContentResolver) : this(QrImageSource { uri -> uri?.let(resolver::openInputStream) })

    fun decode(uri: Uri?): Result = decodeFromSource { source.open(uri) }

    private fun decodeFromSource(open: () -> InputStream?): Result {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        val boundsOpened = runCatching {
            open()?.use { input ->
                BitmapFactory.decodeStream(input, null, bounds)
                true
            } ?: false
        }.getOrDefault(false)

        if (!boundsOpened || bounds.outWidth <= 0 || bounds.outHeight <= 0) {
            return Result.Failure(Failure.UNREADABLE)
        }
        if (bounds.outWidth > DecodePolicy.MAX_SOURCE_DIMENSION ||
            bounds.outHeight > DecodePolicy.MAX_SOURCE_DIMENSION
        ) {
            return Result.Failure(Failure.TOO_LARGE)
        }

        val options = BitmapFactory.Options().apply {
            inSampleSize = DecodePolicy.sampleSize(bounds.outWidth, bounds.outHeight)
            inPreferredConfig = Bitmap.Config.ARGB_8888
            inScaled = false
        }
        val bitmap = runCatching {
            open()?.use { input -> BitmapFactory.decodeStream(input, null, options) }
        }.getOrNull() ?: return Result.Failure(Failure.UNREADABLE)

        if (bitmap.width > DecodePolicy.MAX_DECODED_DIMENSION ||
            bitmap.height > DecodePolicy.MAX_DECODED_DIMENSION ||
            bitmap.width.toLong() * bitmap.height.toLong() > DecodePolicy.MAX_DECODED_PIXELS
        ) {
            bitmap.recycle()
            return Result.Failure(Failure.TOO_LARGE)
        }

        return try {
            val payload = QrFrameDecoder.decode(bitmap)
            if (payload == null) Result.Failure(Failure.NO_QR) else Result.Decoded(payload)
        } catch (_: Exception) {
            Result.Failure(Failure.UNREADABLE)
        } finally {
            // The bitmap is never placed in Compose state or retained by the caller.
            bitmap.recycle()
        }
    }

    internal fun interface QrImageSource {
        fun open(uri: Uri?): InputStream?
    }

    sealed interface Result {
        data class Decoded(val payload: String) : Result
        data class Failure(val reason: QrImageImporter.Failure) : Result
    }

    enum class Failure {
        UNREADABLE,
        TOO_LARGE,
        NO_QR,
    }
}

/** Pure sizing policy so the allocation bound can be covered by JVM tests. */
internal object DecodePolicy {
    const val MAX_SOURCE_DIMENSION = 32_768
    const val MAX_DECODED_DIMENSION = 2_048
    const val MAX_DECODED_PIXELS = 4_194_304L

    fun sampleSize(width: Int, height: Int): Int {
        require(width > 0 && height > 0)
        var sample = 1
        while (decodedWidth(width, sample) > MAX_DECODED_DIMENSION ||
            decodedHeight(height, sample) > MAX_DECODED_DIMENSION ||
            decodedWidth(width, sample).toLong() * decodedHeight(height, sample) > MAX_DECODED_PIXELS
        ) {
            sample = sample shl 1
        }
        return sample
    }

    private fun decodedWidth(width: Int, sample: Int): Int = (width + sample - 1) / sample
    private fun decodedHeight(height: Int, sample: Int): Int = (height + sample - 1) / sample
}
