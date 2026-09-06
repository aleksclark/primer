package com.aleksclark.primer.student.management

import com.aleksclark.primer.updates.GoSignedReleaseVectors
import com.aleksclark.primertasks.client.ReleaseTarget
import java.util.Base64
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ReleaseDeliveryTest {
    private val trust = GoSignedReleaseVectors.TRUST_ROOT_BASE64URL

    private fun target(
        payloadBase64: String,
        signature: String,
        packageName: String = "com.aleksclark.primer.student",
        versionName: String = "0.2.0",
    ): ReleaseTarget = ReleaseTarget(
        byteSize = 12,
        channel = "stable",
        id = "11111111-1111-1111-1111-111111111111",
        manifestPayloadBase64 = payloadBase64,
        manifestSignature = signature,
        minSdk = 28,
        packageName = packageName,
        releaseId = "22222222-2222-2222-2222-222222222222",
        sha256 = GoSignedReleaseVectors.SHA256,
        signerSha256 = GoSignedReleaseVectors.SIGNER_SHA256,
        signingKeyId = GoSignedReleaseVectors.SIGNING_KEY_ID,
        status = "queued",
        targetVersion = 1,
        versionCode = 2,
        versionName = versionName,
    )

    @Test
    fun verifiesExactGoOrderedBytesIncludingUnicode() {
        val versionName = "Primer \"N+1\" 测试"
        val manifest = ReleaseDelivery.verify(
            target(
                payloadBase64 = GoSignedReleaseVectors.STUDENT_UNICODE_PAYLOAD_BASE64URL,
                signature = GoSignedReleaseVectors.STUDENT_UNICODE_SIGNATURE_BASE64URL,
                versionName = versionName,
            ),
            trust,
        )
        assertEquals(versionName, manifest.versionName)
        assertEquals("stable", manifest.channel)
        assertEquals(28, manifest.minSdk)
        assertEquals(listOf("arm64-v8a"), manifest.supportedAbis)
    }

    @Test
    fun tamperedPayloadFailsClosed() {
        val payload = GoSignedReleaseVectors.STUDENT_PLAIN_PAYLOAD.toByteArray(Charsets.UTF_8)
        val tampered = target(
            payloadBase64 = Base64.getUrlEncoder().withoutPadding().encodeToString(payload + 1),
            signature = GoSignedReleaseVectors.STUDENT_PLAIN_SIGNATURE_BASE64URL,
        )
        org.junit.Assert.assertThrows(IllegalStateException::class.java) { ReleaseDelivery.verify(tampered, trust) }
    }

    @Test
    fun currentVersionShortcutStillRequiresTrust() {
        val verified = ReleaseDelivery.verify(
            target(
                payloadBase64 = GoSignedReleaseVectors.STUDENT_OTHER_PAYLOAD_BASE64URL,
                signature = GoSignedReleaseVectors.STUDENT_OTHER_SIGNATURE_BASE64URL,
                packageName = "com.other",
            ),
            trust,
        )
        assertEquals("com.other", verified.packageName)
        assertTrue(verified.packageName != ReleaseDelivery.STUDENT_PACKAGE)
    }

    @Test
    fun minSdkOutOfIntRangeFailsClosed() {
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ReleaseDelivery.verify(
                target(
                    payloadBase64 = GoSignedReleaseVectors.STUDENT_MINSDK_OVERFLOW_PAYLOAD_BASE64URL,
                    signature = GoSignedReleaseVectors.STUDENT_MINSDK_OVERFLOW_SIGNATURE_BASE64URL,
                ),
                trust,
            )
        }
    }
}
