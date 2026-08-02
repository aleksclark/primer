package com.aleksclark.primer.tv.app

import com.aleksclark.primer.tv.core.data.DeviceSettings
import com.aleksclark.primer.tv.core.data.GrantStore
import com.aleksclark.primer.tv.core.data.SettingsStore
import com.aleksclark.primer.tv.core.data.StoredGrant
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first

/** In-memory [SettingsStore] for view model tests. */
class FakeSettingsStore(initial: DeviceSettings = DeviceSettings()) : SettingsStore {
    private val state = MutableStateFlow(initial)

    override val settings: Flow<DeviceSettings> = state

    override suspend fun current(): DeviceSettings = state.first()

    override suspend fun setBaseUrl(baseUrl: String) {
        state.value = state.value.copy(baseUrl = baseUrl)
    }

    override suspend fun savePairing(token: String, deviceId: String, deviceName: String, deviceKind: String) {
        state.value = state.value.copy(
            token = token,
            deviceId = deviceId,
            deviceName = deviceName,
            deviceKind = deviceKind,
        )
    }

    override suspend fun clearPairing() {
        state.value = state.value.copy(token = null, deviceId = null, deviceName = null, deviceKind = null)
    }
}

/** In-memory [GrantStore] for view model tests. */
class FakeGrantStore(initial: StoredGrant? = null) : GrantStore {
    var stored: StoredGrant? = initial
        private set

    override suspend fun load(): StoredGrant? = stored

    override suspend fun save(grant: StoredGrant) {
        stored = grant
    }

    override suspend fun clear() {
        stored = null
    }
}
