package com.aleksclark.primertasks.client

import okhttp3.OkHttpClient

/** Stable façade; generated wire models remain build-only. */
class TasksClient(val baseUrl: String, private val http: OkHttpClient = OkHttpClient()) {
    fun authHeader(token: String) = "Bearer $token"
}
