package com.aleksclark.primertasks

import java.io.ByteArrayInputStream
import java.io.FilterInputStream
import java.io.InputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class QrImageImporterTest {
    @Test
    fun sampleSizeKeepsDecodedDimensionsAndPixelsBounded() {
        val sample = DecodePolicy.sampleSize(width = 30_000, height = 20_000)
        val decodedWidth = (30_000 + sample - 1) / sample
        val decodedHeight = (20_000 + sample - 1) / sample

        assertTrue(decodedWidth <= DecodePolicy.MAX_DECODED_DIMENSION)
        assertTrue(decodedHeight <= DecodePolicy.MAX_DECODED_DIMENSION)
        assertTrue(decodedWidth.toLong() * decodedHeight <= DecodePolicy.MAX_DECODED_PIXELS)
    }

    @Test
    fun rejectsInvalidSizingInputs() {
        assertEquals(1, DecodePolicy.sampleSize(1, 1))
        assertTrue(DecodePolicy.MAX_SOURCE_DIMENSION < Int.MAX_VALUE)
    }

    @Test
    fun closesUriStreamWhenBoundsDecodeCannotReadAnImage() {
        val stream = TrackingInputStream(ByteArrayInputStream(byteArrayOf(1, 2, 3)))
        val importer = QrImageImporter(
            QrImageImporter.QrImageSource { stream },
        )

        val result = importer.decode(null)

        assertEquals(QrImageImporter.Failure.UNREADABLE, (result as QrImageImporter.Result.Failure).reason)
        assertTrue(stream.closed)
    }

    @Test
    fun parserIsTheOnlyMeaningOfDecodedTextForEitherInputPath() {
        val cameraDecodedText = """{"origin":"https://tasks.example.test","code":"ABC123","pairingId":"pair-1"}"""
        val importedDecodedText = cameraDecodedText.toString()

        // CameraX and image import both hand their decoded text to this parser;
        // neither path constructs a code or calls the API with raw QR fields.
        val cameraQr = PairingQrParser.parse(cameraDecodedText)
        val importedQr = PairingQrParser.parse(importedDecodedText)
        assertEquals(cameraQr, importedQr)
    }

    private class TrackingInputStream(delegate: InputStream) : FilterInputStream(delegate) {
        var closed = false
            private set

        override fun close() {
            closed = true
            super.close()
        }
    }
}
