package com.aleksclark.primertasks.client

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

/** Stable façade; generated wire models remain build-only. */
class TasksClient(
    baseUrl: String,
    private val http: OkHttpClient = OkHttpClient(),
) {
    private val apiBaseUrl = baseUrl.trimEnd('/') + "/api"
    private val json = Json { ignoreUnknownKeys = true }

    suspend fun pairDevice(code: String): DevicePairResponse = withContext(Dispatchers.IO) {
        post("$apiBaseUrl/device/pair", PairCode(code), DevicePairResponse.serializer())
    }

    suspend fun studentProfile(token: String): StudentProfile = withContext(Dispatchers.IO) {
        get("$apiBaseUrl/student/profile", token, StudentProfile.serializer())
    }

    suspend fun studentChecklist(token: String): ChecklistResponse = withContext(Dispatchers.IO) {
        get("$apiBaseUrl/student/checklist", token, ChecklistResponse.serializer())
    }

    fun authHeader(token: String) = "Bearer $token"

    private fun <T> post(url: String, body: Any, serializer: kotlinx.serialization.KSerializer<T>): T {
        val payload = when (body) {
            is PairCode -> json.encodeToString(PairCode.serializer(), body)
            else -> error("unsupported request")
        }
        val request = Request.Builder()
            .url(url)
            .post(payload.toRequestBody(JSON))
            .build()
        return execute(request, serializer)
    }

    private fun <T> get(url: String, token: String, serializer: kotlinx.serialization.KSerializer<T>): T {
        val request = Request.Builder().url(url).header("Authorization", authHeader(token)).get().build()
        return execute(request, serializer)
    }

    private fun <T> execute(request: Request, serializer: kotlinx.serialization.KSerializer<T>): T {
        http.newCall(request).execute().use { response ->
            if (!response.isSuccessful) throw TasksHttpException(response.code)
            val body = response.body?.string() ?: throw TasksHttpException(response.code, "empty response")
            return json.decodeFromString(serializer, body)
        }
    }

    companion object {
        private val JSON = "application/json".toMediaType()
    }
}

@Serializable
private data class PairCode(val code: String)

@Serializable
data class DevicePairResponse(val token: String, val studentId: String)

@Serializable
data class StudentProfile(val id: String, val displayName: String)

@Serializable
data class ChecklistResponse(val items: List<ChecklistItem> = emptyList())

@Serializable
data class ChecklistItem(
    val id: String = "",
    val title: String = "",
    val description: String = "",
    val status: String = "",
)

class TasksHttpException(val statusCode: Int, message: String = "request failed") : Exception(message)
