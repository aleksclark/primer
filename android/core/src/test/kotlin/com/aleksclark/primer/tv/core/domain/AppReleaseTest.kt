package com.aleksclark.primer.tv.core.domain

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppReleaseTest {

    private fun release(versionCode: Int, available: Boolean = true) = AppRelease(
        available = available,
        versionCode = versionCode,
        sizeBytes = 1024,
        sha256 = "abc",
        downloadPath = "/api/v1/app/release/apk",
    )

    @Test
    fun `a higher version code is an update`() {
        assertTrue(release(8).isNewerThan(7))
    }

    @Test
    fun `the installed version is not an update`() {
        assertFalse(release(7).isNewerThan(7))
    }

    @Test
    fun `an older published build is never offered`() {
        assertFalse("a rollback must not install itself", release(6).isNewerThan(7))
    }

    @Test
    fun `an unpublished release is not an update`() {
        assertFalse(AppRelease.None.isNewerThan(0))
        assertFalse(release(99, available = false).isNewerThan(1))
    }

    @Test
    fun `an unversioned build is not an update`() {
        // A server with an APK but no version file reports zero. Treating that
        // as newer would reinstall on every launch.
        assertFalse(release(0).isNewerThan(0))
        assertFalse(release(0).isNewerThan(5))
    }
}
