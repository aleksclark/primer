package com.aleksclark.primer.control

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.aleksclark.primer.control.device.DeviceDetailScreen
import com.aleksclark.primer.control.device.DevicesScreen
import com.aleksclark.primer.control.tasks.ControlSignInScreen
import com.aleksclark.primer.control.tasks.OccurrenceDetailScreen
import com.aleksclark.primer.control.tasks.ReviewScreen
import com.aleksclark.primer.control.tasks.RosterScreen
import com.aleksclark.primer.control.tasks.ScheduleEditorScreen
import com.aleksclark.primer.control.tasks.SchedulesScreen
import com.aleksclark.primer.control.tasks.StudentDetailScreen
import com.aleksclark.primer.control.tasks.TaskEditorScreen
import com.aleksclark.primer.control.tasks.TasksScreen
import com.aleksclark.primer.identity.ControlOriginPolicy
import com.aleksclark.primer.ui.PrimerButton
import com.aleksclark.primer.ui.PrimerButtonVariant
import com.aleksclark.primer.ui.PrimerStatus
import com.aleksclark.primer.ui.PrimerStatusTone
import com.aleksclark.primer.ui.PrimerTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val app = application as ControlApp
        val apiBase = ControlOriginPolicy.apiBase(BuildConfig.CONFIGURED_API_ORIGIN, BuildConfig.DEBUG)
        setContent {
            PrimerTheme {
                val model: ControlViewModel = viewModel(
                    factory = object : ViewModelProvider.Factory {
                        @Suppress("UNCHECKED_CAST")
                        override fun <T : ViewModel> create(modelClass: Class<T>): T {
                            return ControlViewModel(app.identity, apiBase) as T
                        }
                    },
                )
                ControlAppScreen(
                    model = model,
                    apiBase = apiBase,
                    clerkConfigured = BuildConfig.CLERK_PUBLISHABLE_KEY.isNotBlank(),
                    originConfigured = apiBase != null,
                )
            }
        }
    }
}

@Composable
private fun ControlAppScreen(
    model: ControlViewModel,
    apiBase: String?,
    clerkConfigured: Boolean,
    originConfigured: Boolean,
) {
    val state by model.state.collectAsStateWithLifecycle()
    if (!state.ready) {
        Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center) {
            PrimerStatus("Loading sign-in…", tone = PrimerStatusTone.Neutral)
        }
        return
    }
    if (!state.householdOk) {
        val setup = when {
            !clerkConfigured -> "Clerk publishable key is not set on this build. Set PRIMER_CLERK_PUBLISHABLE_KEY. Secrets are never bundled. Live Clerk acceptance is not claimed."
            !originConfigured -> "Configure PRIMER_API_ORIGIN as an HTTPS Tasks origin, or emulator http://10.0.2.2."
            else -> state.message
        }
        ControlSignInScreen(
            configured = clerkConfigured && originConfigured,
            denied = state.signedIn && state.message != null,
            message = setup,
            email = state.email,
            password = state.password,
            onEmail = { value -> model.update { it.copy(email = value) } },
            onPassword = { value -> model.update { it.copy(password = value) } },
            onSignIn = model::signIn,
            onSignOut = model::signOut,
            signedIn = state.signedIn,
        )
        return
    }
    val nav = rememberNavController()
    val backStack by nav.currentBackStackEntryAsState()
    val current = backStack?.destination?.route
    Scaffold(
        bottomBar = {
            NavigationBar {
                listOf("students" to "Students", "tasks" to "Tasks", "schedules" to "Schedules", "review" to "Review", "devices" to "Devices").forEach { (route, label) ->
                    NavigationBarItem(
                        selected = current == route,
                        onClick = {
                            when (route) {
                                "tasks" -> model.loadTasks()
                                "schedules" -> model.loadSchedules()
                                "review" -> model.loadOccurrences()
                                "devices" -> model.loadDevices()
                            }
                            nav.navigate(route)
                        },
                        icon = { Text(label.take(1)) },
                        label = { Text(label) },
                    )
                }
            }
        },
    ) { padding ->
        NavHost(nav, startDestination = "students", modifier = Modifier.padding(padding)) {
            composable("students") {
                val student = state.selectedStudent
                if (student == null && !state.creatingStudent) {
                    RosterScreen(
                        students = state.students,
                        query = state.studentQuery,
                        onQuery = model::setStudentQuery,
                        message = state.message,
                        onOpen = model::openStudent,
                        onCreate = { model.update { it.copy(selectedStudent = null, studentName = "", pairing = null, creatingStudent = true) } },
                        onRetry = { model.loadStudents() },
                        hasMore = state.studentsHasMore,
                        onMore = { model.loadStudents(reset = false) },
                    )
                } else {
                    StudentDetailScreen(
                        student = student,
                        pairing = state.pairing,
                        name = state.studentName,
                        onName = { value -> model.update { it.copy(studentName = value) } },
                        message = state.message,
                        onSave = model::saveStudent,
                        onArchive = model::archiveStudent,
                        onIssueQr = model::issuePairing,
                        onBack = { model.update { it.copy(selectedStudent = null, pairing = null, creatingStudent = false, studentName = "") } },
                    )
                }
            }
            composable("tasks") {
                if (state.editingTask != null || state.taskTitle.isNotEmpty()) {
                    TaskEditorScreen(
                        title = state.taskTitle,
                        instructions = state.taskInstructions,
                        editing = state.editingTask != null,
                        onTitle = { value -> model.update { it.copy(taskTitle = value) } },
                        onInstructions = { value -> model.update { it.copy(taskInstructions = value) } },
                        onSave = model::saveTask,
                        onClose = { model.update { it.copy(editingTask = null, taskTitle = "") } },
                        message = state.message,
                    )
                } else {
                    TasksScreen(
                        tasks = state.tasks,
                        query = state.taskQuery,
                        onQuery = { value -> model.update { it.copy(taskQuery = value) }; model.loadTasks() },
                        message = state.message,
                        onCreate = { model.update { it.copy(editingTask = null, taskTitle = " ") } },
                        onPublish = model::publish,
                        onArchive = model::retire,
                        onEdit = { task -> model.update { it.copy(editingTask = task, taskTitle = task.title, taskInstructions = task.instructions) } },
                        hasMore = state.tasksHasMore,
                        onMore = { model.loadTasks(reset = false) },
                    )
                }
            }
            composable("schedules") {
                if (state.creatingSchedule || state.editingSchedule != null || state.scheduleStudentId.isNotBlank() || state.scheduleTaskId.isNotBlank()) {
                    ScheduleEditorScreen(
                        draft = state.scheduleDraft,
                        students = state.students,
                        tasks = state.tasks,
                        studentId = state.scheduleStudentId,
                        taskId = state.scheduleTaskId,
                        editing = state.editingSchedule != null,
                        onStudent = { value -> model.update { it.copy(scheduleStudentId = value) } },
                        onTask = { value -> model.update { it.copy(scheduleTaskId = value) } },
                        onKind = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(kind = value)) } },
                        onDate = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(date = value)) } },
                        onTime = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(time = value)) } },
                        onTimezone = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(timezone = value)) } },
                        onSave = model::saveSchedule,
                        onClose = { model.update { it.copy(editingSchedule = null, scheduleStudentId = "", scheduleTaskId = "", creatingSchedule = false) } },
                        message = state.message,
                    )
                } else {
                    SchedulesScreen(
                        schedules = state.schedules,
                        message = state.message,
                        onCreate = { model.update { it.copy(creatingSchedule = true, editingSchedule = null, scheduleStudentId = "", scheduleTaskId = "") } },
                        onEdit = { schedule -> model.update { it.copy(editingSchedule = schedule, scheduleStudentId = schedule.studentId, scheduleTaskId = schedule.revisionId, creatingSchedule = false) } },
                        onCancel = model::cancelSchedule,
                        hasMore = state.schedulesHasMore,
                        onMore = { model.loadSchedules(reset = false) },
                    )
                }
            }
            composable("review") {
                val occurrence = state.selectedOccurrence
                if (occurrence == null) {
                    ReviewScreen(
                        occurrences = state.occurrences,
                        message = state.message,
                        onApprove = { item -> model.update { it.copy(selectedOccurrence = item) }; model.decide(true) },
                        onReject = { item -> model.update { it.copy(selectedOccurrence = item) }; model.decide(false) },
                        onRetry = { item -> model.update { it.copy(selectedOccurrence = item) }; model.retryOccurrence() },
                        onOpen = { item -> model.update { it.copy(selectedOccurrence = item) } },
                        hasMore = state.occurrencesHasMore,
                        onMore = { model.loadOccurrences(reset = false) },
                    )
                } else {
                    OccurrenceDetailScreen(
                        occurrence = occurrence,
                        reason = state.decisionReason,
                        onReason = { value -> model.update { it.copy(decisionReason = value) } },
                        message = state.message,
                        onApprove = { model.decide(true) },
                        onReject = { model.decide(false) },
                        onRetry = model::retryOccurrence,
                        onSkip = model::skipOccurrence,
                        onCancel = model::cancelOccurrence,
                        onBack = { model.update { it.copy(selectedOccurrence = null) } },
                    )
                }
            }
            composable("devices") {
                val device = state.selectedDevice
                if (device == null) {
                    DevicesScreen(
                        devices = state.devices,
                        enrollment = state.enrollment,
                        message = state.message,
                        onIssue = model::issueEnrollment,
                        onOpen = { item -> model.update { it.copy(selectedDevice = item) } },
                    )
                } else {
                    DeviceDetailScreen(
                        device = device,
                        releases = state.releases,
                        selectedRelease = state.selectedReleaseId,
                        onRelease = { value -> model.update { it.copy(selectedReleaseId = value) } },
                        message = state.message,
                        recoveryCodes = state.recoveryCodes,
                        acknowledged = state.recoveryAcknowledged,
                        onAcknowledge = { value -> model.update { it.copy(recoveryAcknowledged = value) } },
                        onPrepareRecovery = model::prepareRecovery,
                        onRotateRecovery = model::rotateRecovery,
                        onQuarantine = model::quarantineDevice,
                        onRevoke = model::revokeDevice,
                        onTarget = model::targetRelease,
                        onBack = { model.update { it.copy(selectedDevice = null, recoveryCodes = emptyList(), recoveryAcknowledged = false) } },
                    )
                }
            }
        }
        Column(Modifier.padding(8.dp)) {
            PrimerButton(text = "Sign out", onClick = model::signOut, variant = PrimerButtonVariant.Quiet)
        }
    }
}
