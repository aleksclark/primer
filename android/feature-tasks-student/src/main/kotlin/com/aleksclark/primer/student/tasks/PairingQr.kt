package com.aleksclark.primer.student.tasks

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.net.URI

@Serializable
data class PairingQr(
    val origin: String,
    val code: String,
    val pairingId: String,
    val api: String? = null,
    val v: Int? = null,
)

object PairingQrParser {
    private val json = Json { ignoreUnknownKeys = true; isLenient = false }

    fun parse(raw: String): PairingQr? = runCatching {
        json.decodeFromString<PairingQr>(raw).also { qr ->
            require(qr.origin.isNotBlank() && qr.code.isNotBlank() && qr.pairingId.isNotBlank())
        }
    }.getOrNull()
}

/**
 * Pins the QR origin + api mount to the configured HTTPS deployment.
 *
 * Bare configured origin means the legacy `/api` mount. Configured `/tasks/api`
 * must remain exact. Emulator host translation is debug-only.
 */
object ServerOriginPolicy {
    private const val emulatorHost = "10.0.2.2"
    private val allowedApi = setOf("/api", "/tasks/api")

    fun allowedApiBase(
        origin: String,
        api: String?,
        configuredHttpsOrigin: String,
        allowEmulatorOrigin: Boolean,
        version: Int? = 1,
    ): String? {
        if (version != null && version != 1) return null
        val originBase = allowedOrigin(origin, configuredHttpsOrigin, allowEmulatorOrigin) ?: return null
        val mount = canonicalApi(api ?: defaultApi(configuredHttpsOrigin)) ?: return null
        val configuredMount = configuredApi(configuredHttpsOrigin)
        if (configuredMount != null && mount != configuredMount) return null
        return originBase.trimEnd('/') + mount
    }

    fun allowedOrigin(origin: String, configuredHttpsOrigin: String, allowEmulatorOrigin: Boolean): String? {
        val candidate = canonicalOrigin(origin) ?: return null
        val configured = canonicalOrigin(stripApiPath(configuredHttpsOrigin) ?: configuredHttpsOrigin)
        if (configured != null && configured.scheme == "https" && candidate == configured) {
            return candidate.value
        }
        if (allowEmulatorOrigin && candidate.scheme == "http" && (candidate.host == emulatorHost || candidate.host == "127.0.0.1")) {
            return if (candidate.host == "127.0.0.1") candidate.value.replace("127.0.0.1", emulatorHost) else candidate.value
        }
        return null
    }

    private fun defaultApi(configuredHttpsOrigin: String): String =
        configuredApi(configuredHttpsOrigin) ?: "/api"

    private fun configuredApi(configuredHttpsOrigin: String): String? {
        val uri = runCatching { URI(configuredHttpsOrigin.trim()) }.getOrNull() ?: return null
        val path = canonicalApi(uri.rawPath ?: "") ?: return if ((uri.rawPath ?: "").isEmpty() || uri.rawPath == "/") "/api" else null
        return path
    }

    private fun canonicalApi(raw: String): String? {
        val path = raw.trim()
        if (path.isEmpty()) return "/api"
        if (path.contains('\\') || path.contains('\u0000')) return null
        val uri = runCatching { URI(null, null, path, null, null) }.getOrNull() ?: return null
        if (uri.rawQuery != null || uri.rawFragment != null || uri.rawAuthority != null || uri.userInfo != null) return null
        val normalized = uri.normalize().rawPath ?: return null
        if (normalized.contains("..") || normalized.contains("//") || normalized.contains("%")) return null
        val trimmed = normalized.trimEnd('/')
        return trimmed.takeIf { it in allowedApi }
    }

    private fun stripApiPath(raw: String): String? {
        val uri = runCatching { URI(raw.trim()) }.getOrNull() ?: return null
        val path = (uri.rawPath ?: "").trimEnd('/')
        if (path.isEmpty() || path == "/") return raw
        if (path !in allowedApi) return null
        return URI(uri.scheme, uri.authority, null, null, null).toString()
    }

    private fun canonicalOrigin(raw: String): Origin? {
        val uri = runCatching { URI(raw.trim()) }.getOrNull() ?: return null
        val scheme = uri.scheme?.lowercase() ?: return null
        val host = uri.host?.lowercase() ?: return null
        if (uri.userInfo != null || uri.query != null || uri.fragment != null) return null
        val path = uri.path.orEmpty()
        if (path.isNotEmpty() && path != "/") return null
        if (uri.port == 0 || uri.port < -1) return null
        val value = buildString {
            append(scheme).append("://").append(host)
            if (uri.port >= 0) append(':').append(uri.port)
        }
        return Origin(scheme, host, value)
    }

    private data class Origin(val scheme: String, val host: String, val value: String)
}
