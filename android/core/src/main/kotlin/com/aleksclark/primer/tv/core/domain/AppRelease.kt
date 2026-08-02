package com.aleksclark.primer.tv.core.domain

/**
 * What the server publishes for sideloading, compared against what is running.
 *
 * The box has no app store, so the server is the whole distribution channel.
 * Version codes are monotonic, which is why "newer" is an integer comparison
 * and not a string one.
 */
data class AppRelease(
    val available: Boolean,
    val versionCode: Int,
    val sizeBytes: Long,
    val sha256: String,
    val downloadPath: String,
) {
    /**
     * Whether this release is worth installing over [installedVersionCode].
     *
     * An unpublished or unversioned release is never newer: a server with no
     * `version` file reports zero, and offering that as an update would
     * downgrade the box on every launch.
     */
    fun isNewerThan(installedVersionCode: Int): Boolean =
        available && versionCode > installedVersionCode

    companion object {
        /** Nothing published, which is a normal state and not an error. */
        val None = AppRelease(
            available = false,
            versionCode = 0,
            sizeBytes = 0,
            sha256 = "",
            downloadPath = "",
        )
    }
}
