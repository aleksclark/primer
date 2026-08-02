package com.aleksclark.primer.tv.core.net

import java.util.concurrent.TimeUnit
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.serialization.json.Json
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Response
import retrofit2.Retrofit

/** The path the TV server mounts its API under (see `cmd/tv-server/main.go`). */
const val API_PATH_PREFIX = "api/v1/"

/**
 * JSON configured for the TV server: unknown keys are ignored so a server that
 * grows a field does not break an already-sideloaded APK, and defaults are
 * encoded explicitly because Huma validates required properties.
 */
val tvJson: Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = true
}

/**
 * Attaches the paired device's bearer token, matching the `deviceToken` security
 * scheme. The token is read per request rather than captured, so revoking or
 * re-pairing takes effect without rebuilding the client.
 */
class DeviceTokenInterceptor(private val token: () -> String?) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val token = token()
        val request = if (token.isNullOrBlank()) {
            chain.request()
        } else {
            chain.request().newBuilder().header("Authorization", "Bearer $token").build()
        }
        return chain.proceed(request)
    }
}

/**
 * Normalises a user-entered server address into a base URL Retrofit accepts:
 * adds a scheme when the parent typed a bare host or `host:port`, strips a
 * trailing `/api/v1` so pasting the admin UI's address works, and guarantees the
 * trailing slash Retrofit requires.
 */
fun normalizeBaseUrl(input: String): String? {
    val trimmed = input.trim()
    if (trimmed.isEmpty()) return null

    val withScheme = if (trimmed.contains("://")) trimmed else "http://$trimmed"
    val url = withScheme.toHttpUrlOrNull() ?: return null
    if (url.host.isBlank()) return null

    val segments = url.pathSegments.filter { it.isNotBlank() }.toMutableList()
    if (segments.size >= 2 && segments[segments.size - 2] == "api" && segments.last() == "v1") {
        segments.removeAt(segments.size - 1)
        segments.removeAt(segments.size - 1)
    }

    val base = url.newBuilder().apply {
        encodedPath("/")
        segments.forEach { addPathSegment(it) }
    }.build()

    return base.toString().trimEnd('/') + "/"
}

/** Builds the OkHttp client used for both API calls and media playback. */
fun buildOkHttpClient(token: () -> String?): OkHttpClient = OkHttpClient.Builder()
    // The box sits on the same LAN as the server; short timeouts surface a dead
    // server quickly instead of leaving a spinner up.
    .connectTimeout(10, TimeUnit.SECONDS)
    .readTimeout(20, TimeUnit.SECONDS)
    .writeTimeout(20, TimeUnit.SECONDS)
    .retryOnConnectionFailure(true)
    .addInterceptor(DeviceTokenInterceptor(token))
    .build()

/** Builds a [TvApi] bound to one server base URL. */
fun buildTvApi(baseUrl: String, client: OkHttpClient): TvApi = Retrofit.Builder()
    .baseUrl(baseUrl.trimEnd('/') + "/" + API_PATH_PREFIX)
    .client(client)
    .addConverterFactory(tvJson.asConverterFactory("application/json".toMediaType()))
    .build()
    .create(TvApi::class.java)

/** Resolves an image path returned by the catalog against the server base URL. */
fun resolveImageUrl(baseUrl: String, imagePath: String): String {
    if (imagePath.startsWith("http://") || imagePath.startsWith("https://")) return imagePath
    return baseUrl.trimEnd('/') + "/" + API_PATH_PREFIX + imagePath.trimStart('/')
}
