package com.aleksclark.primer.control

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.aleksclark.primer.control.device.ApprovedAppDraft
import com.aleksclark.primer.control.device.ControlSelfUpdate
import com.aleksclark.primer.control.device.ControlSelfUpdatePhase
import com.aleksclark.primer.control.device.ControlSelfUpdateUi
import com.aleksclark.primer.control.device.ControlUpdateDiscovery
import com.aleksclark.primer.control.device.ControlUpdateDiscoverySettings
import com.aleksclark.primer.control.device.DeviceRepository
import com.aleksclark.primer.control.device.DeviceSync
import com.aleksclark.primer.control.device.DeviceSyncStatus
import com.aleksclark.primer.control.device.RecoveryBinding
import com.aleksclark.primer.control.tasks.AuthContext
import com.aleksclark.primer.control.tasks.AuthEpoch
import com.aleksclark.primer.control.tasks.ConflictRefresh
import com.aleksclark.primer.control.tasks.ConflictResource
import com.aleksclark.primer.control.tasks.FailClosedLogout
import com.aleksclark.primer.control.tasks.LogoutAttempt
import com.aleksclark.primer.control.tasks.LogoutDecision
import com.aleksclark.primer.control.tasks.MutationGate
import com.aleksclark.primer.control.tasks.ParentTasksRepository
import com.aleksclark.primer.control.tasks.ScheduleDraft
import com.aleksclark.primer.control.tasks.scheduleDraftFrom
import com.aleksclark.primer.control.tasks.ScheduleIdentity
import com.aleksclark.primer.control.tasks.controlMessage
import com.aleksclark.primer.control.tasks.toInput
import com.aleksclark.primer.identity.ParentIdentity
import com.aleksclark.primer.identity.SignInOutcome
import com.aleksclark.primer.identity.SignOutOutcome
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.DesiredState
import com.aleksclark.primertasks.client.Enrollment
import com.aleksclark.primertasks.client.ManagedDevice
import com.aleksclark.primertasks.client.Occurrence
import com.aleksclark.primertasks.client.Pairing
import com.aleksclark.primertasks.client.RecoveryIntent
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.Schedule
import com.aleksclark.primertasks.client.Student
import com.aleksclark.primertasks.client.TaskRevision
import com.aleksclark.primertasks.client.TasksHttpException
import java.util.concurrent.atomic.AtomicLong
import kotlin.coroutines.cancellation.CancellationException
import com.aleksclark.primer.updates.ArchiveChecks
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient

data class ControlUiState(
    val ready: Boolean = false,
    val signedIn: Boolean = false,
    val householdOk: Boolean = false,
    val message: String? = null,
    val logoutIncomplete: Boolean = false,
    val mutating: Boolean = false,
    val students: List<Student> = emptyList(),
    val studentQuery: String = "",
    val selectedStudent: Student? = null,
    val studentName: String = "",
    val pairing: Pairing? = null,
    val tasks: List<TaskRevision> = emptyList(),
    val taskQuery: String = "",
    val taskTitle: String = "",
    val taskInstructions: String = "Complete the task, then ask a parent to check it.",
    val editingTask: TaskRevision? = null,
    val schedules: List<Schedule> = emptyList(),
    val scheduleDraft: ScheduleDraft = ScheduleDraft(),
    val scheduleStudentId: String = "",
    val scheduleTaskId: String = "",
    val editingSchedule: Schedule? = null,
    val occurrences: List<Occurrence> = emptyList(),
    val selectedOccurrence: Occurrence? = null,
    val decisionReason: String = "",
    val devices: List<ManagedDevice> = emptyList(),
    val selectedDevice: ManagedDevice? = null,
    val desired: DesiredState? = null,
    val syncStatus: DeviceSyncStatus? = null,
    val enrollment: Enrollment? = null,
    val releases: List<Release> = emptyList(),
    val selectedReleaseId: String = "",
    val email: String = "",
    val password: String = "",
    val secondFactorRequired: Boolean = false,
    val secondFactorCode: String = "",
    val secondFactorStrategies: List<String> = emptyList(),
    val selectedSecondFactor: String = "",
    val clerkAuthorizedParty: String? = null,
    val clerkIssuer: String? = null,
    val recovery: RecoveryBinding? = null,
    val recoveryHistory: List<RecoveryIntent> = emptyList(),
    val approvedAppDraft: ApprovedAppDraft = ApprovedAppDraft(),
    val creatingStudent: Boolean = false,
    val creatingTask: Boolean = false,
    val creatingSchedule: Boolean = false,
    val studentsHasMore: Boolean = false,
    val tasksHasMore: Boolean = false,
    val schedulesHasMore: Boolean = false,
    val occurrencesHasMore: Boolean = false,
    val selfUpdate: ControlSelfUpdateUi = ControlSelfUpdateUi(),
    val discovery: ControlUpdateDiscoverySettings = ControlUpdateDiscoverySettings(),
)

class ControlViewModel(
    private val identity: ParentIdentity,
    private val apiBase: String?,
    private val http: OkHttpClient = OkHttpClient(),
    private val updater: ControlSelfUpdateCoordinator? = null,
    private val downloadDir: java.io.File? = null,
    private val io: CoroutineDispatcher = Dispatchers.IO,
    private val clock: () -> Long = { System.currentTimeMillis() },
    private val discoveryStore: ControlDiscoveryStore? = null,
    private val catalogPeriodMs: Long = ControlUpdateDiscovery.MIN_PERIOD_MS,
    private val tasksFactory: (CredentialProvider) -> ParentTasksRepository = { token ->
        ParentTasksRepository(requireNotNull(apiBase), token, http)
    },
    private val devicesFactory: (CredentialProvider) -> DeviceRepository = { token ->
        DeviceRepository(requireNotNull(apiBase), token, http)
    },
) : ViewModel() {
    private val epoch = AuthEpoch()
    private val mutations = MutationGate()
    private val authGate = MutationGate()
    private val studentQueryGen = AtomicLong(0)
    private val taskQueryGen = AtomicLong(0)
    private var session: AuthContext? = null
    private var logoutAttempt: LogoutAttempt? = null
    private var workJob = SupervisorJob(viewModelScope.coroutineContext.job)
    private val workScope: CoroutineScope
        get() = CoroutineScope(viewModelScope.coroutineContext + workJob)
    private var catalogJob: Job? = null

    private val _state = MutableStateFlow(ControlUiState())
    val state: StateFlow<ControlUiState> = _state

    init {
        discoveryStore?.load()?.let { loaded -> _state.value = _state.value.copy(discovery = loaded) }
        syncCatalogTicker()
        viewModelScope.launch { refreshAuth() }
    }

    fun onResume() {
        viewModelScope.launch {
            try {
                updater?.continueConfirmation()
                refreshAuth()
                val ctx = session
                if (ctx != null && stillValid(ctx)) {
                    refreshSelfUpdate(ctx)
                    applyDiscovery(ctx)
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _state.value = _state.value.copy(message = controlMessage(error))
            }
        }
    }

    fun setDiscovery(checkOnResume: Boolean? = null, periodicEnabled: Boolean? = null, unattendedCatchUp: Boolean? = null) {
        val current = _state.value.discovery
        val next = current.copy(
            checkOnResume = checkOnResume ?: current.checkOnResume,
            periodicEnabled = periodicEnabled ?: current.periodicEnabled,
            unattendedCatchUp = unattendedCatchUp ?: current.unattendedCatchUp,
        )
        _state.value = _state.value.copy(
            discovery = next,
            selfUpdate = _state.value.selfUpdate.copy(discovery = next),
        )
        discoveryStore?.save(next)
        syncCatalogTicker()
    }

    fun clearPassword() {
        if (_state.value.password.isNotEmpty()) _state.value = _state.value.copy(password = "")
    }

    fun signIn() {
        if (mutations.isBusy() || !authGate.tryBegin()) {
            _state.value = _state.value.copy(message = "Wait for the current change to finish.")
            return
        }
        val email = _state.value.email.trim()
        val password = _state.value.password
        _state.value = _state.value.copy(password = "", message = null)
        viewModelScope.launch {
            try {
                fence()
                applySignInOutcome(identity.signIn(email, password))
            } finally {
                authGate.end()
            }
        }
    }

    fun signInWithGoogle() {
        if (mutations.isBusy() || !authGate.tryBegin()) {
            _state.value = _state.value.copy(message = "Wait for the current change to finish.")
            return
        }
        _state.value = _state.value.copy(password = "", message = null)
        viewModelScope.launch {
            try {
                fence()
                applySignInOutcome(identity.signInWithGoogle())
            } finally {
                authGate.end()
            }
        }
    }

    fun continueSecondFactor() {
        if (mutations.isBusy() || !authGate.tryBegin()) {
            _state.value = _state.value.copy(message = "Wait for the current change to finish.")
            return
        }
        val code = _state.value.secondFactorCode
        val strategy = _state.value.selectedSecondFactor
        _state.value = _state.value.copy(secondFactorCode = "", message = null)
        viewModelScope.launch {
            try {
                fence()
                applySignInOutcome(identity.continueSecondFactor(code, strategy), keepSecondFactorOnFailure = true)
            } finally {
                authGate.end()
            }
        }
    }

    fun selectSecondFactor(strategy: String) {
        if (mutations.isBusy() || !authGate.tryBegin()) {
            _state.value = _state.value.copy(message = "Wait for the current change to finish.")
            return
        }
        _state.value = _state.value.copy(selectedSecondFactor = strategy, secondFactorCode = "", message = null)
        viewModelScope.launch {
            try {
                applySignInOutcome(identity.prepareSecondFactor(strategy), keepSecondFactorOnFailure = true)
            } finally {
                authGate.end()
            }
        }
    }

    fun cancelSecondFactor() {
        viewModelScope.launch {
            identity.cancelIncompleteSignIn()
            _state.value = _state.value.copy(
                secondFactorRequired = false,
                secondFactorCode = "",
                secondFactorStrategies = emptyList(),
                selectedSecondFactor = "",
                message = null,
            )
        }
    }

    fun signOut() {
        if (!authGate.tryBegin()) {
            _state.value = _state.value.copy(message = "Wait for the current change to finish.")
            return
        }
        viewModelScope.launch {
            try {
                identity.cancelIncompleteSignIn()
                signOutLocked()
            } finally {
                authGate.end()
            }
        }
    }

    private suspend fun applySignInOutcome(outcome: SignInOutcome, keepSecondFactorOnFailure: Boolean = false) {
        when (outcome) {
            is SignInOutcome.SignedIn -> completeSignedIn(outcome.sessionId)
            is SignInOutcome.NeedsSecondFactor -> _state.value = _state.value.copy(
                ready = true,
                signedIn = false,
                householdOk = false,
                secondFactorRequired = true,
                secondFactorCode = "",
                secondFactorStrategies = outcome.strategies,
                selectedSecondFactor = outcome.selectedStrategy,
                message = outcome.message,
            )
            is SignInOutcome.Incomplete -> _state.value = _state.value.copy(
                ready = true,
                signedIn = false,
                householdOk = false,
                secondFactorRequired = false,
                secondFactorCode = "",
                message = outcome.message,
            )
            is SignInOutcome.Failed -> _state.value = _state.value.copy(
                ready = true,
                signedIn = false,
                householdOk = false,
                secondFactorRequired = keepSecondFactorOnFailure && _state.value.secondFactorRequired,
                message = outcome.message,
            )
        }
    }

    private suspend fun completeSignedIn(sessionId: String) {
        val attemptEpoch = epoch.current()
        val token = identity.sessionToken(skipCache = true)
        val liveSid = identity.sessionId()
        if (!epoch.isCurrent(attemptEpoch)) return
        if (token.isNullOrBlank() || liveSid != sessionId) {
            _state.value = signedOut(message = "Clerk did not activate the newly created session.")
            return
        }
        session = AuthContext(sessionId = liveSid, epoch = attemptEpoch, token = token)
        logoutAttempt = null
        refreshAuth()
    }

    private suspend fun signOutLocked() {
        fence()
        val current = session
        val previous = logoutAttempt?.takeIf { it.sessionId == current?.sessionId || current == null }
        var attempt = previous
            ?: current?.let { LogoutAttempt(sessionId = it.sessionId, token = it.token, serverRevoked = false) }
        if (attempt == null) {
            val liveSid = identity.sessionId()
            if (liveSid.isNullOrBlank()) {
                session = null
                logoutAttempt = null
                _state.value = signedOut()
                return
            }
            val liveToken = identity.sessionToken(skipCache = true)
            val sidAfter = identity.sessionId()
            if (sidAfter != liveSid || liveToken.isNullOrBlank()) {
                _state.value = _state.value.copy(
                    ready = true,
                    signedIn = true,
                    message = "Sign in again. The parent session is missing or expired.",
                    logoutIncomplete = true,
                )
                return
            }
            attempt = LogoutAttempt(sessionId = liveSid, token = liveToken, serverRevoked = false)
        }
        logoutAttempt = attempt
        val liveSid = identity.sessionId()
        if (liveSid != null && liveSid != attempt.sessionId) {
            _state.value = _state.value.copy(message = "The signed-in account changed. Sign out again.", logoutIncomplete = true)
            return
        }
        if (previous != null && !attempt.serverRevoked && liveSid == attempt.sessionId) {
            val refreshed = try {
                identity.sessionToken(skipCache = true)
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                null
            }
            val sidAfter = identity.sessionId()
            if (sidAfter == attempt.sessionId && !refreshed.isNullOrBlank()) {
                attempt = attempt.copy(token = refreshed)
                logoutAttempt = attempt
            }
        }
        var serverRevoked = attempt.serverRevoked
        var serverError: String? = null
        if (!serverRevoked) {
            if (apiBase == null) {
                serverError = "Tasks origin is not configured."
            } else {
                try {
                    tasksFor(AuthContext(attempt.sessionId, epoch.current(), attempt.token)).logout()
                    serverRevoked = true
                } catch (error: CancellationException) {
                    throw error
                } catch (error: Exception) {
                    serverError = controlMessage(error)
                }
            }
        }
        logoutAttempt = attempt.copy(serverRevoked = serverRevoked)
        val provider = if (serverRevoked) {
            when (val out = identity.signOutProvider()) {
                SignOutOutcome.SignedOut -> true to null
                is SignOutOutcome.Failed -> false to out.message
            }
        } else {
            false to null
        }
        when (val decision = FailClosedLogout.decide(serverRevoked, provider.first, serverError, provider.second)) {
            LogoutDecision.ClearSession -> {
                session = null
                logoutAttempt = null
                _state.value = signedOut()
            }
            is LogoutDecision.Incomplete -> {
                if (decision.serverRevoked) {
                    session = null
                } else {
                    session = AuthContext(attempt.sessionId, epoch.current(), attempt.token)
                }
                _state.value = _state.value.copy(
                    ready = true,
                    signedIn = true,
                    householdOk = !decision.serverRevoked,
                    message = decision.message,
                    logoutIncomplete = true,
                )
            }
        }
    }

    fun setStudentQuery(value: String) {
        _state.value = _state.value.copy(studentQuery = value)
        loadStudents()
    }

    fun loadStudents(reset: Boolean = true) = act { ctx -> refreshStudents(ctx, reset) }

    fun openStudent(student: Student) = act { ctx ->
        val found = tasksFor(ctx).getStudent(student.id)
        commit(ctx) { it.copy(selectedStudent = found, studentName = found.displayName, pairing = null, creatingStudent = false) }
    }

    fun saveStudent() {
        val student = _state.value.selectedStudent
        val name = _state.value.studentName.trim()
        mutate {
            if (student == null) tasksFor(it).createStudent(name) else tasksFor(it).updateStudent(student.id, name)
            refreshStudents(it)
            commit(it) { state -> state.copy(selectedStudent = null, studentName = "", creatingStudent = false) }
        }
    }

    fun archiveStudent() {
        val id = _state.value.selectedStudent?.id ?: return
        mutate(ConflictResource.Student) { ctx ->
            tasksFor(ctx).archiveStudent(id)
            refreshStudents(ctx)
            commit(ctx) { it.copy(selectedStudent = null, creatingStudent = false) }
        }
    }

    fun issuePairing() {
        val id = _state.value.selectedStudent?.id ?: return
        mutate { ctx ->
            val pairing = tasksFor(ctx).issuePairing(id)
            commit(ctx) { it.copy(pairing = pairing) }
        }
    }

    fun loadTasks(reset: Boolean = true) = act { ctx -> refreshTasks(ctx, reset) }

    fun saveTask() {
        val editing = _state.value.editingTask
        val title = _state.value.taskTitle.trim()
        val instructions = _state.value.taskInstructions
        mutate(ConflictResource.Task) { ctx ->
            if (editing == null) tasksFor(ctx).createTask(title, instructions)
            else tasksFor(ctx).reviseTask(editing.templateId, title, instructions, editing.requirements)
            refreshTasks(ctx)
            commit(ctx) { it.copy(editingTask = null, creatingTask = false, taskTitle = "", taskInstructions = "Complete the task, then ask a parent to check it.") }
        }
    }

    fun publish(task: TaskRevision) = mutate(ConflictResource.Task) { ctx -> tasksFor(ctx).publishTask(task.id); refreshTasks(ctx) }
    fun retire(task: TaskRevision) = mutate(ConflictResource.Task) { ctx -> tasksFor(ctx).retireTask(task.templateId); refreshTasks(ctx) }

    fun loadSchedules(reset: Boolean = true) = act { ctx -> refreshSchedules(ctx, reset) }

    fun saveSchedule() {
        val snapshot = _state.value
        val revision = ScheduleIdentity.fromPublishedTask(
            templateId = snapshot.tasks.firstOrNull { it.id == snapshot.scheduleTaskId }?.templateId ?: snapshot.editingSchedule?.templateId,
            revisionId = snapshot.tasks.firstOrNull { it.id == snapshot.scheduleTaskId }?.id ?: snapshot.editingSchedule?.revisionId,
        ) ?: run {
            _state.value = snapshot.copy(message = "Choose a published task revision from the server list.")
            return
        }
        val studentId = snapshot.scheduleStudentId.ifBlank { snapshot.editingSchedule?.studentId.orEmpty() }
        if (studentId.isBlank()) {
            _state.value = snapshot.copy(message = "Choose a student.")
            return
        }
        val body = try {
            snapshot.scheduleDraft.toInput(studentId, revision.templateId, revision.revisionId)
        } catch (error: Exception) {
            _state.value = snapshot.copy(message = controlMessage(error))
            return
        }
        val editingId = snapshot.editingSchedule?.id
        mutate(ConflictResource.Schedule) { ctx ->
            if (editingId == null) tasksFor(ctx).createSchedule(body) else tasksFor(ctx).updateSchedule(editingId, body)
            refreshSchedules(ctx)
            commit(ctx) { it.copy(editingSchedule = null, scheduleStudentId = "", scheduleTaskId = "", creatingSchedule = false, scheduleDraft = ScheduleDraft()) }
        }
    }

    fun cancelSchedule(schedule: Schedule) = mutate(ConflictResource.Schedule) { ctx ->
        tasksFor(ctx).retireSchedule(schedule.id)
        refreshSchedules(ctx)
    }

    fun loadOccurrences(reset: Boolean = true) = act { ctx -> refreshOccurrences(ctx, reset) }

    fun beginCreateTask() {
        _state.value = _state.value.copy(
            creatingTask = true,
            editingTask = null,
            taskTitle = "",
            taskInstructions = "Complete the task, then ask a parent to check it.",
        )
    }

    fun beginEditTask(task: TaskRevision) {
        _state.value = _state.value.copy(
            creatingTask = false,
            editingTask = task,
            taskTitle = task.title,
            taskInstructions = task.instructions,
        )
    }

    fun closeTaskEditor() {
        _state.value = _state.value.copy(creatingTask = false, editingTask = null, taskTitle = "", taskInstructions = "Complete the task, then ask a parent to check it.")
    }

    fun beginCreateSchedule() {
        _state.value = _state.value.copy(
            creatingSchedule = true,
            editingSchedule = null,
            scheduleStudentId = "",
            scheduleTaskId = "",
            scheduleDraft = ScheduleDraft(),
            taskQuery = "",
        )
        loadStudents()
        loadPublishedTasks()
    }

    fun beginEditSchedule(schedule: Schedule) {
        _state.value = _state.value.copy(
            creatingSchedule = false,
            editingSchedule = schedule,
            scheduleStudentId = schedule.studentId,
            scheduleTaskId = schedule.revisionId,
            scheduleDraft = scheduleDraftFrom(schedule),
        )
        loadStudents()
        loadPublishedTasks()
    }

    fun setScheduleTaskQuery(value: String) {
        _state.value = _state.value.copy(taskQuery = value)
        loadPublishedTasks()
    }

    fun loadPublishedTasks(reset: Boolean = true) = act { ctx -> refreshPublishedTasks(ctx, reset) }

    fun decide(accepted: Boolean, occurrence: Occurrence? = null, reason: String? = null) {
        val target = occurrence ?: _state.value.selectedOccurrence ?: return
        val decision = (reason ?: _state.value.decisionReason).trim()
        if (decision.isEmpty()) {
            _state.value = _state.value.copy(message = "Enter a reason before approving or rejecting this work.")
            return
        }
        mutate(ConflictResource.Occurrence) { ctx ->
            tasksFor(ctx).decide(target.id, accepted, decision)
            val latest = tasksFor(ctx).getOccurrence(target.id)
            refreshOccurrences(ctx)
            commit(ctx) { it.copy(selectedOccurrence = latest, decisionReason = "") }
        }
    }

    fun retryOccurrence(occurrence: Occurrence? = null) {
        val target = occurrence ?: _state.value.selectedOccurrence ?: return
        mutate(ConflictResource.Occurrence) { ctx ->
            tasksFor(ctx).retry(target.id)
            val latest = tasksFor(ctx).getOccurrence(target.id)
            refreshOccurrences(ctx)
            commit(ctx) { it.copy(selectedOccurrence = latest) }
        }
    }

    fun skipOccurrence() {
        val id = _state.value.selectedOccurrence?.id ?: return
        mutate(ConflictResource.Occurrence) { ctx ->
            tasksFor(ctx).skip(id)
            val occurrence = tasksFor(ctx).getOccurrence(id)
            refreshOccurrences(ctx)
            commit(ctx) { it.copy(selectedOccurrence = occurrence) }
        }
    }

    fun cancelOccurrence() {
        val id = _state.value.selectedOccurrence?.id ?: return
        mutate(ConflictResource.Occurrence) { ctx ->
            tasksFor(ctx).cancel(id)
            val occurrence = tasksFor(ctx).getOccurrence(id)
            refreshOccurrences(ctx)
            commit(ctx) { it.copy(selectedOccurrence = occurrence) }
        }
    }

    fun loadDevices() = act { ctx ->
        refreshDevices(ctx)
        refreshSelfUpdate(ctx)
        applyDiscovery(ctx)
    }

    fun installControlUpdate() {
        val snapshot = _state.value
        if (blocksInstall(snapshot.selfUpdate.phase, snapshot.selfUpdate.canContinueConfirmation)) {
            _state.value = snapshot.copy(message = "Finish or cancel the current install confirmation first.")
            return
        }
        val candidate = ControlSelfUpdate.selectCandidate(snapshot.releases, snapshot.selfUpdate.installedVersion)
            ?: run {
                _state.value = snapshot.copy(message = "No newer Control release is published on the stable channel.")
                return
            }
        mutate { ctx ->
            if (!_state.value.selfUpdate.canInstall) {
                prepareCandidate(ctx, candidate)
                if (!stillValid(ctx)) return@mutate
                refreshSelfUpdate(ctx)
            }
            val latest = _state.value.selfUpdate
            if (blocksInstall(latest.phase, latest.canContinueConfirmation) || !latest.canInstall) return@mutate
            withContext(io) {
                if (!stillValid(ctx)) return@withContext
                updater?.installPrepared()
            }
            if (stillValid(ctx)) refreshSelfUpdate(ctx)
        }
    }

    fun continueControlUpdate() {
        act { ctx ->
            updater?.continueConfirmation()
            refreshSelfUpdate(ctx)
        }
    }

    fun cancelControlUpdate() {
        val snapshot = _state.value.selfUpdate
        if (!snapshot.canCancel) {
            _state.value = _state.value.copy(message = "There is no live Control install to cancel.")
            return
        }
        mutate { ctx ->
            withContext(io) {
                if (!stillValid(ctx)) return@withContext
                updater?.cancelInstall()
            }
            if (stillValid(ctx)) {
                refreshSelfUpdate(ctx)
                commit(ctx) { it.copy(message = "Control install cancelled. Retry is required before another attempt.") }
            }
        }
    }

    fun retryControlUpdate() {
        val snapshot = _state.value.selfUpdate
        if (snapshot.canCancel || snapshot.canContinueConfirmation) {
            _state.value = _state.value.copy(message = "Finish or cancel the current install confirmation first.")
            return
        }
        if (!snapshot.canRetry) {
            _state.value = _state.value.copy(message = "There is no failed Control update to retry.")
            return
        }
        mutate { ctx ->
            withContext(io) {
                if (!stillValid(ctx)) return@withContext
                updater?.retryFailed()
            }
            if (!stillValid(ctx)) return@mutate
            refreshSelfUpdate(ctx)
            commit(ctx) { it.copy(message = "Failed Control install cleared. Prepare and install again to retry.") }
        }
    }

    fun openInstallSettings(): android.content.Intent? = updater?.settingsIntent()

    fun openDevice(device: ManagedDevice) = act { ctx -> reloadDevice(ctx, device.id) }

    fun issueEnrollment() = mutate { ctx ->
        val enrollment = devicesFor(ctx).issue()
        commit(ctx) { it.copy(enrollment = enrollment) }
    }

    fun targetRelease(requeue: Boolean = false) {
        val deviceId = _state.value.selectedDevice?.id ?: return
        val release = _state.value.releases.firstOrNull { it.id == _state.value.selectedReleaseId }
            ?: run {
                _state.value = _state.value.copy(message = "Choose a published release.")
                return
            }
        mutate(ConflictResource.Device) { ctx ->
            val desired = devicesFor(ctx).desired(deviceId)
            devicesFor(ctx).target(deviceId, release, desired, requeue)
            reloadDevice(ctx, deviceId)
        }
    }

    fun quarantineDevice() {
        val deviceId = _state.value.selectedDevice?.id ?: return
        mutate(ConflictResource.Device) { ctx ->
            devicesFor(ctx).quarantine(deviceId, "parent quarantine")
            refreshDevices(ctx)
            reloadDevice(ctx, deviceId)
        }
    }

    fun revokeDevice() {
        val deviceId = _state.value.selectedDevice?.id ?: return
        mutate(ConflictResource.Device) { ctx ->
            devicesFor(ctx).revoke(deviceId, "parent revoke")
            refreshDevices(ctx)
            val latest = devicesFor(ctx).get(deviceId)
            commit(ctx) { it.copy(selectedDevice = latest, recovery = null) }
        }
    }

    fun addApprovedApp() {
        val deviceId = _state.value.selectedDevice?.id ?: return
        val draft = _state.value.approvedAppDraft
        mutate(ConflictResource.Device) { ctx ->
            val desired = devicesFor(ctx).desired(deviceId)
            devicesFor(ctx).addApprovedApp(deviceId, desired, draft)
            commit(ctx) { it.copy(approvedAppDraft = ApprovedAppDraft()) }
            reloadDevice(ctx, deviceId)
        }
    }

    fun removeApprovedApp(packageName: String) {
        val deviceId = _state.value.selectedDevice?.id ?: return
        mutate(ConflictResource.Device) { ctx ->
            devicesFor(ctx).removeApprovedApp(deviceId, devicesFor(ctx).desired(deviceId), packageName)
            reloadDevice(ctx, deviceId)
        }
    }

    fun setParentUnlock(allow: Boolean) {
        val deviceId = _state.value.selectedDevice?.id ?: return
        mutate(ConflictResource.Device) { ctx ->
            devicesFor(ctx).setParentUnlock(deviceId, devicesFor(ctx).desired(deviceId), allow)
            reloadDevice(ctx, deviceId)
        }
    }

    fun prepareRecovery() {
        val deviceId = _state.value.selectedDevice?.id ?: return
        mutate { ctx ->
            val latest = devicesFor(ctx).get(deviceId)
            val publicKey = latest.enrollmentPublicKey
            if (publicKey.isNullOrBlank()) {
                commit(ctx) { it.copy(selectedDevice = latest, message = "This device has no enrollment public key yet.") }
                return@mutate
            }
            val prepared = devicesFor(ctx).prepareRotation(latest.id, publicKey)
            commit(ctx) { it.copy(selectedDevice = latest, recovery = prepared, message = null) }
        }
    }

    fun acknowledgeRecovery(value: Boolean) {
        val recovery = _state.value.recovery ?: return
        _state.value = _state.value.copy(recovery = recovery.copy(acknowledged = value))
    }

    fun rotateRecovery() {
        val deviceId = _state.value.selectedDevice?.id ?: return
        val prepared = _state.value.recovery
        val acknowledged = prepared?.acknowledged == true
        mutate(ConflictResource.Device) { ctx ->
            val latest = devicesFor(ctx).get(deviceId)
            devicesFor(ctx).rotateRecovery(latest.id, latest.enrollmentPublicKey, prepared, acknowledged)
            commit(ctx) { it.copy(selectedDevice = latest, recovery = null) }
        }
    }

    fun closeDevice() {
        _state.value = _state.value.copy(
            selectedDevice = null,
            desired = null,
            recovery = null,
            recoveryHistory = emptyList(),
            syncStatus = null,
            approvedAppDraft = ApprovedAppDraft(),
        )
    }

    fun update(transform: (ControlUiState) -> ControlUiState) {
        _state.value = transform(_state.value)
    }

    private suspend fun refreshStudents(ctx: AuthContext, reset: Boolean = true) {
        val query = _state.value.studentQuery
        val gen = studentQueryGen.incrementAndGet()
        val offset = if (reset) 0L else _state.value.students.size.toLong()
        val page = tasksFor(ctx).listStudents(query, offset)
        if (!stillValid(ctx) || gen != studentQueryGen.get()) return
        val items = if (reset) page.items else _state.value.students + page.items
        commit(ctx) { it.copy(students = items, studentsHasMore = items.size < page.totalCount.toInt(), message = null) }
    }

    private suspend fun refreshTasks(ctx: AuthContext, reset: Boolean = true, status: String = "active") {
        val query = _state.value.taskQuery
        val gen = taskQueryGen.incrementAndGet()
        val offset = if (reset) 0L else _state.value.tasks.size.toLong()
        val page = tasksFor(ctx).listTasks(query, offset, status)
        if (!stillValid(ctx) || gen != taskQueryGen.get()) return
        val items = if (reset) page.items else _state.value.tasks + page.items
        commit(ctx) { it.copy(tasks = items, tasksHasMore = items.size < page.totalCount.toInt()) }
    }

    private suspend fun refreshPublishedTasks(ctx: AuthContext, reset: Boolean = true) {
        refreshTasks(ctx, reset, status = "published")
    }

    private suspend fun refreshSchedules(ctx: AuthContext, reset: Boolean = true) {
        val offset = if (reset) 0L else _state.value.schedules.size.toLong()
        val page = tasksFor(ctx).listSchedules(offset, "active")
        val items = if (reset) page.items else _state.value.schedules + page.items
        commit(ctx) { it.copy(schedules = items, schedulesHasMore = items.size < page.totalCount.toInt()) }
        if (reset && stillValid(ctx)) {
            refreshPublishedTasks(ctx)
            refreshStudents(ctx)
        }
    }

    private suspend fun refreshOccurrences(ctx: AuthContext, reset: Boolean = true) {
        val offset = if (reset) 0L else _state.value.occurrences.size.toLong()
        val page = tasksFor(ctx).listOccurrences("", "asc", offset)
        val items = if (reset) page.items else _state.value.occurrences + page.items
        commit(ctx) { it.copy(occurrences = items, occurrencesHasMore = items.size < page.totalCount.toInt()) }
    }

    private suspend fun refreshDevices(ctx: AuthContext) {
        val listed = devicesFor(ctx).list().items
        val published = devicesFor(ctx).releases().items
        commit(ctx) { it.copy(devices = listed, releases = published) }
    }

    private suspend fun reloadDevice(ctx: AuthContext, deviceId: String) {
        val latest = devicesFor(ctx).get(deviceId)
        val desired = devicesFor(ctx).desired(deviceId)
        if (!stillValid(ctx)) return
        val recovery = _state.value.recovery?.takeIf { it.matches(latest.id, latest.enrollmentPublicKey) }
        val history = devicesFor(ctx).recoveryHistory(deviceId).items
        if (!stillValid(ctx)) return
        commit(ctx) {
            it.copy(
                selectedDevice = latest,
                desired = desired,
                recovery = recovery,
                recoveryHistory = history,
                syncStatus = DeviceSync.status(latest.desiredRevision, latest.appliedRevision, latest.latestReport),
            )
        }
    }

    private fun refreshSelfUpdate(ctx: AuthContext) {
        val installed = updater?.ui()?.installedVersion ?: _state.value.selfUpdate.installedVersion
        val candidate = ControlSelfUpdate.selectCandidate(_state.value.releases, installed)
        val ui = (updater?.ui(candidate) ?: ControlSelfUpdateUi(
            phase = ControlSelfUpdatePhase.Failed,
            status = "Release trust root is not configured.",
        )).copy(discovery = _state.value.discovery)
        commit(ctx) { it.copy(selfUpdate = ui) }
    }

    private suspend fun applyDiscovery(ctx: AuthContext, catalogOnly: Boolean = false) {
        val snapshot = _state.value
        if (ControlUpdateDiscovery.shouldRefreshCatalog(snapshot.discovery, clock(), catalogPeriodMs)) {
            refreshDevices(ctx)
            if (!stillValid(ctx)) return
            val next = _state.value.discovery.copy(lastCatalogCheckAtMs = clock())
            commit(ctx) { it.copy(discovery = next, selfUpdate = it.selfUpdate.copy(discovery = next)) }
            discoveryStore?.save(next)
            refreshSelfUpdate(ctx)
            if (!catalogOnly && stillValid(ctx)) prepareCurrentCandidate(ctx)
        }
        val latest = _state.value.selfUpdate
        if (
            !catalogOnly &&
            ControlUpdateDiscovery.shouldRequestUnattendedInstall(
                settings = _state.value.discovery,
                phase = latest.phase,
                canInstall = latest.canInstall,
                pendingConfirmation = latest.canContinueConfirmation,
            ) &&
            !blocksInstall(latest.phase, latest.canContinueConfirmation) &&
            !mutations.isBusy()
        ) {
            installControlUpdate()
        }
    }

    private suspend fun prepareCurrentCandidate(ctx: AuthContext) {
        val snapshot = _state.value
        if (snapshot.selfUpdate.canInstall) return
        if (blocksInstall(snapshot.selfUpdate.phase, snapshot.selfUpdate.canContinueConfirmation)) return
        val candidate = ControlSelfUpdate.selectCandidate(snapshot.releases, snapshot.selfUpdate.installedVersion) ?: return
        prepareCandidate(ctx, candidate)
        if (stillValid(ctx)) refreshSelfUpdate(ctx)
    }

    private suspend fun prepareCandidate(ctx: AuthContext, candidate: com.aleksclark.primertasks.client.Release) {
        val coordinator = updater ?: error("Release trust root is not configured.")
        val dir = downloadDir ?: error("Download directory is not available.")
        withContext(io) {
            val expected = coordinator.verify(candidate)
            if (!stillValid(ctx)) return@withContext
            val download = java.io.File.createTempFile("control-", ".apk", dir)
            try {
                download.outputStream().use { raw ->
                    ArchiveChecks.digestingSink(raw, expected.byteSize, expected.sha256).use { sink ->
                        tasksFor(ctx).downloadReleaseArtifact(candidate.id, sink)
                    }
                }
                if (!stillValid(ctx)) {
                    download.delete()
                    return@withContext
                }
                if (!stillValid(ctx)) {
                    download.delete()
                    return@withContext
                }
                coordinator.prepare(download, expected)
            } catch (error: Exception) {
                download.delete()
                throw error
            }
        }
    }

    private fun blocksInstall(phase: ControlSelfUpdatePhase, pendingConfirmation: Boolean): Boolean {
        if (pendingConfirmation) return true
        return phase == ControlSelfUpdatePhase.Failed ||
            phase == ControlSelfUpdatePhase.Deferred ||
            phase == ControlSelfUpdatePhase.WaitingConfirmation ||
            phase == ControlSelfUpdatePhase.NeedsSettings
    }

    private fun syncCatalogTicker() {
        if (!_state.value.discovery.periodicEnabled) {
            catalogJob?.cancel()
            catalogJob = null
            return
        }
        if (catalogJob?.isActive == true) return
        catalogJob = viewModelScope.launch {
            while (isActive) {
                val wait = remainingCatalogDelay()
                if (wait > 0) delay(wait)
                val tick = workScope.launch {
                    val ctx = try {
                        captureAuth()
                    } catch (error: CancellationException) {
                        throw error
                    } catch (error: Exception) {
                        _state.value = _state.value.copy(message = controlMessage(error))
                        null
                    } ?: return@launch
                    runAct(ctx, ConflictResource.None) { applyDiscovery(it, catalogOnly = true) }
                }
                try {
                    tick.join()
                } catch (_: CancellationException) {
                    // Auth fence cancelled an in-flight catalog tick; the next period retries.
                }
                // Failure does not advance lastCatalogCheckAtMs. Rate-limit every
                // completed attempt so an overdue success timestamp cannot hot-loop.
                delay(catalogPeriodMs)
            }
        }
    }

    private suspend fun refreshAuth() {
        val attemptEpoch = epoch.current()
        val ready = try {
            identity.ready()
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            false
        }
        if (!epoch.isCurrent(attemptEpoch)) return
        if (!ready) {
            _state.value = _state.value.copy(ready = true, signedIn = false, householdOk = false, message = "Clerk did not become ready. Check the network and try again.")
            return
        }
        val signedIn = identity.isSignedIn()
        val sessionId = identity.sessionId()
        if (!epoch.isCurrent(attemptEpoch)) return
        if (!signedIn || sessionId == null || apiBase == null) {
            session = null
            if (_state.value.secondFactorRequired && !signedIn) {
                _state.value = _state.value.copy(ready = true, signedIn = false, householdOk = false)
            } else {
                _state.value = signedOut(signedIn = signedIn)
            }
            return
        }
        if (session != null && session?.sessionId != sessionId) {
            if (!epoch.isCurrent(attemptEpoch)) return
            fence()
            return
        }
        val token = try {
            identity.sessionToken(skipCache = true)
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            if (!epoch.isCurrent(attemptEpoch)) return
            _state.value = _state.value.copy(ready = true, signedIn = true, householdOk = false, message = controlMessage(error))
            return
        }
        val liveSid = identity.sessionId()
        if (!epoch.isCurrent(attemptEpoch)) return
        if (token.isNullOrBlank() || liveSid != sessionId) {
            session = null
            _state.value = signedOut(signedIn = true, message = "Sign in again. The parent session is missing or expired.")
            return
        }
        val ctx = AuthContext(sessionId = liveSid, epoch = attemptEpoch, token = token)
        session = ctx
        val claims = identity.sessionClaims()
        try {
            tasksFor(ctx).session()
            commit(ctx) {
                it.copy(
                    ready = true,
                    signedIn = true,
                    householdOk = true,
                    message = null,
                    logoutIncomplete = false,
                    pairing = null,
                    enrollment = null,
                    secondFactorRequired = false,
                    secondFactorCode = "",
                    secondFactorStrategies = emptyList(),
                    selectedSecondFactor = "",
                    clerkAuthorizedParty = claims?.authorizedParty,
                    clerkIssuer = claims?.issuer,
                )
            }
            if (_state.value.discovery.periodicEnabled && catalogJob?.isActive != true) syncCatalogTicker()
            if (stillValid(ctx)) refreshStudents(ctx)
        } catch (error: CancellationException) {
            throw error
        } catch (error: TasksHttpException) {
            if (!stillValid(ctx)) return
            if (error.statusCode == 401 || error.statusCode == 403) {
                fence()
                session = null
                _state.value = signedOut(
                    signedIn = true,
                    message = controlMessage(error),
                    clerkAuthorizedParty = claims?.authorizedParty,
                    clerkIssuer = claims?.issuer,
                )
            } else {
                commit(ctx) {
                    it.copy(
                        ready = true,
                        signedIn = true,
                        householdOk = false,
                        message = controlMessage(error),
                        clerkAuthorizedParty = claims?.authorizedParty,
                        clerkIssuer = claims?.issuer,
                    )
                }
            }
        } catch (error: Exception) {
            if (stillValid(ctx)) commit(ctx) { it.copy(ready = true, signedIn = true, householdOk = false, message = controlMessage(error)) }
        }
    }

    private fun remainingCatalogDelay(): Long {
        val last = _state.value.discovery.lastCatalogCheckAtMs
        if (last <= 0) return catalogPeriodMs
        val elapsed = clock() - last
        return (catalogPeriodMs - elapsed).coerceAtLeast(0L)
    }

    private fun fence() {
        epoch.bump()
        updater?.invalidatePrepared()
        workJob.cancel()
        workJob = SupervisorJob(viewModelScope.coroutineContext.job)
    }

    private fun stillValid(ctx: AuthContext): Boolean {
        val live = session
        return epoch.isCurrent(ctx.epoch) && live != null && live.sessionId == ctx.sessionId
    }

    private fun tasksFor(ctx: AuthContext) = tasksFactory(CredentialProvider { ctx.token })
    private fun devicesFor(ctx: AuthContext) = devicesFactory(CredentialProvider { ctx.token })

    private fun signedOut(
        signedIn: Boolean = false,
        message: String? = null,
        clerkAuthorizedParty: String? = null,
        clerkIssuer: String? = null,
    ): ControlUiState {
        catalogJob?.cancel()
        catalogJob = null
        return ControlUiState(
            ready = true,
            signedIn = signedIn,
            householdOk = false,
            email = _state.value.email,
            message = message,
            discovery = _state.value.discovery,
            selfUpdate = _state.value.selfUpdate.copy(discovery = _state.value.discovery),
            clerkAuthorizedParty = clerkAuthorizedParty,
            clerkIssuer = clerkIssuer,
        )
    }

    private fun commit(ctx: AuthContext, transform: (ControlUiState) -> ControlUiState) {
        if (!stillValid(ctx)) return
        _state.value = transform(_state.value)
    }

    private fun mutate(resource: ConflictResource = ConflictResource.None, block: suspend (AuthContext) -> Unit) {
        if (!mutations.tryBegin()) {
            _state.value = _state.value.copy(message = "Wait for the current change to finish.")
            return
        }
        val job = workScope.launch {
            val ctx = try {
                captureAuth()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _state.value = _state.value.copy(message = controlMessage(error))
                null
            }
            if (ctx == null) return@launch
            commit(ctx) { it.copy(mutating = true) }
            try {
                runAct(ctx, resource, block)
            } finally {
                if (stillValid(ctx)) commit(ctx) { it.copy(mutating = false) }
            }
        }
        job.invokeOnCompletion { mutations.end() }
    }

    private fun act(resource: ConflictResource = ConflictResource.None, block: suspend (AuthContext) -> Unit) {
        workScope.launch {
            val ctx = captureAuth() ?: return@launch
            runAct(ctx, resource, block)
        }
    }

    private suspend fun captureAuth(): AuthContext? {
        val existing = session
        if (existing == null || !epoch.isCurrent(existing.epoch)) return null
        val sessionId = try {
            identity.sessionId()
        } catch (error: CancellationException) {
            throw error
        }
        if (sessionId == null || sessionId != existing.sessionId) {
            fence()
            session = null
            _state.value = signedOut(message = "Sign in again. The parent session is missing or expired.")
            return null
        }
        val token = try {
            identity.sessionToken(skipCache = true)
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            _state.value = _state.value.copy(message = controlMessage(error))
            return null
        }
        val liveSid = identity.sessionId()
        if (!epoch.isCurrent(existing.epoch)) return null
        if (token.isNullOrBlank() || liveSid != existing.sessionId) {
            fence()
            session = null
            _state.value = signedOut(signedIn = true, message = "Sign in again. The parent session is missing or expired.")
            return null
        }
        if (session?.sessionId != liveSid) return null
        val ctx = AuthContext(sessionId = liveSid, epoch = existing.epoch, token = token)
        session = ctx
        return ctx
    }

    private suspend fun runAct(ctx: AuthContext, resource: ConflictResource, block: suspend (AuthContext) -> Unit) {
        try {
            if (!stillValid(ctx)) return
            block(ctx)
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            if (!stillValid(ctx)) return
            if (error is TasksHttpException && (error.statusCode == 401 || error.statusCode == 403)) {
                fence()
                session = null
                _state.value = signedOut(signedIn = true, message = controlMessage(error))
                return
            }
            if (error is TasksHttpException && error.statusCode == 409) {
                handleConflict(ctx, resource, error)
                return
            }
            commit(ctx) { it.copy(message = controlMessage(error)) }
        }
    }

    private suspend fun handleConflict(ctx: AuthContext, resource: ConflictResource, error: TasksHttpException) {
        val message = controlMessage(error)
        when {
            ConflictRefresh.shouldRefreshOccurrence(resource) -> {
                val id = _state.value.selectedOccurrence?.id
                if (id != null) {
                    runCatching { tasksFor(ctx).getOccurrence(id) }.onSuccess { occurrence ->
                        commit(ctx) { it.copy(selectedOccurrence = occurrence, message = message) }
                        return
                    }
                }
            }
            ConflictRefresh.shouldRefreshSchedule(resource) -> refreshSchedules(ctx)
            ConflictRefresh.shouldRefreshDevice(resource) -> {
                val device = _state.value.selectedDevice
                if (device != null) reloadDevice(ctx, device.id)
            }
        }
        commit(ctx) { it.copy(message = message) }
    }
}
