package com.aleksclark.primer.student.tasks

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.tooling.preview.PreviewParameter
import androidx.compose.ui.tooling.preview.PreviewParameterProvider
import com.aleksclark.primer.ui.PrimerTheme
import com.aleksclark.primertasks.client.OccurrenceResponse

private class TasksOccurrencePreviewProvider : PreviewParameterProvider<OccurrenceResponse> {
    override val values = sequenceOf(
        TasksMockCases.notStarted,
        TasksMockCases.inProgress,
        TasksMockCases.waitingForParent,
        TasksMockCases.rejectedRetry,
        TasksMockCases.completed,
        TasksMockCases.unsupported,
    )
}

@Composable
private fun PreviewFrame(darkTheme: Boolean, content: @Composable () -> Unit) {
    PrimerTheme(darkTheme = darkTheme) {
        Surface(modifier = Modifier.fillMaxSize(), color = PrimerTheme.colors.surface, content = content)
    }
}

@Preview(name = "Pairing dark", widthDp = 390, heightDp = 844, showBackground = true)
@Composable
private fun PairingDarkPreview() {
    PreviewFrame(darkTheme = true) {
        PairingScreen(message = null, onScan = {}, onImportImage = {})
    }
}

@Preview(name = "Pairing light", widthDp = 390, heightDp = 844, showBackground = true)
@Composable
private fun PairingLightPreview() {
    PreviewFrame(darkTheme = false) {
        PairingScreen(message = null, onScan = {}, onImportImage = {})
    }
}

@Preview(name = "Checklist dark", widthDp = 390, heightDp = 844, showBackground = true)
@Composable
private fun ChecklistDarkPreview() {
    PreviewFrame(darkTheme = true) {
        ChecklistScreen(
            name = "Ada",
            items = TasksMockCases.checklistItems,
            occurrences = TasksMockCases.today,
            upcoming = TasksMockCases.upcoming,
            message = null,
            onOpen = {},
            onRefresh = {},
            onBack = null,
        )
    }
}

@Preview(name = "Occurrence states", widthDp = 390, heightDp = 844, showBackground = true)
@Composable
private fun OccurrenceStatePreview(@PreviewParameter(TasksOccurrencePreviewProvider::class) occurrence: OccurrenceResponse) {
    PreviewFrame(darkTheme = true) {
        OccurrenceDetailScreen(
            occurrence = occurrence,
            message = null,
            onBack = {},
            onRefresh = {},
            onStart = {},
            onSubmit = {},
        )
    }
}

@Preview(name = "Unavailable", widthDp = 390, heightDp = 844, showBackground = true)
@Composable
private fun UnavailablePreview() {
    PreviewFrame(darkTheme = true) {
        UnavailableOccurrenceScreen(onBack = {})
    }
}
