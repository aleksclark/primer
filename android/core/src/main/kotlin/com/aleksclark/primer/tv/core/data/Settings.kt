package com.aleksclark.primer.tv.core.data

import com.aleksclark.primer.tv.core.domain.PlayGrant
import java.time.Instant
import kotlinx.coroutines.flow.Flow
import kotlinx.serialization.Serializable

/** Everything persisted about the pairing with a TV server. */
data class DeviceSettings(
    val baseUrl: String? = null,
    val token: String? = null,
    val deviceId: String? = null,
    val deviceName: String? = null,
    val deviceKind: String? = null,
) {
    /** Whether the app has both an address and a token, and can talk to the server. */
    val isPaired: Boolean get() = !baseUrl.isNullOrBlank() && !token.isNullOrBlank()
}

/**
 * Persistent device settings. Backed by DataStore in the app module; the
 * interface keeps [TvRepository] and the view models testable without Android.
 */
interface SettingsStore {
    val settings: Flow<DeviceSettings>

    /** Reads the current settings once, for a synchronous caller such as an interceptor. */
    suspend fun current(): DeviceSettings

    suspend fun setBaseUrl(baseUrl: String)

    suspend fun savePairing(token: String, deviceId: String, deviceName: String, deviceKind: String)

    /** Forgets the token and device identity, keeping the server address. */
    suspend fun clearPairing()
}

/**
 * A grant persisted across process death so an app killed mid-film can rejoin
 * its own session instead of burning a second play.
 */
@Serializable
data class StoredGrant(
    val grantId: String,
    val mediaItemId: String,
    val streamUrl: String,
    val startOffsetSeconds: Int,
    val mode: String,
    val expiresAtEpochSeconds: Long,
    val positionSeconds: Int,
    val watchedSeconds: Int,
    val redeemed: Boolean,
) {
    fun toGrant(serverTime: Instant): PlayGrant = PlayGrant(
        grantId = grantId,
        mediaItemId = mediaItemId,
        streamUrl = streamUrl,
        startOffsetSeconds = startOffsetSeconds,
        mode = mode,
        expiresAt = Instant.ofEpochSecond(expiresAtEpochSeconds),
        serverTime = serverTime,
    )

    /**
     * Whether this grant is still worth resuming.
     *
     * A grant already redeemed stays resumable past its expiry: the server gates
     * expiry only on *starting* playback, and its session keeps accepting
     * progress for as long as the client reports it. An unredeemed grant past
     * expiry is dead and must be replaced by a fresh request.
     */
    fun resumableAt(now: Instant): Boolean =
        redeemed || now.isBefore(Instant.ofEpochSecond(expiresAtEpochSeconds))

    companion object {
        fun of(grant: PlayGrant, positionSeconds: Int, watchedSeconds: Int, redeemed: Boolean) = StoredGrant(
            grantId = grant.grantId,
            mediaItemId = grant.mediaItemId,
            streamUrl = grant.streamUrl,
            startOffsetSeconds = grant.startOffsetSeconds,
            mode = grant.mode,
            expiresAtEpochSeconds = grant.expiresAt.epochSecond,
            positionSeconds = positionSeconds,
            watchedSeconds = watchedSeconds,
            redeemed = redeemed,
        )
    }
}

/** Persists the in-flight grant so playback survives process death. */
interface GrantStore {
    suspend fun load(): StoredGrant?
    suspend fun save(grant: StoredGrant)
    suspend fun clear()
}
