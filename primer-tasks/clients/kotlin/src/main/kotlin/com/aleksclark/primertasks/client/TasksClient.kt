package com.aleksclark.primertasks.client

import com.aleksclark.primertasks.generated.ChecklistItem as GeneratedChecklistItem
import com.aleksclark.primertasks.generated.ChecklistResponse as GeneratedChecklistResponse
import com.aleksclark.primertasks.generated.DevicePairResponse as GeneratedDevicePairResponse
import com.aleksclark.primertasks.generated.PairCode
import com.aleksclark.primertasks.generated.PrimerTasksOperations
import com.aleksclark.primertasks.generated.StudentProfile as GeneratedStudentProfile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

/** Stable transport façade over the OpenAPI-generated request, response, and route types. */
class TasksClient(
    baseUrl: String,
    private val http: OkHttpClient = OkHttpClient(),
) {
    private val apiBaseUrl = baseUrl.trimEnd('/') + "/api"
    private val json = Json { ignoreUnknownKeys = true }

    suspend fun pairDevice(code: String): DevicePairResponse = withContext(Dispatchers.IO) {
        post(PrimerTasksOperations.DEVICE_PAIR, PairCode(code), DevicePairResponse.serializer())
    }

    suspend fun studentProfile(token: String): StudentProfile = withContext(Dispatchers.IO) {
        get(PrimerTasksOperations.STUDENT_PROFILE, token, StudentProfile.serializer())
    }

    suspend fun studentChecklist(token: String): ChecklistResponse = withContext(Dispatchers.IO) {
        get(PrimerTasksOperations.STUDENT_CHECKLIST, token, ChecklistResponse.serializer())
    }

    fun authHeader(token: String) = "Bearer $token"

    private fun <T> post(path: String, body: PairCode, serializer: kotlinx.serialization.KSerializer<T>): T {
        val request = Request.Builder()
            .url(apiBaseUrl + path)
            .post(json.encodeToString(PairCode.serializer(), body).toRequestBody(JSON))
            .build()
        return execute(request, serializer)
    }

    private fun <T> get(path: String, token: String, serializer: kotlinx.serialization.KSerializer<T>): T {
        val request = Request.Builder().url(apiBaseUrl + path).header("Authorization", authHeader(token)).get().build()
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

typealias DevicePairResponse = GeneratedDevicePairResponse
typealias StudentProfile = GeneratedStudentProfile
typealias ChecklistResponse = GeneratedChecklistResponse
typealias ChecklistItem = GeneratedChecklistItem

class TasksHttpException(val statusCode: Int, message: String = "request failed") : Exception(message)
