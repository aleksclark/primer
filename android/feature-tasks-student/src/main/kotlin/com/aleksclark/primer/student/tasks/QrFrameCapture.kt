package com.aleksclark.primer.student.tasks

import android.content.Context
import java.io.File
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.util.concurrent.atomic.AtomicBoolean

/**
 * A deliberately narrow seam around the first real CameraX analysis frame.
 * Implementations must not decode or persist the frame payload.
 */
internal fun interface QrFrameCapture {
    fun capture(frame: RgbaFrame, imageWidth: Int, imageHeight: Int, rotationDegrees: Int)
}

internal object NoOpQrFrameCapture : QrFrameCapture {
    override fun capture(frame: RgbaFrame, imageWidth: Int, imageHeight: Int, rotationDegrees: Int) = Unit
}

/**
 * Writes one redacted diagnostic artifact for a DEBUG build.
 *
 * The artifact contains only frame metadata, the complete plane byte length, and a SHA-256
 * digest. It intentionally contains neither RGBA bytes nor a sample: the QR in the frame can
 * contain a one-use pairing code and must not be copied to disk. A failed write is not retried,
 * so this hook can never create more than one artifact per analyzer instance.
 *
 * [outputFile] is caller-supplied so acceptance can choose a pullable debug location. The app
 * supplies its app-specific external files directory; this class is inert in non-DEBUG builds.
 */
internal class DebugQrFrameCapture(private val outputFile: File) : QrFrameCapture {
    private val captured = AtomicBoolean(false)

    override fun capture(frame: RgbaFrame, imageWidth: Int, imageHeight: Int, rotationDegrees: Int) {
        if (!BuildConfig.DEBUG || !captured.compareAndSet(false, true)) return

        // Keep diagnostics best-effort and isolated from image analysis/pairing behavior.
        runCatching {
            outputFile.parentFile?.mkdirs()
            outputFile.outputStream().bufferedWriter(StandardCharsets.UTF_8).use { writer ->
                writer.write(
                    QrFrameArtifact.json(
                        frame = frame,
                        imageWidth = imageWidth,
                        imageHeight = imageHeight,
                        rotationDegrees = rotationDegrees,
                    ),
                )
                writer.newLine()
            }
        }
    }
}

internal object QrFrameCaptureFactory {
    private const val artifactName = "primer-qr-frame-debug.json"

    /** Release builds never resolve an output path or construct the file-writing hook. */
    fun forContext(context: Context): QrFrameCapture {
        if (!BuildConfig.DEBUG) return NoOpQrFrameCapture
        val outputFile = context.getExternalFilesDir(null)?.resolve(artifactName) ?: return NoOpQrFrameCapture
        return DebugQrFrameCapture(outputFile)
    }
}

/** Pure redacted artifact packing; no camera or Android APIs are involved here. */
internal object QrFrameArtifact {
    fun json(frame: RgbaFrame, imageWidth: Int, imageHeight: Int, rotationDegrees: Int): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(frame.bytes).toHex()
        return buildString {
            append("{\"version\":1")
            append(",\"redacted\":true")
            append(",\"imageWidth\":").append(imageWidth)
            append(",\"imageHeight\":").append(imageHeight)
            append(",\"cropRect\":{")
            append("\"left\":").append(frame.cropLeft)
            append(",\"top\":").append(frame.cropTop)
            append(",\"right\":").append(frame.cropLeft + frame.width)
            append(",\"bottom\":").append(frame.cropTop + frame.height)
            append("}")
            append(",\"cropWidth\":").append(frame.width)
            append(",\"cropHeight\":").append(frame.height)
            append(",\"rotationDegrees\":").append(rotationDegrees)
            append(",\"rowStride\":").append(frame.rowStride)
            append(",\"pixelStride\":").append(frame.pixelStride)
            append(",\"byteLength\":").append(frame.bytes.size)
            append(",\"sha256\":\"").append(digest).append("\"}")
        }
    }

    private fun ByteArray.toHex(): String = joinToString(separator = "") { "%02x".format(it.toInt() and 0xff) }
}
