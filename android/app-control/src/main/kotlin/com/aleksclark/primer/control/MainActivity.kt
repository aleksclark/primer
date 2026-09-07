package com.aleksclark.primer.control

import android.os.Bundle
import android.view.WindowManager
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
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
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
import com.aleksclark.primer.control.tasks.ScheduleDraft
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
    override fun onResume() {
        super.onResume()
        (application as ControlApp).resumedActivity = this
    }

    override fun onPause() {
        val app = application as ControlApp
        if (app.resumedActivity === this) app.resumedActivity = null
        super.onPause()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val app = application as ControlApp
        val apiBase = ControlOriginPolicy.apiBase(BuildConfig.CONFIGURED_API_ORIGIN, BuildConfig.DEBUG)
        window.setFlags(WindowManager.LayoutParams.FLAG_SECURE, WindowManager.LayoutParams.FLAG_SECURE)
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)
        setContent {
            PrimerTheme {
                val model: ControlViewModel = viewModel(
                    factory = object : ViewModelProvider.Factory {
                        @Suppress("UNCHECKED_CAST")
                        override fun <T : ViewModel> create(modelClass: Class<T>): T {
                            return ControlViewModel(
                                identity = app.identity,
                                apiBase = apiBase,
                                updater = app.updater,
                                downloadDir = cacheDir,
                                discoveryStore = PrefsControlDiscoveryStore(app),
                            ) as T
                        }
                    },
                )
                ControlAppScreen(
                    model = model,
                    apiBase = apiBase,
                    clerkConfigured = BuildConfig.CLERK_PUBLISHABLE_KEY.isNotBlank(),
                    originConfigured = apiBase != null,
                    startSettings = { intent -> startActivity(intent) },
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
    startSettings: (android.content.Intent) -> Unit = {},
) {
    val state by model.state.collectAsStateWithLifecycle()
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner, model) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) model.onResume()
            if (event == Lifecycle.Event.ON_STOP) model.clearPassword()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }
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
            secondFactorRequired = state.secondFactorRequired,
            secondFactorCode = state.secondFactorCode,
            onSecondFactorCode = { value -> model.update { it.copy(secondFactorCode = value) } },
            onContinueSecondFactor = model::continueSecondFactor,
            secondFactorStrategies = state.secondFactorStrategies,
            selectedSecondFactor = state.selectedSecondFactor,
            onSelectSecondFactor = model::selectSecondFactor,
            onCancelSecondFactor = model::cancelSecondFactor,
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
                if (state.creatingTask || state.editingTask != null) {
                    TaskEditorScreen(
                        title = state.taskTitle,
                        instructions = state.taskInstructions,
                        editing = state.editingTask != null,
                        creating = state.creatingTask,
                        onTitle = { value -> model.update { it.copy(taskTitle = value) } },
                        onInstructions = { value -> model.update { it.copy(taskInstructions = value) } },
                        onSave = model::saveTask,
                        onClose = model::closeTaskEditor,
                        message = state.message,
                    )
                } else {
                    TasksScreen(
                        tasks = state.tasks,
                        query = state.taskQuery,
                        onQuery = { value -> model.update { it.copy(taskQuery = value) }; model.loadTasks() },
                        message = state.message,
                        onCreate = model::beginCreateTask,
                        onPublish = model::publish,
                        onArchive = model::retire,
                        onEdit = model::beginEditTask,
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
                        studentQuery = state.studentQuery,
                        taskQuery = state.taskQuery,
                        editing = state.editingSchedule != null,
                        onStudent = { value -> model.update { it.copy(scheduleStudentId = value) } },
                        onTask = { value -> model.update { it.copy(scheduleTaskId = value) } },
                        onStudentQuery = model::setStudentQuery,
                        onTaskQuery = model::setScheduleTaskQuery,
                        onMoreStudents = { model.loadStudents(reset = false) },
                        onMoreTasks = { model.loadPublishedTasks(reset = false) },
                        studentsHasMore = state.studentsHasMore,
                        tasksHasMore = state.tasksHasMore,
                        onKind = { value ->
                            model.update {
                                it.copy(
                                    scheduleDraft = it.scheduleDraft.copy(
                                        kind = value,
                                        rrule = if (value == "one_off") "" else it.scheduleDraft.rrule,
                                    ),
                                )
                            }
                        },
                        onDate = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(date = value, originalStartAt = null)) } },
                        onTime = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(time = value, originalStartAt = null)) } },
                        onTimezone = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(timezone = value, originalStartAt = null)) } },
                        onRrule = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(rrule = value)) } },
                        onDueOffset = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(dueOffsetText = value)) } },
                        onEndAt = { value -> model.update { it.copy(scheduleDraft = it.scheduleDraft.copy(endAt = value)) } },
                        onSave = model::saveSchedule,
                        onClose = { model.update { it.copy(editingSchedule = null, scheduleStudentId = "", scheduleTaskId = "", creatingSchedule = false, scheduleDraft = ScheduleDraft()) } },
                        message = state.message,
                    )
                } else {
                    SchedulesScreen(
                        schedules = state.schedules,
                        message = state.message,
                        onCreate = model::beginCreateSchedule,
                        onEdit = model::beginEditSchedule,
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
                        onOpen = { item -> model.update { it.copy(selectedOccurrence = item, decisionReason = "") } },
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
                        selfUpdate = state.selfUpdate,
                        onIssue = model::issueEnrollment,
                        onOpen = model::openDevice,
                        onInstallUpdate = model::installControlUpdate,
                        onContinueUpdate = model::continueControlUpdate,
                        onDiscoveryCheckOnResume = { model.setDiscovery(checkOnResume = it) },
                        onDiscoveryPeriodic = { model.setDiscovery(periodicEnabled = it) },
                        onDiscoveryUnattended = { model.setDiscovery(unattendedCatchUp = it) },
                        onOpenInstallSettings = {
                            model.openInstallSettings()?.let(startSettings)
                        },
                    )
                } else {
                    DeviceDetailScreen(
                        device = device,
                        desired = state.desired,
                        syncStatus = state.syncStatus,
                        releases = state.releases,
                        selectedRelease = state.selectedReleaseId,
                        onRelease = { value -> model.update { it.copy(selectedReleaseId = value) } },
                        message = state.message,
                        recovery = state.recovery,
                        approvedAppDraft = state.approvedAppDraft,
                        onApprovedPackage = { value -> model.update { it.copy(approvedAppDraft = it.approvedAppDraft.copy(packageName = value)) } },
                        onApprovedSigner = { value -> model.update { it.copy(approvedAppDraft = it.approvedAppDraft.copy(signerSha256 = value)) } },
                        onApprovedLabel = { value -> model.update { it.copy(approvedAppDraft = it.approvedAppDraft.copy(label = value)) } },
                        onApprovedRequired = { value -> model.update { it.copy(approvedAppDraft = it.approvedAppDraft.copy(required = value)) } },
                        onAddApprovedApp = model::addApprovedApp,
                        onRemoveApprovedApp = model::removeApprovedApp,
                        onParentUnlock = model::setParentUnlock,
                        onAcknowledge = model::acknowledgeRecovery,
                        onPrepareRecovery = model::prepareRecovery,
                        onRotateRecovery = model::rotateRecovery,
                        onQuarantine = model::quarantineDevice,
                        onRevoke = model::revokeDevice,
                        onTarget = { model.targetRelease(false) },
                        onRequeue = { model.targetRelease(true) },
                        onBack = model::closeDevice,
                        mutating = state.mutating,
                    )
                }
            }
        }
        Column(Modifier.padding(8.dp)) {
            PrimerButton(text = "Sign out", onClick = model::signOut, variant = PrimerButtonVariant.Quiet)
        }
    }
}
