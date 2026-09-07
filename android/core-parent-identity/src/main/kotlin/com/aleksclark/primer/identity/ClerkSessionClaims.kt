package com.aleksclark.primer.identity

import java.util.Base64

/**
 * Public Clerk session claims used for origin/azp diagnosis.
 * Never returns the raw JWT, signature, or private payload.
 */
data class ClerkSessionClaims(
    val issuer: String?,
    val authorizedParty: String?,
    val audience: String?,
    val hasSessionId: Boolean,
    val hasSubject: Boolean,
) {
    override fun toString(): String =
        "issuer=$issuer azp=$authorizedParty aud=$audience sid=${if (hasSessionId) "present" else "missing"} sub=${if (hasSubject) "present" else "missing"}"
}

object ClerkSessionClaimReader {
    fun fromJwt(jwt: String?): ClerkSessionClaims? {
        if (jwt.isNullOrBlank()) return null
        val parts = jwt.split('.')
        if (parts.size != 3) return null
        val payload = decodePayload(parts[1]) ?: return null
        return ClerkSessionClaims(
            issuer = payload["iss"],
            authorizedParty = payload["azp"],
            audience = payload["aud"],
            hasSessionId = !payload["sid"].isNullOrBlank(),
            hasSubject = !payload["sub"].isNullOrBlank(),
        )
    }

    private fun decodePayload(segment: String): Map<String, String> {
        val bytes = runCatching { Base64.getUrlDecoder().decode(segment) }.getOrNull() ?: return emptyMap()
        val json = runCatching { String(bytes, Charsets.UTF_8) }.getOrNull() ?: return emptyMap()
        return extractStrings(json)
    }

    internal fun extractStrings(json: String): Map<String, String> {
        val out = linkedMapOf<String, String>()
        val matcher = Regex("\"(iss|azp|aud|sid|sub)\"\\s*:\\s*(\"[^\"]*\"|\\[[^\\]]*\\])").findAll(json)
        for (match in matcher) {
            val key = match.groupValues[1]
            val raw = match.groupValues[2].trim()
            val value = if (raw.startsWith("[")) {
                Regex("\"([^\"]+)\"").findAll(raw).map { it.groupValues[1] }.joinToString(",")
            } else {
                raw.trim('"')
            }
            if (value.isNotBlank()) out[key] = value
        }
        return out
    }
}
