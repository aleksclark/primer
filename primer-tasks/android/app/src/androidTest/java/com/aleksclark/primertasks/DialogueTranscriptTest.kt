package com.aleksclark.primertasks

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DialogueTranscriptTest {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun rendersRuledQuestionHistoryComposerAndSafeProgress() {
        var sent = 0
        composeRule.setContent {
            var draft by remember { mutableStateOf("") }
            MaterialTheme {
                DialogueTranscript(
                    occurrenceTitle = "Chapter 4 reading",
                    state = DialogueUiState(
                        connection = DialogueConnectionState.Connected,
                        phase = DialoguePhase.Ready,
                        attemptId = "attempt-1",
                        occurrenceId = "occurrence-1",
                        currentQuestion = "What changed in chapter 4?",
                        requiredCount = 3,
                        history = listOf(
                            DialogueHistoryEntry("q-1", DialogueEntryRole.Question, "What changed in chapter 4?", 1),
                            DialogueHistoryEntry("a-1", DialogueEntryRole.StudentAnswer, "The character made a difficult choice.", 2),
                        ),
                    ),
                    draft = draft,
                    onDraftChange = { draft = it },
                    onSend = { sent++ },
                    onReconnect = {},
                    onRetry = {},
                    onBack = {},
                )
            }
        }

        composeRule.onNodeWithText("DECIDE / LEARN").assertIsDisplayed()
        composeRule.onNodeWithText("What changed in chapter 4?").assertIsDisplayed()
        composeRule.onNodeWithText("The character made a difficult choice.").assertIsDisplayed()
        composeRule.onNodeWithText("0 of 3 accepted answers").assertIsDisplayed()
        composeRule.onNodeWithContentDescription("Send answer").assertIsDisplayed()
        composeRule.onNodeWithText("Send answer").assertIsDisplayed()

        composeRule.onNodeWithTag("dialogue-answer").performTextInput("The choice is supported by the character's action.")
        composeRule.onNodeWithContentDescription("Send answer").assertIsEnabled().performClick()
        assertEquals(1, sent)
    }

    @Test
    fun retryOfflineAndCompletionStatesStayGenericAndStopInput() {
        var retryClicks = 0
        composeRule.setContent {
            MaterialTheme {
                DialogueTranscript(
                    occurrenceTitle = "Chapter 4 reading",
                    state = DialogueUiState(
                        connection = DialogueConnectionState.Connected,
                        phase = DialoguePhase.Retry,
                        attemptId = "attempt-1",
                        occurrenceId = "occurrence-1",
                        currentQuestion = "What is one piece of evidence?",
                        requiredCount = 3,
                        errorMessage = "Try again with a fuller answer.",
                        retryable = true,
                    ),
                    draft = "",
                    onDraftChange = {},
                    onSend = {},
                    onReconnect = {},
                    onRetry = { retryClicks++ },
                    onBack = {},
                )
            }
        }

        composeRule.onNodeWithText("Try again with a fuller answer. Your previous answer was saved, but another answer is needed.").assertIsDisplayed()
        composeRule.onNodeWithText("Your answer").assertIsDisplayed()
        assertEquals(0, composeRule.onAllNodesWithText("hidden model reasoning").fetchSemanticsNodes().size)

        composeRule.onNodeWithText("Send answer").assertIsDisplayed()
        assertEquals(0, retryClicks)
    }

    @Test
    fun completionSummaryRemovesComposer() {
        composeRule.setContent {
            MaterialTheme {
                DialogueTranscript(
                    occurrenceTitle = "Chapter 4 reading",
                    state = DialogueUiState(
                        connection = DialogueConnectionState.Connected,
                        phase = DialoguePhase.Complete,
                        attemptId = "attempt-1",
                        occurrenceId = "occurrence-1",
                        acceptedCount = 3,
                        requiredCount = 3,
                    ),
                    draft = "",
                    onDraftChange = {},
                    onSend = {},
                    onReconnect = {},
                    onRetry = {},
                    onBack = {},
                )
            }
        }

        composeRule.onNodeWithText("Verification complete. This task is finished.").assertIsDisplayed()
        assertEquals(0, composeRule.onAllNodesWithText("Send answer").fetchSemanticsNodes().size)
    }

    @Test
    fun offlineStateExplainsDurabilityAndOffersReconnect() {
        composeRule.setContent {
            MaterialTheme {
                DialogueTranscript(
                    occurrenceTitle = "Chapter 4 reading",
                    state = DialogueUiState(connection = DialogueConnectionState.Offline),
                    draft = "",
                    onDraftChange = {},
                    onSend = {},
                    onReconnect = {},
                    onRetry = {},
                    onBack = {},
                )
            }
        }

        composeRule.onNodeWithText("Offline").assertIsDisplayed()
        composeRule.onNodeWithText("Your previous answers remain saved. Reconnect to continue the remaining questions.").assertIsDisplayed()
        composeRule.onNodeWithText("Reconnect").assertIsDisplayed()
    }
}
