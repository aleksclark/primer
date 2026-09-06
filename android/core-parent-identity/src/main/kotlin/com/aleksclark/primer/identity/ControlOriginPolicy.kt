package com.aleksclark.primer.identity

/** Control talks to an absolute Tasks origin. Apps still enforce HTTPS except the emulator host. */
object ControlOriginPolicy {
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
