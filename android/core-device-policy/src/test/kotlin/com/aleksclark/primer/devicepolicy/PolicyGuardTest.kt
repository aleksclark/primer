package com.aleksclark.primer.devicepolicy

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PolicyGuardTest {
    private val student = ApprovedApp("com.aleksclark.primer.student", "Primer Student", setOf("aa"))

    @Test
    fun remotePolicyCannotDropStudent() {
        val (apps, controls) = PolicyGuard.sanitizeRemoteApps(
            requested = listOf(ApprovedApp("com.example.calc", "Calc", setOf("bb"))),
            studentSigners = student.signers,
            installedSigners = { emptySet() },
        )
        assertTrue(apps.any { it.packageName == student.packageName })
        assertEquals("applied", controls.first { it.name == "student-home" }.status)
    }

    @Test
    fun mismatchedInstalledSignerIsReportedNotSilentlyApplied() {
        val (apps, controls) = PolicyGuard.sanitizeRemoteApps(
            requested = listOf(student, ApprovedApp("com.example.calc", "Calc", setOf("bb"))),
            studentSigners = student.signers,
            installedSigners = { if (it == "com.example.calc") setOf("cc") else emptySet() },
        )
        assertTrue(apps.none { it.packageName == "com.example.calc" })
        assertEquals("failed", controls.first { it.name == "approved:com.example.calc" }.status)
        assertEquals("failed", PolicyGuard.overallStatus(controls))
    }
}

class PairingCapabilityPolicyTest {
    @Test
    fun lockTaskWithoutGrantDoesNotPretendSystemPermissionUiWorks() {
        val capability = PairingCapabilityPolicy.evaluate(
            cameraGranted = false,
            owner = true,
            inMaintenance = false,
            permissionControllerPackage = "com.android.permissioncontroller",
            photoPickerPackage = "com.android.providers.media.module",
        )
        assertEquals(false, capability.canScan)
        assertEquals(false, capability.parentCanGrantCamera)
        assertTrue(capability.message.contains("open maintenance"))
    }

    @Test
    fun parentMaintenanceCanGrantWithoutPermanentSettingsAllowlist() {
        val capability = PairingCapabilityPolicy.evaluate(
            cameraGranted = false,
            owner = true,
            inMaintenance = true,
            permissionControllerPackage = "com.android.permissioncontroller",
            photoPickerPackage = "com.google.android.photopicker",
        )
        assertEquals(true, capability.parentCanGrantCamera)
        assertEquals(
            listOf("com.android.permissioncontroller", "com.google.android.photopicker", "com.android.documentsui"),
            PairingCapabilityPolicy.maintenanceDelegates(
                "com.android.permissioncontroller",
                "com.google.android.photopicker",
                "com.android.documentsui",
            ),
        )
    }
}
