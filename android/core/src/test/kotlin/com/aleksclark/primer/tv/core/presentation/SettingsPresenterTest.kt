package com.aleksclark.primer.tv.core.presentation

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SettingsPresenterTest {

    @Test
    fun `settings groups device server updates and danger zone titles`() {
        assertEquals(
            listOf("Device", "Server", "Updates", "Danger Zone"),
            SettingsSectionId.entries.map { it.title() },
        )
    }

    @Test
    fun `device and server identity are presented with friendly fallbacks`() {
        val model = SettingsPresenter.present(
            deviceName = "Playroom",
            deviceKind = "tv_box",
            deviceId = "d1",
            serverUrl = "https://tv.local:8081",
            installedVersionCode = 7,
            updateStatus = UpdateStatus.UP_TO_DATE,
        )

        assertEquals("Playroom", model.deviceName)
        assertEquals("Television", model.deviceKind)
        assertEquals("d1", model.deviceId)
        assertEquals("https://tv.local:8081", model.serverUrl)
        assertEquals("Installed version 7", model.installedVersionLabel)
        assertTrue(model.update is UpdateCardModel.UpToDate)
        assertTrue(model.update.canRetryCheck)
        assertFalse(model.update.canInstall)
    }

    @Test
    fun `available update exposes install action and version copy`() {
        val update = SettingsPresenter.presentUpdate(
            installedVersionCode = 7,
            status = UpdateStatus.AVAILABLE,
            availableVersionCode = 9,
        ) as UpdateCardModel.Available

        assertEquals(9, update.versionCode)
        assertTrue(update.canInstall)
        assertTrue(update.statusLabel.value.contains("9"))
        assertTrue(update.statusLabel.value.contains("7"))
    }

    @Test
    fun `downloading and installing are busy and block re-check spam`() {
        val downloading = SettingsPresenter.presentUpdate(7, UpdateStatus.DOWNLOADING)
        val installing = SettingsPresenter.presentUpdate(7, UpdateStatus.INSTALLING)

        assertTrue(downloading.isBusy)
        assertFalse(downloading.canRetryCheck)
        assertTrue(installing.isBusy)
        assertFalse(installing.canRetryCheck)
    }

    @Test
    fun `failed update keeps a retryable error with clear status text`() {
        val failed = SettingsPresenter.presentUpdate(
            installedVersionCode = 7,
            status = UpdateStatus.FAILED,
            failureMessage = "The download was corrupted and has been discarded.",
        ) as UpdateCardModel.Failed

        assertTrue(failed.isError)
        assertTrue(failed.canRetryCheck)
        assertFalse(failed.canInstall)
        assertEquals(
            "The download was corrupted and has been discarded.",
            failed.statusLabel.value,
        )
    }

    @Test
    fun `missing identity uses intentional placeholders`() {
        val model = SettingsPresenter.present(
            deviceName = null,
            deviceKind = null,
            deviceId = null,
            serverUrl = null,
            installedVersionCode = 1,
            updateStatus = UpdateStatus.UP_TO_DATE,
        )

        assertEquals("Unnamed device", model.deviceName)
        assertEquals("Unknown", model.deviceKind)
        assertEquals(null, model.deviceId)
        assertEquals("Not set", model.serverUrl)
    }
}
