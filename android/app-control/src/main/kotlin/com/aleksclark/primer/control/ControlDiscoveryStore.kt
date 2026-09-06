package com.aleksclark.primer.control

import android.content.Context
import com.aleksclark.primer.control.device.ControlUpdateDiscoverySettings

interface ControlDiscoveryStore {
    fun load(): ControlUpdateDiscoverySettings
    fun save(settings: ControlUpdateDiscoverySettings)
}

class PrefsControlDiscoveryStore(
    context: Context,
) : ControlDiscoveryStore {
    private val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    override fun load() = ControlUpdateDiscoverySettings(
        checkOnResume = prefs.getBoolean("checkOnResume", true),
        periodicEnabled = prefs.getBoolean("periodicEnabled", false),
        unattendedCatchUp = prefs.getBoolean("unattendedCatchUp", false),
        lastCatalogCheckAtMs = prefs.getLong("lastCatalogCheckAtMs", 0),
    )

    override fun save(settings: ControlUpdateDiscoverySettings) {
        prefs.edit()
            .putBoolean("checkOnResume", settings.checkOnResume)
            .putBoolean("periodicEnabled", settings.periodicEnabled)
            .putBoolean("unattendedCatchUp", settings.unattendedCatchUp)
            .putLong("lastCatalogCheckAtMs", settings.lastCatalogCheckAtMs)
            .apply()
    }

    companion object {
        private const val PREFS = "control-discovery"
    }
}
