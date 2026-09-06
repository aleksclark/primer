package com.aleksclark.primer.student.management

import com.aleksclark.primer.updates.ReleaseTrust
import com.aleksclark.primertasks.client.ReleaseTarget
import java.util.Base64
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ReleaseDeliveryTest {
    private val key = ByteArray(32) { it.toByte() }
    private val trust = Base64.getUrlEncoder().withoutPadding().encodeToString(key)

    private fun target(payload: ByteArray, versionName: String = "0.2.0"): ReleaseTarget {
        val signature = ReleaseTrust.testSignature(key, payload)
        return ReleaseTarget(
            byteSize = 12,
            channel = "stable",
            id = "11111111-1111-1111-1111-111111111111",
            manifestPayloadBase64 = Base64.getUrlEncoder().withoutPadding().encodeToString(payload),
            manifestSignature = signature,
            minSdk = 28,
            packageName = "com.aleksclark.primer.student",
            releaseId = "22222222-2222-2222-2222-222222222222",
            sha256 = "b".repeat(64),
            signerSha256 = "a".repeat(64),
            signingKeyId = "ed25519-v1",
            status = "queued",
            targetVersion = 1,
            versionCode = 2,
            versionName = versionName,
        )
    }

    @Test
    fun verifiesExactGoOrderedBytesIncludingUnicode() {
        val versionName = "Primer \"N+1\" 测试"
        val payload = """{"packageName":"com.aleksclark.primer.student","channel":"stable","versionCode":2,"versionName":"Primer \"N+1\" 测试","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"${"a".repeat(64)}","sha256":"${"b".repeat(64)}","byteSize":12}"""
            .toByteArray(Charsets.UTF_8)
        val manifest = ReleaseDelivery.verify(target(payload, versionName), trust)
        assertEquals(versionName, manifest.versionName)
        assertEquals("stable", manifest.channel)
        assertEquals(28, manifest.minSdk)
        assertEquals(listOf("arm64-v8a"), manifest.supportedAbis)
    }

    @Test
    fun tamperedPayloadFailsClosed() {
        val payload = """{"packageName":"com.aleksclark.primer.student","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"${"a".repeat(64)}","sha256":"${"b".repeat(64)}","byteSize":12}"""
            .toByteArray(Charsets.UTF_8)
        val signed = target(payload)
        val tampered = signed.copy(
            manifestPayloadBase64 = Base64.getUrlEncoder().withoutPadding().encodeToString(payload + 1),
        )
        org.junit.Assert.assertThrows(IllegalStateException::class.java) { ReleaseDelivery.verify(tampered, trust) }
    }

    @Test
    fun currentVersionShortcutStillRequiresTrust() {
        val payload = """{"packageName":"com.other","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"${"a".repeat(64)}","sha256":"${"b".repeat(64)}","byteSize":12}"""
            .toByteArray(Charsets.UTF_8)
        val verified = ReleaseDelivery.verify(target(payload).copy(packageName = "com.other"), trust)
        assertEquals("com.other", verified.packageName)
        assertTrue(verified.packageName != ReleaseDelivery.STUDENT_PACKAGE)
    }

    @Test
    fun minSdkOutOfIntRangeFailsClosed() {
        val payload = """{"packageName":"com.aleksclark.primer.student","channel":"stable","versionCode":2,"versionName":"0.2.0","minSdk":${Int.MAX_VALUE.toLong() + 1},"supportedAbis":["arm64-v8a"],"signerSha256":"${"a".repeat(64)}","sha256":"${"b".repeat(64)}","byteSize":12}"""
            .toByteArray(Charsets.UTF_8)
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ReleaseDelivery.verify(target(payload), trust)
        }
        assertTrue(true)
    }
}
