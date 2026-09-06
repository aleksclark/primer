package com.aleksclark.primer.student.management

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class EnrollmentQrTest {
    @Test
    fun parsesTrustedMountedEnrollment() {
        val qr = ManagementEnrollmentQrParser.parse(
            "primer-management:v1:https://tasks.example.test/tasks/management-device/enroll#00112233445566778899AABBCCDDEEFF",
            configuredHttpsOrigin = "https://tasks.example.test/tasks/api",
            allowEmulatorOrigin = false,
        )
        assertEquals("https://tasks.example.test", qr?.origin)
        assertEquals("/tasks", qr?.mount)
        assertEquals("00112233445566778899AABBCCDDEEFF", qr?.code)
    }

    @Test
    fun rejectsWrongHostVersionAndPathTraversal() {
        assertNull(
            ManagementEnrollmentQrParser.parse(
                "primer-management:v1:https://evil.test/tasks/management-device/enroll#00112233445566778899AABBCCDDEEFF",
                "https://tasks.example.test",
                false,
            ),
        )
        assertNull(
            ManagementEnrollmentQrParser.parse(
                "primer-management:v2:https://tasks.example.test/tasks/management-device/enroll#00112233445566778899AABBCCDDEEFF",
                "https://tasks.example.test",
                false,
            ),
        )
        assertNull(
            ManagementEnrollmentQrParser.parse(
                "primer-management:v1:https://tasks.example.test/tasks/../admin/management-device/enroll#00112233445566778899AABBCCDDEEFF",
                "https://tasks.example.test",
                false,
            ),
        )
    }
}

class RemoteLeasePolicyTest {
    @Test
    fun expiredOrRebootedLeaseCannotOpen() {
        val lease = RemoteLease(
            intentId = "r1",
            kind = "maintenance_lease",
            deliveryExpiresAtMs = 2_000,
            leaseExpiresAtMs = 1_500,
            serverNowMs = 1_000,
            receivedElapsedMs = 100,
            receivedBoot = 7,
        )
        assertEquals(true, RemoteLeasePolicy.canOpen(lease, requestElapsedMs = 100, responseElapsedMs = 150, requestBoot = 7, responseBoot = 7))
        assertEquals(false, RemoteLeasePolicy.canOpen(lease, requestElapsedMs = 100, responseElapsedMs = 150, requestBoot = 7, responseBoot = 8))
        assertEquals(false, RemoteLeasePolicy.canOpen(lease, requestElapsedMs = 100, responseElapsedMs = 700, requestBoot = 7, responseBoot = 7))
    }
}
