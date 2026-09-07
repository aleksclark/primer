package com.aleksclark.primer.identity

/** Control talks to an absolute Tasks origin. Apps still enforce HTTPS except the emulator host. */
object ControlOriginPolicy {
    const val CONTROL_APPLICATION_ID = "com.aleksclark.primer.control"

    /** Clerk Android 0.1.31 SSOReceiverActivity: clerk://{applicationId}.oauth */
    const val CLERK_NATIVE_OAUTH_REDIRECT = "clerk://com.aleksclark.primer.control.oauth"

    fun apiBase(configuredOrigin: String, allowEmulatorOrigin: Boolean): String? {
        val trimmed = configuredOrigin.trim()
        return TasksUrlGuard.endpoint(
            originRaw = trimmed,
            apiRaw = trimmed,
            configuredHttpsOrigin = trimmed,
            allowEmulatorOrigin = allowEmulatorOrigin,
        )?.apiBase
    }
}
