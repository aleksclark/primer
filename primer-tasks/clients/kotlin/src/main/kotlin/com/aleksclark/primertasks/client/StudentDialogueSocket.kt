package com.aleksclark.primertasks.client

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener

/** Exclusive Android transport façade for the student dialogue socket.
 *
 * The bearer is sent as an Authorization header, never in a URL. The durable
 * server cursor/sequence is carried by commands so a reconnect resumes rather
 * than cancelling the detached run.
 */
class StudentDialogueSocket(
    baseUrl: String,
    private val bearer: String,
    private val listener: Listener,
    private val http: OkHttpClient = OkHttpClient(),
) {
    private val endpoint = baseUrl.trimEnd('/').replaceFirst("http", "ws") + "/student/ws"
    private val json = Json { ignoreUnknownKeys = true }
    private var socket: WebSocket? = null

    interface Listener {
        fun onEvent(event: StudentDialogueEvent)
        fun onClosed(code: Int, reason: String)
        fun onFailure(error: Throwable)
    }

    fun connect(): WebSocket {
        val request = Request.Builder().url(endpoint).header("Authorization", "Bearer $bearer").build()
        return http.newWebSocket(request, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) {
                runCatching { json.decodeFromString(StudentDialogueEvent.serializer(), text) }
                    .onSuccess(listener::onEvent)
                    .onFailure(listener::onFailure)
            }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) { listener.onClosed(code, reason) }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) { listener.onFailure(t) }
        }).also { socket = it }
    }

    fun subscribe(attemptId: String? = null, occurrenceId: String? = null, cursor: Long = 0) = send(StudentDialogueCommand.Subscribe(attemptId, occurrenceId, cursor))
    fun answer(attemptId: String, occurrenceId: String, clientMessageId: String, text: String, expectedSequence: Long) = send(StudentDialogueCommand.UserMessage(attemptId, occurrenceId, clientMessageId, text, expectedSequence))
    fun retry(attemptId: String, occurrenceId: String) = send(StudentDialogueCommand.Retry(attemptId, occurrenceId))
    fun close() { socket?.close(1000, "client closed"); socket = null }
    private fun send(command: StudentDialogueCommand) { socket?.send(json.encodeToString(StudentDialogueCommand.serializer(), command)) ?: error("student dialogue socket is not connected") }
}

@Serializable
sealed class StudentDialogueCommand {
    abstract val protocol: Int
    abstract val kind: String
    @Serializable data class Subscribe(val attemptId: String? = null, val occurrenceId: String? = null, val cursor: Long = 0, override val protocol: Int = 1, override val kind: String = "subscribe") : StudentDialogueCommand()
    @Serializable data class UserMessage(val attemptId: String, val occurrenceId: String, val clientMessageId: String, val text: String, val expectedSequence: Long, override val protocol: Int = 1, override val kind: String = "user_message") : StudentDialogueCommand()
    @Serializable data class Retry(val attemptId: String, val occurrenceId: String, override val protocol: Int = 1, override val kind: String = "retry") : StudentDialogueCommand()
}

@Serializable
data class StudentDialogueEvent(
    val protocol: Int = 1,
    val kind: String,
    val attemptId: String? = null,
    val occurrenceId: String? = null,
    val sequence: Long = 0,
    val cursor: Long = 0,
    val messageId: String? = null,
    val clientMessageId: String? = null,
    val text: String? = null,
    val role: String? = null,
    val phase: String? = null,
    val status: String? = null,
    val code: String? = null,
    val message: String? = null,
    val retryable: Boolean = false,
    val acceptedCount: Int = 0,
    val requiredCount: Int = 0,
    val questionKey: String? = null,
)
