package com.aleksclark.primer.tv.app.data

import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.mutablePreferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DataStoreMappingTest {

    @Test
    fun `empty settings are unpaired`() {
        val settings = DataStoreSettingsStore.toSettings(emptyPreferences())

        assertNull(settings.token)
        assertNull(settings.baseUrl)
        assertFalse(settings.isPaired)
    }

    @Test
    fun `a base url without a token is still unpaired`() {
        val prefs = mutablePreferencesOf(stringPreferencesKey("base_url") to "http://tv.local/")

        val settings = DataStoreSettingsStore.toSettings(prefs)

        assertEquals("http://tv.local/", settings.baseUrl)
        assertFalse("an address alone cannot authenticate", settings.isPaired)
    }

    @Test
    fun `a full pairing round-trips`() {
        val prefs = mutablePreferencesOf(
            stringPreferencesKey("base_url") to "http://tv.local/",
            stringPreferencesKey("token") to "secret",
            stringPreferencesKey("device_id") to "device-1",
            stringPreferencesKey("device_name") to "Playroom",
            stringPreferencesKey("device_kind") to "tv_box",
        )

        val settings = DataStoreSettingsStore.toSettings(prefs)

        assertTrue(settings.isPaired)
        assertEquals("secret", settings.token)
        assertEquals("Playroom", settings.deviceName)
        assertEquals("tv_box", settings.deviceKind)
    }

    @Test
    fun `no stored grant reads as null`() {
        assertNull(DataStoreGrantStore.toGrant(emptyPreferences()))
    }

    @Test
    fun `a grant missing its stream url is discarded rather than half-restored`() {
        val prefs = mutablePreferencesOf(
            stringPreferencesKey("grant_id") to "grant-1",
            stringPreferencesKey("media_item_id") to "item-1",
        )

        assertNull(DataStoreGrantStore.toGrant(prefs))
    }

    @Test
    fun `a stored grant round-trips with its counters`() {
        val prefs = mutablePreferencesOf(
            stringPreferencesKey("grant_id") to "grant-1",
            stringPreferencesKey("media_item_id") to "item-1",
            stringPreferencesKey("stream_url") to "http://jellyfin.local/stream",
            intPreferencesKey("start_offset_seconds") to 12,
            stringPreferencesKey("mode") to "on_demand",
            longPreferencesKey("expires_at_epoch_seconds") to 1_740_000_000L,
            intPreferencesKey("position_seconds") to 300,
            intPreferencesKey("watched_seconds") to 280,
            booleanPreferencesKey("redeemed") to true,
        )

        val grant = DataStoreGrantStore.toGrant(prefs)!!

        assertEquals("grant-1", grant.grantId)
        assertEquals("item-1", grant.mediaItemId)
        assertEquals(300, grant.positionSeconds)
        assertEquals(280, grant.watchedSeconds)
        assertTrue(grant.redeemed)
        // Missing furthest key falls back to position so older stores still resume.
        assertEquals(300, grant.furthestPositionSeconds)
    }

    @Test
    fun `a stored grant keeps an explicit furthest watermark`() {
        val prefs = mutablePreferencesOf(
            stringPreferencesKey("grant_id") to "grant-1",
            stringPreferencesKey("media_item_id") to "item-1",
            stringPreferencesKey("stream_url") to "http://jellyfin.local/stream",
            intPreferencesKey("start_offset_seconds") to 0,
            stringPreferencesKey("mode") to "on_demand",
            longPreferencesKey("expires_at_epoch_seconds") to 1_740_000_000L,
            intPreferencesKey("position_seconds") to 250,
            intPreferencesKey("watched_seconds") to 240,
            booleanPreferencesKey("redeemed") to true,
            intPreferencesKey("furthest_position_seconds") to 900,
        )

        val grant = DataStoreGrantStore.toGrant(prefs)!!
        assertEquals(900, grant.furthestPositionSeconds)
        assertEquals(250, grant.positionSeconds)
    }
}
