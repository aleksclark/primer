package com.aleksclark.primertasks

import com.aleksclark.primertasks.client.StudentDialogueEvent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DialogueScreenTest {
    @Test
    fun replayBuildsQuestionAndAnswerHistoryAndIsIdempotent() {
        var state = DialogueUiState(occurrenceId = "occ-1")
        state = DialogueReducer.reduce(state, StudentDialogueEvent(kind = "state", attemptId = "attempt-1", occurrenceId = "occ-1", requiredCount = 3, sequence = 0, cursor = 0, status = "open"))
        state = DialogueReducer.reduce(state, StudentDialogueEvent(kind = "question", attemptId = "attempt-1", occurrenceId = "occ-1", messageId = "q-1", questionKey = "q1", text = "What changed in chapter 4?", sequence = 1, cursor = 1))
        val answer = StudentDialogueEvent(kind = "message_ack", attemptId = "attempt-1", occurrenceId = "occ-1", messageId = "m-1", clientMessageId = "client-1", text = "The character made a difficult choice.", sequence = 2, cursor = 2)
        state = DialogueReducer.reduce(state, answer)
        state = DialogueReducer.reduce(state, answer)

        assertEquals("attempt-1", state.attemptId)
        assertEquals("What changed in chapter 4?", state.currentQuestion)
        assertEquals(2, state.history.size)
        assertEquals(DialogueEntryRole.StudentAnswer, state.history.last().role)
        assertEquals(DialoguePhase.Evaluating, state.phase)
        assertEquals(2L, state.cursor)
    }

    @Test
    fun reconnectOrProcessDeathReplaysDurableProjectionFromCursorZero() {
        val durableReplay = listOf(
            StudentDialogueEvent(kind = "state", attemptId = "attempt-1", occurrenceId = "occ-1", requiredCount = 3, sequence = 0, cursor = 0, status = "open"),
            StudentDialogueEvent(kind = "question", attemptId = "attempt-1", occurrenceId = "occ-1", messageId = "q-1", text = "What changed?", sequence = 1, cursor = 1),
            StudentDialogueEvent(kind = "message_ack", attemptId = "attempt-1", occurrenceId = "occ-1", messageId = "m-1", text = "The character chose to act.", sequence = 2, cursor = 2),
        )

        // A new process has no in-memory transcript. Replaying the durable
        // stream from cursor zero reconstructs the same attempt and history.
        var restored = DialogueUiState(occurrenceId = "occ-1")
        durableReplay.forEach { restored = DialogueReducer.reduce(restored, it) }
        assertEquals("attempt-1", restored.attemptId)
        assertEquals(2, restored.history.size)
        assertEquals(2L, restored.cursor)

        // A reconnect that receives an already-seen event does not duplicate it.
        restored = DialogueReducer.reduce(restored, durableReplay[2])
        assertEquals(2, restored.history.size)
    }

    @Test
    fun retryAndCompletionExposeOnlySafeStudentProgress() {
        var state = DialogueUiState(connection = DialogueConnectionState.Connected, attemptId = "a", occurrenceId = "o", requiredCount = 3)
        state = DialogueReducer.reduce(state, StudentDialogueEvent(kind = "question", text = "Give one piece of evidence.", messageId = "q", sequence = 1, cursor = 1))
        state = DialogueReducer.reduce(state, StudentDialogueEvent(kind = "answer_evaluation", status = "rejected", acceptedCount = 0, requiredCount = 3, sequence = 2, cursor = 2))
        assertEquals(DialoguePhase.Retry, state.phase)
        assertTrue(state.canAnswer)

        state = DialogueReducer.reduce(state, StudentDialogueEvent(kind = "complete", acceptedCount = 3, requiredCount = 3, sequence = 3, cursor = 3))
        assertTrue(state.isComplete)
        assertFalse(state.canAnswer)
        assertEquals(3, state.acceptedCount)
    }

    @Test
    fun providerFailureBecomesGenericRetryableErrorWithoutReasoning() {
        val state = DialogueReducer.reduce(
            DialogueUiState(attemptId = "a", occurrenceId = "o"),
            StudentDialogueEvent(
                kind = "error",
                code = "internal",
                message = "hidden chain of thought: do not show this",
                retryable = true,
                sequence = 4,
                cursor = 4,
            ),
        )

        assertEquals(DialoguePhase.Error, state.phase)
        assertTrue(state.retryable)
        assertEquals("The verifier could not finish this turn. Your answer is saved.", state.errorMessage)
        assertFalse(state.errorMessage!!.contains("chain of thought"))
    }
}
