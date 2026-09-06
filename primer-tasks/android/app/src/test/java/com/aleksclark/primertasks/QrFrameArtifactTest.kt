package com.aleksclark.primertasks

import java.security.MessageDigest
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class QrFrameArtifactTest {
    @Test
    fun packsOnlyRedactedMetadataWithoutFrameBytes() {
        val firstBytes = byteArrayOf(0x01, 0x23, 0x45, 0x67)
        val artifact = QrFrameArtifact.json(
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

        assertTrue(artifact.contains("\"redacted\":true"))
        assertTrue(artifact.contains("\"imageWidth\":640"))
        assertTrue(artifact.contains("\"imageHeight\":480"))
        assertTrue(artifact.contains("\"cropRect\":{\"left\":3,\"top\":5,\"right\":15,\"bottom\":13}"))
        assertTrue(artifact.contains("\"rotationDegrees\":90"))
        assertTrue(artifact.contains("\"rowStride\":52"))
        assertTrue(artifact.contains("\"pixelStride\":4"))
        assertTrue(artifact.contains("\"byteLength\":4"))
        assertTrue(artifact.contains("\"sha256\":\"${firstBytes.sha256()}\""))
        assertFalse(artifact.contains("raw"))
        assertFalse(artifact.contains("sample"))
        assertFalse(artifact.contains("01234567"))
    }

    private fun ByteArray.sha256(): String = MessageDigest.getInstance("SHA-256")
        .digest(this)
        .joinToString(separator = "") { "%02x".format(it.toInt() and 0xff) }
}
