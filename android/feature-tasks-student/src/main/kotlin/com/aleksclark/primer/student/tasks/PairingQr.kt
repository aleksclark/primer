package com.aleksclark.primer.student.tasks

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.net.URI

@Serializable
data class PairingQr(
    val origin: String,
    val code: String,
    val pairingId: String,
)

object PairingQrParser {
    private val json = Json { ignoreUnknownKeys = true; isLenient = false }

    fun parse(raw: String): PairingQr? = runCatching {
        json.decodeFromString<PairingQr>(raw).also { qr ->
            require(qr.origin.isNotBlank() && qr.code.isNotBlank() && qr.pairingId.isNotBlank())
        }
    }.getOrNull()
}

/** Only a deployment-configured HTTPS origin or the Android emulator's host alias is accepted. */
object ServerOriginPolicy {
    private const val emulatorHost = "10.0.2.2"

    fun allowedOrigin(origin: String, configuredHttpsOrigin: String, allowEmulatorOrigin: Boolean): String? {
        val candidate = canonicalOrigin(origin) ?: return null
        val configured = canonicalOrigin(configuredHttpsOrigin)
        if (configured != null && configured.scheme == "https" && candidate == configured) {
            return candidate.value
        }
        if (allowEmulatorOrigin && candidate.scheme == "http" && (candidate.host == emulatorHost || candidate.host == "127.0.0.1")) {
            return if (candidate.host == "127.0.0.1") candidate.value.replace("127.0.0.1", emulatorHost) else candidate.value
        }
        return null
    }

    private fun canonicalOrigin(raw: String): Origin? {
        val uri = runCatching { URI(raw.trim()) }.getOrNull() ?: return null
        val scheme = uri.scheme?.lowercase() ?: return null
        val host = uri.host?.lowercase() ?: return null
        if (uri.userInfo != null || uri.query != null || uri.fragment != null) return null
        if (uri.path.isNotEmpty() && uri.path != "/") return null
        if (uri.port == 0 || uri.port < -1) return null
        val port = uri.port
        val value = buildString {
            append(scheme).append("://").append(host)
            if (port >= 0) append(':').append(port)
        }
        return Origin(scheme, host, value)
    }

    private data class Origin(val scheme: String, val host: String, val value: String)
}
