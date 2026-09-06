package com.aleksclark.primer.security

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RecoveryHpkeTest {
    @Test
    fun publicKeyIdIsSha256OfDecodedJsonBytes() {
        val handle = RecoveryHpke.generatePrivateHandle()
        val json = RecoveryHpke.publicKeysetJson(handle)
        val encoded = RecoveryHpke.encodePublicEnrollmentKey(json)
        assertEquals(json.toList(), RecoveryHpke.decodePublicEnrollmentKey(encoded).toList())
        assertEquals(64, RecoveryHpke.keyId(json).length)
        assertTrue(RecoveryHpke.keyId(json).matches(Regex("[0-9a-f]{64}")))
    }

    @Test
    fun contextInfoIsNulSeparatedAndIncludesRequestIdBeforeEncrypt() {
        val info = RecoveryHpke.contextInfo("device-1", "11111111-1111-1111-1111-111111111111", "abcd")
        val parts = String(info, Charsets.UTF_8).split('\u0000')
        assertEquals(
            listOf("primer-management/recovery/v1", "device-1", "11111111-1111-1111-1111-111111111111", "abcd"),
            parts,
        )
    }

    @Test
    fun encryptDecryptRoundTripRequiresMatchingRequestId() {
        val privateHandle = RecoveryHpke.generatePrivateHandle()
        val publicJson = RecoveryHpke.publicKeysetJson(privateHandle)
        val keyId = RecoveryHpke.keyId(publicJson)
        val codes = listOf(
            "00112233-44556677-8899AABB-CCDDEEFF",
            "11223344-55667788-99AABBCC-DDEEFF00",
            "22334455-66778899-AABBCCDD-EEFF0011",
            "33445566-778899AA-BBCCDDEE-FF001122",
            "44556677-8899AABB-CCDDEEFF-00112233",
            "55667788-99AABBCC-DDEEFF00-11223344",
        )
        val requestId = "22222222-2222-2222-2222-222222222222"
        val envelope = RecoveryHpke.encryptCodes(publicJson, "device-1", requestId, codes)
        assertEquals(RecoveryHpke.ALG, envelope.alg)
        assertEquals(keyId, envelope.keyId)
        assertTrue(envelope.ciphertext.isNotBlank())

        val payload = RecoveryHpke.decryptCodes(privateHandle, envelope, "device-1", requestId, keyId)
        assertEquals(codes, payload.codes)
        assertEquals(requestId, payload.requestId)
    }

    @Test(expected = GeneralSecurityOrIllegal::class)
    fun wrongRequestIdDoesNotDecrypt() {
        val privateHandle = RecoveryHpke.generatePrivateHandle()
        val publicJson = RecoveryHpke.publicKeysetJson(privateHandle)
        val envelope = RecoveryHpke.encryptCodes(
            publicJson,
            "device-1",
            "22222222-2222-2222-2222-222222222222",
            listOf("A", "B", "C", "D", "E", "F"),
        )
        try {
            RecoveryHpke.decryptCodes(
                privateHandle,
                envelope,
                "device-1",
                "33333333-3333-3333-3333-333333333333",
                RecoveryHpke.keyId(publicJson),
            )
        } catch (error: Exception) {
            throw GeneralSecurityOrIllegal(error)
        }
    }

    @Test
    fun cleartextPrivateRoundTripIsDistinctFromPublicJson() {
        val privateHandle = RecoveryHpke.generatePrivateHandle()
        val publicJson = RecoveryHpke.publicKeysetJson(privateHandle)
        val privateJson = RecoveryHpke.serializeCleartextPrivate(privateHandle)
        assertNotEquals(publicJson.toList(), privateJson.toList())
        val restored = RecoveryHpke.parseCleartextPrivate(privateJson)
        val envelope = RecoveryHpke.encrypt(publicJson, "secret".toByteArray(), "d", "r")
        assertEquals(
            "secret",
            String(RecoveryHpke.decrypt(restored, envelope, "d", "r", RecoveryHpke.keyId(publicJson))),
        )
    }
}

class GeneralSecurityOrIllegal(cause: Throwable) : RuntimeException(cause)
