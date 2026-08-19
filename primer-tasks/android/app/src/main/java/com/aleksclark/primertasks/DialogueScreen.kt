package com.aleksclark.primertasks

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.collectAsState
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.compose.ui.platform.LocalLifecycleOwner
import com.aleksclark.primertasks.client.StudentDialogueEvent
import com.aleksclark.primertasks.client.StudentDialogueSocket
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import java.util.UUID

/** Connection state deliberately describes delivery, not whether the server run exists. */
enum class DialogueConnectionState { Idle, Connecting, Connected, Offline }

enum class DialoguePhase { Ready, Thinking, Evaluating, Retry, Error, Complete }

enum class DialogueEntryRole { Question, StudentAnswer }

data class DialogueHistoryEntry(
    val id: String,
    val role: DialogueEntryRole,
    val text: String,
    val sequence: Long,
)

data class DialogueUiState(
    val connection: DialogueConnectionState = DialogueConnectionState.Idle,
    val phase: DialoguePhase = DialoguePhase.Ready,
    val attemptId: String? = null,
    val occurrenceId: String? = null,
    val cursor: Long = 0,
    val acceptedCount: Int = 0,
    val requiredCount: Int = 0,
    val currentQuestion: String? = null,
    val history: List<DialogueHistoryEntry> = emptyList(),
    val errorMessage: String? = null,
    val retryable: Boolean = false,
) {
    val isComplete: Boolean get() = phase == DialoguePhase.Complete
    val canAnswer: Boolean
        get() = connection == DialogueConnectionState.Connected &&
            attemptId != null &&
            !currentQuestion.isNullOrBlank() &&
            (phase == DialoguePhase.Ready || phase == DialoguePhase.Retry)
}

/**
 * Pure protocol reducer. Events are replayed from the server after every
 * reconnect, so IDs and sequence numbers are used to make the projection
 * idempotent. The reducer intentionally has no fields for model output,
 * scores, or reasoning.
 */
internal object DialogueReducer {
    fun reduce(previous: DialogueUiState, event: StudentDialogueEvent): DialogueUiState {
        if (event.sequence > 0 && event.sequence <= previous.cursor && event.kind != "state") return previous
        var next = previous.copy(
            cursor = maxOf(previous.cursor, event.cursor, event.sequence),
            attemptId = event.attemptId ?: previous.attemptId,
            occurrenceId = event.occurrenceId ?: previous.occurrenceId,
        )
        when (event.kind) {
            "hello" -> next = next.copy(connection = DialogueConnectionState.Connected)
            "state" -> next = next.copy(
                phase = phaseForStatus(event.status, next.phase),
                acceptedCount = event.acceptedCount,
                requiredCount = event.requiredCount,
                errorMessage = null,
                retryable = false,
            )
            "message_ack" -> {
                val id = event.messageId ?: event.clientMessageId ?: "answer-${event.sequence}"
                if (next.history.none { it.id == id }) {
                    next = next.copy(history = next.history + DialogueHistoryEntry(
                        id = id,
                        role = DialogueEntryRole.StudentAnswer,
                        text = event.text.orEmpty(),
                        sequence = event.sequence,
                    ))
                }
                next = next.copy(phase = DialoguePhase.Evaluating, errorMessage = null, retryable = false)
            }
            "progress" -> when (event.phase) {
                "evaluating" -> next = next.copy(phase = DialoguePhase.Evaluating, errorMessage = null, retryable = false)
                "retry" -> next = next.copy(phase = DialoguePhase.Retry, errorMessage = null, retryable = true)
                "complete" -> next = next.copy(phase = DialoguePhase.Complete, errorMessage = null, retryable = false)
            }
            "question" -> {
                val text = event.text?.trim().orEmpty()
                if (text.isNotEmpty()) {
                    val id = event.messageId ?: "question-${event.questionKey ?: event.sequence}"
                    if (next.history.none { it.id == id }) {
                        next = next.copy(history = next.history + DialogueHistoryEntry(
                            id = id,
                            role = DialogueEntryRole.Question,
                            text = text,
                            sequence = event.sequence,
                        ))
                    }
                    next = next.copy(currentQuestion = text, phase = DialoguePhase.Ready, errorMessage = null, retryable = false)
                }
            }
            "answer_evaluation" -> {
                val accepted = if (event.acceptedCount == 0 && next.acceptedCount > 0) next.acceptedCount else event.acceptedCount
                val complete = event.status == "accepted" ||
                    (event.requiredCount > 0 && accepted >= event.requiredCount)
                next = next.copy(
                    acceptedCount = accepted,
                    requiredCount = maxOf(next.requiredCount, event.requiredCount),
                    phase = if (complete) DialoguePhase.Complete else if (event.status == "rejected") DialoguePhase.Retry else DialoguePhase.Thinking,
                    errorMessage = null,
                    retryable = event.status == "rejected",
                )
            }
            "complete" -> next = next.copy(
                phase = DialoguePhase.Complete,
                acceptedCount = maxOf(next.acceptedCount, event.acceptedCount),
                requiredCount = maxOf(next.requiredCount, event.requiredCount),
                errorMessage = null,
                retryable = false,
            )
            "error" -> next = next.copy(
                phase = DialoguePhase.Error,
                errorMessage = safeErrorMessage(event.code),
                retryable = event.retryable,
            )
        }
        return next
    }

    private fun phaseForStatus(status: String?, prior: DialoguePhase): DialoguePhase = when (status) {
        "accepted", "completed", "complete", "succeeded" -> DialoguePhase.Complete
        "failed", "error" -> DialoguePhase.Error
        "retry" -> DialoguePhase.Retry
        "open", "awaiting_verification", "in_progress" -> prior.takeUnless { it == DialoguePhase.Error || it == DialoguePhase.Complete } ?: DialoguePhase.Ready
        else -> prior
    }

    internal fun safeErrorMessage(code: String?): String = when (code) {
        "conflict" -> "Another device is already answering this turn. Reconnect to continue."
        "not_found" -> "This verification is unavailable. Return to the task list."
        "revoked" -> "This device pairing is no longer active."
        "invalid_request", "protocol_version" -> "That answer could not be sent. Try again."
        else -> "The verifier could not finish this turn. Your answer is saved."
    }
}

/** Owns the Android socket and the durable replay cursor for one occurrence. */
class DialogueSession(
    private val baseUrl: String,
    private val bearer: String,
    private val occurrenceId: String,
) {
    private val _state = MutableStateFlow(DialogueUiState(occurrenceId = occurrenceId))
    val state: StateFlow<DialogueUiState> = _state
    private var socket: StudentDialogueSocket? = null
    private var closed = false

    fun connect() {
        if (closed || socket != null) return
        _state.value = _state.value.copy(connection = DialogueConnectionState.Connecting, errorMessage = null)
        val newSocket = StudentDialogueSocket(baseUrl, bearer, object : StudentDialogueSocket.Listener {
            override fun onEvent(event: StudentDialogueEvent) {
                _state.value = DialogueReducer.reduce(_state.value, event)
            }

            override fun onClosed(code: Int, reason: String) {
                if (!closed) {
                    socket = null
                    _state.value = _state.value.copy(connection = DialogueConnectionState.Offline)
                }
            }

            override fun onFailure(error: Throwable) {
                if (!closed) {
                    socket = null
                    _state.value = _state.value.copy(
                        connection = DialogueConnectionState.Offline,
                        phase = if (_state.value.isComplete) DialoguePhase.Complete else _state.value.phase,
                        errorMessage = "The connection was lost. Your previous answers remain saved.",
                        retryable = true,
                    )
                }
            }
        })
        socket = newSocket
        runCatching {
            newSocket.connect()
            // Occurrence binding is server-authorized. No attempt ID is guessed
            // or placed in the URL; the server returns the current durable attempt.
            newSocket.subscribe(occurrenceId = occurrenceId, cursor = _state.value.cursor)
        }.onFailure {
            socket = null
            _state.value = _state.value.copy(connection = DialogueConnectionState.Offline, errorMessage = "Unable to connect. Your previous answers remain saved.", retryable = true)
        }
    }

    fun sendAnswer(text: String): Boolean {
        val state = _state.value
        val attempt = state.attemptId ?: return false
        if (!state.canAnswer || text.isBlank()) return false
        val clientMessageId = UUID.randomUUID().toString()
        return runCatching {
            socket?.answer(attempt, occurrenceId, clientMessageId, text.trim(), expectedSequence = 0)
                ?: error("not connected")
        }.onSuccess {
            // The durable message is rendered only after message_ack. If the
            // app dies before that event, replay supplies it on the next open.
            _state.value = _state.value.copy(phase = DialoguePhase.Evaluating, errorMessage = null)
        }.onFailure {
            _state.value = _state.value.copy(connection = DialogueConnectionState.Offline, errorMessage = "Your answer could not be sent. Reconnect to continue.", retryable = true)
        }.isSuccess
    }

    fun retryEvaluation(): Boolean {
        val state = _state.value
        val attempt = state.attemptId ?: return false
        return runCatching {
            socket?.retry(attempt, occurrenceId) ?: error("not connected")
        }.onSuccess {
            _state.value = _state.value.copy(phase = DialoguePhase.Evaluating, errorMessage = null, retryable = false)
        }.onFailure {
            _state.value = _state.value.copy(connection = DialogueConnectionState.Offline, errorMessage = "Unable to retry while offline.", retryable = true)
        }.isSuccess
    }

    fun disconnectForBackground() {
        socket?.close()
        socket = null
        if (!closed && !_state.value.isComplete) _state.value = _state.value.copy(connection = DialogueConnectionState.Offline)
    }

    fun close() {
        closed = true
        socket?.close()
        socket = null
    }
}

@Composable
fun DialogueScreen(
    occurrenceTitle: String,
    occurrenceId: String,
    baseUrl: String,
    bearer: String,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val lifecycleOwner = LocalLifecycleOwner.current
    val session = remember(baseUrl, bearer, occurrenceId) { DialogueSession(baseUrl, bearer, occurrenceId) }
    val state by session.state.collectAsState()
    var draft by remember { mutableStateOf("") }

    DisposableEffect(lifecycleOwner, session) {
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_START -> session.connect()
                Lifecycle.Event.ON_STOP -> session.disconnectForBackground()
                else -> Unit
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        session.connect()
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
            session.close()
        }
    }

    DialogueTranscript(
        occurrenceTitle = occurrenceTitle,
        state = state,
        draft = draft,
        onDraftChange = { draft = it },
        onSend = {
            if (session.sendAnswer(draft)) draft = ""
        },
        onReconnect = { session.connect() },
        onRetry = { session.retryEvaluation() },
        onBack = onBack,
        modifier = modifier,
    )
}

/** Stateless surface used by the app and by Compose tests. */
@Composable
fun DialogueTranscript(
    occurrenceTitle: String,
    state: DialogueUiState,
    draft: String,
    onDraftChange: (String) -> Unit,
    onSend: () -> Unit,
    onReconnect: () -> Unit,
    onRetry: () -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Column(Modifier.weight(1f)) {
                Text("DECIDE / LEARN", style = MaterialTheme.typography.labelLarge)
                Text(occurrenceTitle, style = MaterialTheme.typography.headlineMedium)
            }
            OutlinedButton(onClick = onBack) { Text("Back") }
        }
        Text(connectionLabel(state.connection), style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.semantics { contentDescription = "Dialogue connection ${connectionLabel(state.connection)}" })

        if (state.connection == DialogueConnectionState.Offline) {
            Text("Your previous answers remain saved. Reconnect to continue the remaining questions.", color = MaterialTheme.colorScheme.error)
            Button(onClick = onReconnect) { Text("Reconnect") }
        }
        if (state.phase == DialoguePhase.Error) {
            Text(state.errorMessage ?: DialogueReducer.safeErrorMessage(null), color = MaterialTheme.colorScheme.error)
            if (state.retryable && state.connection == DialogueConnectionState.Connected) {
                OutlinedButton(onClick = onRetry) { Text("Retry verification") }
            }
        }
        if (state.phase == DialoguePhase.Retry) {
            Text("Try again with a fuller answer. Your previous answer was saved, but another answer is needed.", color = MaterialTheme.colorScheme.error)
        }
        if (state.phase == DialoguePhase.Thinking || state.phase == DialoguePhase.Evaluating || state.connection == DialogueConnectionState.Connecting) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                CircularProgressIndicator(modifier = Modifier.height(18.dp), strokeWidth = 2.dp)
                Text(if (state.phase == DialoguePhase.Evaluating) "Evaluating" else "Thinking")
            }
        }

        if (state.requiredCount > 0) {
            Text("${state.acceptedCount} of ${state.requiredCount} accepted answers", style = MaterialTheme.typography.labelMedium)
        }

        val currentQuestionIndex = state.currentQuestion?.let { question ->
            state.history.indexOfLast { it.role == DialogueEntryRole.Question && it.text == question }
        } ?: -1
        val visibleHistory = state.history.filterIndexed { index, _ -> index != currentQuestionIndex }
        LazyColumn(
            modifier = Modifier.weight(1f).fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(0.dp),
        ) {
            if (state.currentQuestion != null) {
                item(key = "current-question") {
                    DialogueHistoryRow(DialogueHistoryEntry("current-question", DialogueEntryRole.Question, state.currentQuestion, state.cursor))
                    HorizontalDivider()
                }
            }
            if (visibleHistory.isEmpty() && state.currentQuestion == null) {
                item { Text("Waiting for the first question.", modifier = Modifier.padding(vertical = 20.dp)) }
            }
            items(visibleHistory, key = { it.id }) { entry ->
                DialogueHistoryRow(entry)
                HorizontalDivider()
            }
        }

        if (state.isComplete) {
            Text("Verification complete. This task is finished.", style = MaterialTheme.typography.titleMedium)
        } else {
            OutlinedTextField(
                value = draft,
                onValueChange = onDraftChange,
                modifier = Modifier.fillMaxWidth().testTag("dialogue-answer"),
                label = { Text("Your answer") },
                placeholder = { Text(if (state.currentQuestion == null) "Waiting for the question" else "Write a complete answer") },
                enabled = state.canAnswer,
                minLines = 3,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Default),
            )
            Text("Answers are saved before the verifier evaluates them.", style = MaterialTheme.typography.bodySmall)
            Button(
                onClick = onSend,
                enabled = state.canAnswer && draft.isNotBlank(),
                modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Send answer" },
            ) { Text("Send answer") }
        }
    }
}

@Composable
private fun DialogueHistoryRow(entry: DialogueHistoryEntry) {
    Column(Modifier.fillMaxWidth().padding(vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(5.dp)) {
        Text(if (entry.role == DialogueEntryRole.Question) "Question" else "Your answer", style = MaterialTheme.typography.labelMedium)
        Text(entry.text, style = MaterialTheme.typography.bodyLarge)
    }
}

private fun connectionLabel(connection: DialogueConnectionState): String = when (connection) {
    DialogueConnectionState.Idle -> "Preparing verification"
    DialogueConnectionState.Connecting -> "Connecting"
    DialogueConnectionState.Connected -> "Connected"
    DialogueConnectionState.Offline -> "Offline"
}
