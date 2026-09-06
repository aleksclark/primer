package com.aleksclark.primer.identity

import java.net.URI

/**
 * Control origin guard. Pins the configured Tasks origin to HTTPS except the
 * emulator loopback. This is not a Student QR parser; Student keeps
 * [com.aleksclark.primer.student.tasks.ServerOriginPolicy].
 */
object TasksUrlGuard {
    data class Endpoint(val origin: String, val apiBase: String)

    private const val emulatorHost = "10.0.2.2"

    fun endpoint(
        originRaw: String,
        apiRaw: String,
        configuredHttpsOrigin: String,
        allowEmulatorOrigin: Boolean,
    ): Endpoint? {
        val raw = originRaw.trim().ifEmpty { apiRaw.trim() }.ifEmpty { configuredHttpsOrigin.trim() }
        if (raw.isEmpty()) return null
        val uri = runCatching { URI(raw) }.getOrNull() ?: return null
        val scheme = uri.scheme?.lowercase() ?: return null
        val host = uri.host?.lowercase() ?: return null
        if (uri.userInfo != null || uri.query != null || uri.fragment != null) return null
        if (uri.port == 0 || uri.port < -1) return null
        val emulator = host == emulatorHost || host == "127.0.0.1"
        val allowed = when (scheme) {
            "https" -> true
            "http" -> allowEmulatorOrigin && emulator
            else -> false
        }
        if (!allowed) return null
        val originHost = if (host == "127.0.0.1" && allowEmulatorOrigin) emulatorHost else host
        val origin = buildString {
            append(scheme).append("://").append(originHost)
            if (uri.port >= 0) append(':').append(uri.port)
        }
        val path = (uri.rawPath ?: "").trimEnd('/')
        val mount = when (path) {
            "", "/" -> "/api"
            "/api" -> "/api"
            "/tasks" -> "/tasks/api"
            "/tasks/api" -> "/tasks/api"
            else -> return null
        }
        return Endpoint(origin = origin, apiBase = origin + mount)
    }
}
