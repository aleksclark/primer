package com.aleksclark.primer.tv.app.admin

import android.os.UserManager
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class KioskPolicyTest {
    @Test
    fun `television accepts Android TV mode and generic TV box firmware`() {
        assertTrue(KioskPolicy.isTelevision(4, "tablet"))
        assertTrue(KioskPolicy.isTelevision(1, "box"))
        assertTrue(KioskPolicy.isTelevision(1, "default,box"))
        assertFalse(KioskPolicy.isTelevision(1, "tablet"))
    }

    @Test
    fun `kiosk requires both television and device ownership`() {
        assertTrue(KioskPolicy.isEligible(isTelevision = true, isDeviceOwner = true))
        assertFalse(KioskPolicy.isEligible(isTelevision = true, isDeviceOwner = false))
        assertFalse(KioskPolicy.isEligible(isTelevision = false, isDeviceOwner = true))
        assertFalse(KioskPolicy.isEligible(isTelevision = false, isDeviceOwner = false))
    }

    @Test
    fun `restrictions preserve networking media and package installation`() {
        val restrictions = KioskPolicy.userRestrictions

        assertTrue(UserManager.DISALLOW_FACTORY_RESET in restrictions)
        assertTrue(UserManager.DISALLOW_SAFE_BOOT in restrictions)
        assertFalse(UserManager.DISALLOW_CONFIG_WIFI in restrictions)
        assertFalse(UserManager.DISALLOW_NETWORK_RESET in restrictions)
        assertFalse(UserManager.DISALLOW_INSTALL_APPS in restrictions)
        assertFalse(UserManager.DISALLOW_INSTALL_UNKNOWN_SOURCES in restrictions)
        assertFalse(UserManager.DISALLOW_ADJUST_VOLUME in restrictions)
        assertFalse(UserManager.DISALLOW_BLUETOOTH in restrictions)
    }
}
