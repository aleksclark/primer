package com.aleksclark.primer.control.tasks

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.aleksclark.primer.ui.PrimerButton
import com.aleksclark.primer.ui.PrimerButtonVariant
import com.aleksclark.primer.ui.PrimerEmptyState
import com.aleksclark.primer.ui.PrimerRecordRow
import com.aleksclark.primer.ui.PrimerRuledSurface
import com.aleksclark.primer.ui.PrimerSectionHeader
import com.aleksclark.primer.ui.PrimerStatus
import com.aleksclark.primer.ui.PrimerStatusTone
import com.aleksclark.primer.ui.PrimerQrMark
import com.aleksclark.primer.ui.PrimerTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import com.aleksclark.primertasks.client.Occurrence
import com.aleksclark.primertasks.client.Pairing
import com.aleksclark.primertasks.client.Schedule
import com.aleksclark.primertasks.client.Student
import com.aleksclark.primertasks.client.TaskRevision

@Composable
fun ControlSignInScreen(
    configured: Boolean,
    denied: Boolean,
    message: String?,
    email: String,
    password: String,
    onEmail: (String) -> Unit,
    onPassword: (String) -> Unit,
    onSignIn: () -> Unit,
    onSignOut: () -> Unit,
    signedIn: Boolean,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PrimerSectionHeader(
            label = "Primer Control",
            title = "Sign in to Primer Tasks",
            description = if (!configured) {
                "Clerk is not configured on this build. Set PRIMER_CLERK_PUBLISHABLE_KEY. Secrets are never bundled. Live Clerk acceptance is not claimed."
            } else {
                "Official Clerk Android SDK 0.1.31 password sign-in (Kotlin 2.0 compatible). Household membership is checked on the server."
            },
        )
        if (denied) PrimerStatus("Signed in, but this household does not include your account.", tone = PrimerStatusTone.Attention)
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        PrimerTextField(
            value = email,
            onValueChange = onEmail,
            label = "Email",
            enabled = configured,
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.None,
                autoCorrectEnabled = false,
                keyboardType = KeyboardType.Email,
                imeAction = ImeAction.Next,
            ),
        )
        PrimerTextField(
            value = password,
            onValueChange = onPassword,
            label = "Password",
            enabled = configured,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.None,
                autoCorrectEnabled = false,
                keyboardType = KeyboardType.Password,
                imeAction = ImeAction.Done,
            ),
        )
        PrimerButton(text = "Sign in", onClick = onSignIn, enabled = configured && email.isNotBlank() && password.isNotBlank())
        if (signedIn) PrimerButton(text = "Sign out", onClick = onSignOut, variant = PrimerButtonVariant.Quiet)
        PrimerStatus("Live Clerk acceptance is not claimed by this build.", tone = PrimerStatusTone.Neutral)
    }
}

@Composable
fun RosterScreen(
    students: List<Student>,
    query: String,
    onQuery: (String) -> Unit,
    message: String?,
    onOpen: (Student) -> Unit,
    onCreate: () -> Unit,
    onRetry: () -> Unit,
    hasMore: Boolean = false,
    onMore: (() -> Unit)? = null,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PrimerSectionHeader(label = "For parents", title = "Students", description = "Add students and issue a one-use pairing QR.") {
            PrimerButton(text = "Add student", onClick = onCreate)
        }
        PrimerTextField(value = query, onValueChange = onQuery, label = "Search students")
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        if (students.isEmpty()) {
            PrimerEmptyState(title = "No students yet", message = "Create the first student in this household, then issue a pairing QR.")
            PrimerButton(text = "Try again", onClick = onRetry, variant = PrimerButtonVariant.Secondary)
        } else {
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp), contentPadding = PaddingValues(bottom = 24.dp)) {
                items(students, key = { it.id }) { student ->
                    PrimerRecordRow(
                        label = if (student.archivedAt == null) "Ready" else "Archived",
                        value = student.displayName,
                        status = if (student.archivedAt == null) "Open" else "Archived",
                    )
                    PrimerButton(text = "Open ${student.displayName}", onClick = { onOpen(student) }, variant = PrimerButtonVariant.Quiet)
                }
                if (hasMore && onMore != null) item { PrimerButton(text = "More students", onClick = onMore, variant = PrimerButtonVariant.Secondary) }
            }
        }
    }
}

@Composable
fun StudentDetailScreen(
    student: Student?,
    pairing: Pairing?,
    name: String,
    onName: (String) -> Unit,
    message: String?,
    onSave: () -> Unit,
    onArchive: () -> Unit,
    onIssueQr: () -> Unit,
    onBack: () -> Unit,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PrimerSectionHeader(label = "For parents", title = student?.displayName ?: "New student", trailing = { PrimerButton(text = "Back", onClick = onBack, variant = PrimerButtonVariant.Quiet) })
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        PrimerTextField(value = name, onValueChange = onName, label = "Display name")
        PrimerButton(text = if (student == null) "Create student" else "Save name", onClick = onSave, enabled = name.isNotBlank())
        if (student != null && student.archivedAt == null) PrimerButton(text = "Archive student", onClick = onArchive, variant = PrimerButtonVariant.Attention)
        PrimerButton(text = if (pairing == null) "Issue pairing QR" else "Issue a new code", onClick = onIssueQr, enabled = student != null && student.archivedAt == null)
        if (pairing != null) {
            PrimerRuledSurface(Modifier.fillMaxWidth()) {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    PrimerStatus("Show once", tone = PrimerStatusTone.Accent)
                    PrimerQrMark(
                        payload = pairing.qrPayload,
                        contentDescription = "Pairing QR for ${student?.displayName ?: "student"}. Expires ${pairing.expiresAt}.",
                    )
                    androidx.compose.material3.Text(pairing.code, style = com.aleksclark.primer.ui.PrimerTheme.typography.title)
                    androidx.compose.material3.Text("Expires ${pairing.expiresAt}. Keep this page open while the student scans. Sign-out clears this code.", style = com.aleksclark.primer.ui.PrimerTheme.typography.small)
                }
            }
        }
    }
}

@Composable
fun TasksScreen(
    tasks: List<TaskRevision>,
    query: String,
    onQuery: (String) -> Unit,
    message: String?,
    onCreate: () -> Unit,
    onPublish: (TaskRevision) -> Unit,
    onArchive: (TaskRevision) -> Unit,
    onEdit: (TaskRevision) -> Unit,
    hasMore: Boolean = false,
    onMore: (() -> Unit)? = null,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PrimerSectionHeader(label = "For parents", title = "Tasks", trailing = { PrimerButton(text = "Create task", onClick = onCreate) })
        PrimerTextField(value = query, onValueChange = onQuery, label = "Search tasks")
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        if (tasks.isEmpty()) PrimerEmptyState(title = "No tasks match", message = "Create a task or try a different search.")
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(tasks, key = { it.id }) { task ->
                PrimerRecordRow(label = task.status, value = task.title, status = task.templateStatus ?: task.status)
                PrimerButton(text = "Edit", onClick = { onEdit(task) }, variant = PrimerButtonVariant.Quiet)
                if (task.status == "draft") PrimerButton(text = "Publish", onClick = { onPublish(task) })
                if (task.templateStatus != "retired") PrimerButton(text = "Archive", onClick = { onArchive(task) }, variant = PrimerButtonVariant.Attention)
            }
            if (hasMore && onMore != null) item { PrimerButton(text = "More tasks", onClick = onMore, variant = PrimerButtonVariant.Secondary) }
        }
    }
}

@Composable
fun TaskEditorScreen(
    title: String,
    instructions: String,
    editing: Boolean,
    creating: Boolean,
    onTitle: (String) -> Unit,
    onInstructions: (String) -> Unit,
    onSave: () -> Unit,
    onClose: () -> Unit,
    message: String?,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PrimerSectionHeader(label = "For parents", title = if (creating) "Create a task" else if (editing) "Edit task" else "Create a task")
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        PrimerTextField(value = title, onValueChange = onTitle, label = "Task title")
        PrimerTextField(value = instructions, onValueChange = onInstructions, label = "Instructions", singleLine = false)
        PrimerButton(text = if (editing) "Save new draft" else "Create draft", onClick = onSave, enabled = title.isNotBlank())
        PrimerButton(text = "Cancel", onClick = onClose, variant = PrimerButtonVariant.Quiet)
    }
}

@Composable
fun SchedulesScreen(
    schedules: List<Schedule>,
    message: String?,
    onCreate: () -> Unit,
    onEdit: (Schedule) -> Unit,
    onCancel: (Schedule) -> Unit,
    hasMore: Boolean = false,
    onMore: (() -> Unit)? = null,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PrimerSectionHeader(label = "For parents", title = "Schedules", trailing = { PrimerButton(text = "Schedule a task", onClick = onCreate) })
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        if (schedules.isEmpty()) PrimerEmptyState(title = "No schedules match", message = "Publish a task, then schedule it for a student.")
        LazyColumn {
            items(schedules, key = { it.id }) { schedule ->
                PrimerRecordRow(
                    label = schedule.studentName ?: "Student",
                    value = schedule.title ?: "Task",
                    status = if (schedule.enabled == true) "Active" else "Canceled",
                )
                if (schedule.enabled == true) {
                    PrimerButton(text = "Edit schedule", onClick = { onEdit(schedule) }, variant = PrimerButtonVariant.Secondary)
                    PrimerButton(text = "Cancel schedule", onClick = { onCancel(schedule) }, variant = PrimerButtonVariant.Attention)
                }
            }
            if (hasMore && onMore != null) item { PrimerButton(text = "More schedules", onClick = onMore, variant = PrimerButtonVariant.Secondary) }
        }
    }
}

@Composable
fun ScheduleEditorScreen(
    draft: ScheduleDraft,
    students: List<Student>,
    tasks: List<TaskRevision>,
    studentId: String,
    taskId: String,
    studentQuery: String,
    taskQuery: String,
    editing: Boolean,
    onStudent: (String) -> Unit,
    onTask: (String) -> Unit,
    onStudentQuery: (String) -> Unit,
    onTaskQuery: (String) -> Unit,
    onMoreStudents: () -> Unit,
    onMoreTasks: () -> Unit,
    studentsHasMore: Boolean,
    tasksHasMore: Boolean,
    onKind: (String) -> Unit,
    onDate: (String) -> Unit,
    onTime: (String) -> Unit,
    onTimezone: (String) -> Unit,
    onRrule: (String) -> Unit,
    onDueOffset: (String) -> Unit,
    onEndAt: (String) -> Unit,
    onSave: () -> Unit,
    onClose: () -> Unit,
    message: String?,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        PrimerSectionHeader(label = "For parents", title = if (editing) "Edit schedule" else "Schedule a task")
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        PrimerTextField(value = studentQuery, onValueChange = onStudentQuery, label = "Search students")
        students.forEach { student ->
            val selected = student.id == studentId
            PrimerButton(
                text = if (selected) "Selected ${student.displayName}" else student.displayName,
                onClick = { onStudent(student.id) },
                variant = if (selected) PrimerButtonVariant.Secondary else PrimerButtonVariant.Quiet,
            )
        }
        if (studentsHasMore) PrimerButton(text = "More students", onClick = onMoreStudents, variant = PrimerButtonVariant.Secondary)
        PrimerTextField(value = taskQuery, onValueChange = onTaskQuery, label = "Search published tasks")
        val published = tasks.filter { it.status == "published" || it.templateStatus == "published" }
        published.forEach { task ->
            val selected = task.id == taskId
            PrimerButton(
                text = if (selected) "Selected ${task.title}" else task.title,
                onClick = { onTask(task.id) },
                variant = if (selected) PrimerButtonVariant.Secondary else PrimerButtonVariant.Quiet,
            )
        }
        if (tasksHasMore) PrimerButton(text = "More published tasks", onClick = onMoreTasks, variant = PrimerButtonVariant.Secondary)
        PrimerTextField(value = draft.kind, onValueChange = onKind, label = "Kind (one_off, daily, weekly, recurrence)")
        PrimerTextField(value = draft.date, onValueChange = onDate, label = "Start date (YYYY-MM-DD)")
        PrimerTextField(value = draft.time, onValueChange = onTime, label = "Time (HH:MM)")
        PrimerTextField(value = draft.timezone, onValueChange = onTimezone, label = "IANA timezone")
        PrimerTextField(value = draft.rrule, onValueChange = onRrule, label = "RRULE (optional)")
        PrimerTextField(value = draft.dueOffsetMinutes.toString(), onValueChange = onDueOffset, label = "Due offset minutes")
        PrimerTextField(value = draft.endAt, onValueChange = onEndAt, label = "End at (optional RFC3339)")
        PrimerButton(text = "Save schedule", onClick = onSave)
        PrimerButton(text = "Cancel", onClick = onClose, variant = PrimerButtonVariant.Quiet)
    }
}

@Composable
fun ReviewScreen(
    occurrences: List<Occurrence>,
    message: String?,
    onApprove: (Occurrence) -> Unit,
    onReject: (Occurrence) -> Unit,
    onRetry: (Occurrence) -> Unit,
    onOpen: (Occurrence) -> Unit,
    hasMore: Boolean = false,
    onMore: (() -> Unit)? = null,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        PrimerSectionHeader(label = "For parents", title = "Assigned work")
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        if (occurrences.isEmpty()) PrimerEmptyState(title = "No assigned work", message = "Publish a task and create a schedule to add work here.")
        LazyColumn {
            items(occurrences, key = { it.id }) { occurrence ->
                PrimerRecordRow(
                    label = occurrence.studentName ?: "Student",
                    value = occurrence.title,
                    status = occurrence.status,
                )
                PrimerButton(text = "Open", onClick = { onOpen(occurrence) }, variant = PrimerButtonVariant.Quiet)
                if (occurrence.status == "awaiting_verification") {
                    PrimerButton(text = "Approve", onClick = { onApprove(occurrence) })
                    PrimerButton(text = "Reject", onClick = { onReject(occurrence) }, variant = PrimerButtonVariant.Attention)
                }
                if (occurrence.status == "pending") PrimerButton(text = "Retry", onClick = { onRetry(occurrence) }, variant = PrimerButtonVariant.Secondary)
            }
            if (hasMore && onMore != null) item { PrimerButton(text = "More assigned work", onClick = onMore, variant = PrimerButtonVariant.Secondary) }
        }
    }
}

@Composable
fun OccurrenceDetailScreen(
    occurrence: Occurrence,
    reason: String,
    onReason: (String) -> Unit,
    message: String?,
    onApprove: () -> Unit,
    onReject: () -> Unit,
    onRetry: () -> Unit,
    onSkip: () -> Unit,
    onCancel: () -> Unit,
    onBack: () -> Unit,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        PrimerSectionHeader(label = "Assigned work", title = occurrence.title, trailing = { PrimerButton(text = "Back", onClick = onBack, variant = PrimerButtonVariant.Quiet) })
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        PrimerRecordRow(label = "Status", value = occurrence.status)
        PrimerRecordRow(label = "Student", value = occurrence.studentName ?: occurrence.studentId)
        PrimerRecordRow(label = "Instructions", value = occurrence.instructions)
        PrimerTextField(value = reason, onValueChange = onReason, label = "Decision reason")
        if (occurrence.status == "awaiting_verification") {
            PrimerButton(text = "Approve", onClick = onApprove)
            PrimerButton(text = "Reject", onClick = onReject, variant = PrimerButtonVariant.Attention)
        }
        if (occurrence.status == "pending") PrimerButton(text = "Retry", onClick = onRetry)
        PrimerButton(text = "Skip", onClick = onSkip, variant = PrimerButtonVariant.Secondary)
        PrimerButton(text = "Cancel", onClick = onCancel, variant = PrimerButtonVariant.Attention)
    }
}
