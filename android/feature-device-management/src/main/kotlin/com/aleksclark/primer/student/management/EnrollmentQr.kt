package com.aleksclark.primer.student.management

import java.net.URI

data class ManagementEnrollmentQr(
    val origin: String,
    val mount: String,
    val enrollPath: String,
    val code: String,
)

object ManagementEnrollmentQrParser {
    private const val SCHEME = "primer-management"
    private const val VERSION = "v1"
    private val allowedMounts = setOf("", "/tasks")

    fun parse(raw: String, configuredHttpsOrigin: String, allowEmulatorOrigin: Boolean): ManagementEnrollmentQr? {
        val parts = raw.trim().split(':', limit = 4)
        if (parts.size != 4 || parts[0] != SCHEME || parts[1] != VERSION) return null
        val url = runCatching { URI(parts[2] + ":" + parts[3]) }.getOrNull() ?: return null
        if (url.userInfo != null || url.query != null) return null
        val code = url.fragment?.trim().orEmpty()
        if (!code.matches(Regex("[0-9A-Fa-f]{32}"))) return null
        val path = url.path.orEmpty()
        if (!path.endsWith("/management-device/enroll")) return null
        val mount = path.removeSuffix("/management-device/enroll")
        if (mount !in allowedMounts) return null
        val origin = originOf(url) ?: return null
        val configured = canonicalConfiguredOrigin(configuredHttpsOrigin)
        val emulator = allowEmulatorOrigin && url.scheme == "http" && (url.host == "10.0.2.2" || url.host == "127.0.0.1")
        if (configured != null) {
            if (origin != configured && !emulator) return null
        } else if (!emulator) {
            return null
        }
        val translated = if (emulator && url.host == "127.0.0.1") origin.replace("127.0.0.1", "10.0.2.2") else origin
        return ManagementEnrollmentQr(translated, mount, "/management-device/enroll", code)
    }

    private fun originOf(uri: URI): String? {
        val scheme = uri.scheme?.lowercase() ?: return null
        val host = uri.host?.lowercase() ?: return null
        if (scheme != "https" && scheme != "http") return null
        return buildString {
            append(scheme).append("://").append(host)
            if (uri.port >= 0) append(':').append(uri.port)
        }
    }

    private fun canonicalConfiguredOrigin(raw: String): String? {
        if (raw.isBlank()) return null
        val uri = runCatching { URI(raw.trim()) }.getOrNull() ?: return null
        return originOf(uri)
    }
}
