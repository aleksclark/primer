package com.aleksclark.primer.student.tasks

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import com.aleksclark.primer.ui.PrimerRuledSurface
import com.aleksclark.primer.ui.PrimerStatus
import com.aleksclark.primer.ui.PrimerStatusTone
import com.aleksclark.primer.ui.PrimerTheme

data class StudentDashboardUi(
    val snapshot: StudentDashboardSnapshot?,
    val loading: Boolean,
)

@Composable
fun rememberStudentDashboardUi(enabled: Boolean, refreshKey: Any? = null): StudentDashboardUi {
    val context = LocalContext.current
    val session = remember { TasksSession(context) }
    var snapshot by remember { mutableStateOf<StudentDashboardSnapshot?>(null) }
    var loading by remember { mutableStateOf(enabled) }
    LaunchedEffect(enabled, refreshKey, session) {
        if (!enabled) {
            loading = false
            return@LaunchedEffect
        }
        loading = true
        snapshot = studentDashboardSnapshot(session.restore())
        loading = false
    }
    return StudentDashboardUi(snapshot, loading)
}

@Composable
fun StudentTasksDashboardCard(
    snapshot: StudentDashboardSnapshot?,
    loading: Boolean,
    onOpenTasks: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val name = snapshot?.greetingName ?: "Student"
    val summary = when {
        loading && snapshot == null -> "Loading today's tasks."
        snapshot == null -> "Today's tasks are unavailable."
        else -> snapshot.pendingSummary
    }
    val pending = snapshot?.pendingToday.orEmpty()
    PrimerRuledSurface(
        modifier = modifier
            .fillMaxWidth()
            .clickable(role = Role.Button, onClick = onOpenTasks)
            .semantics { contentDescription = "Open today's tasks" },
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(PrimerTheme.spacing.sm)) {
            Text("TASKS", style = PrimerTheme.typography.label, color = PrimerTheme.colors.textMuted)
            Text(name, style = PrimerTheme.typography.sectionTitle, color = PrimerTheme.colors.text)
            Text(summary, style = PrimerTheme.typography.body, color = PrimerTheme.colors.text)
            pending.take(4).forEach { task ->
                Text(
                    "${task.title} · ${task.statusLabel}",
                    style = PrimerTheme.typography.body,
                    color = PrimerTheme.colors.textMuted,
                )
            }
            if (pending.size > 4) {
                Text(
                    "+${pending.size - 4} more",
                    style = PrimerTheme.typography.small,
                    color = PrimerTheme.colors.textMuted,
                )
            }
            if (snapshot?.message != null) {
                PrimerStatus(snapshot.message, tone = PrimerStatusTone.Attention)
            }
            Text("OPEN TASKS", style = PrimerTheme.typography.button, color = PrimerTheme.colors.accent)
        }
    }
}
