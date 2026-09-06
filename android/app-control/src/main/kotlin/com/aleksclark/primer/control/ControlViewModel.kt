package com.aleksclark.primer.control

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.aleksclark.primer.control.device.ApprovedAppDraft
import com.aleksclark.primer.control.device.ControlSelfUpdateUi
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
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.Schedule
import com.aleksclark.primertasks.client.Student
import com.aleksclark.primertasks.client.TaskRevision
import com.aleksclark.primertasks.client.TasksHttpException
import java.util.concurrent.atomic.AtomicLong
import kotlin.coroutines.cancellation.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
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
    val decisionReason: String = "Parent observed completion.",
    val devices: List<ManagedDevice> = emptyList(),
    val selectedDevice: ManagedDevice? = null,
    val desired: DesiredState? = null,
    val syncStatus: DeviceSyncStatus? = null,
    val enrollment: Enrollment? = null,
    val releases: List<Release> = emptyList(),
    val selectedReleaseId: String = "",
    val email: String = "",
    val password: String = "",
    val recovery: RecoveryBinding? = null,
    val approvedAppDraft: ApprovedAppDraft = ApprovedAppDraft(),
    val creatingStudent: Boolean = false,
    val creatingSchedule: Boolean = false,
    val studentsHasMore: Boolean = false,
    val tasksHasMore: Boolean = false,
    val schedulesHasMore: Boolean = false,
    val occurrencesHasMore: Boolean = false,
    val selfUpdate: ControlSelfUpdateUi = ControlSelfUpdateUi(),
)

class ControlViewModel(
    private val identity: ParentIdentity,
    private val apiBase: String?,
    private val http: OkHttpClient = OkHttpClient(),
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

    private val _state = MutableStateFlow(ControlUiState())
    val state: StateFlow<ControlUiState> = _state

    init {
        viewModelScope.launch { refreshAuth() }
    }

    fun onResume() {
        viewModelScope.launch {
            try {
                refreshAuth()
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                _state.value = _state.value.copy(message = controlMessage(error))
            }
        }
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
                when (val outcome = identity.signIn(email, password)) {
                    is SignInOutcome.SignedIn -> {
                        val attemptEpoch = epoch.current()
                        val token = identity.sessionToken(skipCache = true)
                        val liveSid = identity.sessionId()
                        if (!epoch.isCurrent(attemptEpoch)) return@launch
                        if (token.isNullOrBlank() || liveSid != outcome.sessionId) {
                            _state.value = signedOut(message = "Clerk did not activate the newly created session.")
                            return@launch
                        }
                        session = AuthContext(sessionId = liveSid, epoch = attemptEpoch, token = token)
                        logoutAttempt = null
                        refreshAuth()
                    }
                    is SignInOutcome.Incomplete -> _state.value = _state.value.copy(ready = true, signedIn = false, householdOk = false, message = outcome.message)
                    is SignInOutcome.Failed -> _state.value = _state.value.copy(ready = true, signedIn = false, householdOk = false, message = outcome.message)
                }
            } finally {
                authGate.end()
            }
        }
    }

    fun signOut() {
        if (!authGate.tryBegin()) {
            _state.value = _state.value.copy(message = "Wait for the current change to finish.")
            return
        }
        viewModelScope.launch {
            try {
                signOutLocked()
            } finally {
                authGate.end()
            }
        }
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
            commit(ctx) { it.copy(editingTask = null, taskTitle = "", taskInstructions = "Complete the task, then ask a parent to check it.") }
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
        val body = snapshot.scheduleDraft.toInput(studentId, revision.templateId, revision.revisionId)
        val editingId = snapshot.editingSchedule?.id
        mutate(ConflictResource.Schedule) { ctx ->
            if (editingId == null) tasksFor(ctx).createSchedule(body) else tasksFor(ctx).updateSchedule(editingId, body)
            refreshSchedules(ctx)
            commit(ctx) { it.copy(editingSchedule = null, scheduleStudentId = "", scheduleTaskId = "", creatingSchedule = false) }
        }
    }

    fun cancelSchedule(schedule: Schedule) = mutate(ConflictResource.Schedule) { ctx ->
        tasksFor(ctx).retireSchedule(schedule.id)
        refreshSchedules(ctx)
    }

    fun loadOccurrences(reset: Boolean = true) = act { ctx -> refreshOccurrences(ctx, reset) }

    fun decide(accepted: Boolean) {
        val id = _state.value.selectedOccurrence?.id ?: return
        val reason = _state.value.decisionReason
        mutate(ConflictResource.Occurrence) { ctx ->
            tasksFor(ctx).decide(id, accepted, reason)
            val occurrence = tasksFor(ctx).getOccurrence(id)
            refreshOccurrences(ctx)
            commit(ctx) { it.copy(selectedOccurrence = occurrence) }
        }
    }

    fun retryOccurrence() {
        val id = _state.value.selectedOccurrence?.id ?: return
        mutate(ConflictResource.Occurrence) { ctx ->
            tasksFor(ctx).retry(id)
            val occurrence = tasksFor(ctx).getOccurrence(id)
            refreshOccurrences(ctx)
            commit(ctx) { it.copy(selectedOccurrence = occurrence) }
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

    fun loadDevices() = act { ctx -> refreshDevices(ctx) }

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
        _state.value = _state.value.copy(selectedDevice = null, desired = null, recovery = null, syncStatus = null, approvedAppDraft = ApprovedAppDraft())
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

    private suspend fun refreshTasks(ctx: AuthContext, reset: Boolean = true) {
        val query = _state.value.taskQuery
        val gen = taskQueryGen.incrementAndGet()
        val offset = if (reset) 0L else _state.value.tasks.size.toLong()
        val page = tasksFor(ctx).listTasks(query, offset, "active")
        if (!stillValid(ctx) || gen != taskQueryGen.get()) return
        val items = if (reset) page.items else _state.value.tasks + page.items
        commit(ctx) { it.copy(tasks = items, tasksHasMore = items.size < page.totalCount.toInt()) }
    }

    private suspend fun refreshSchedules(ctx: AuthContext, reset: Boolean = true) {
        val offset = if (reset) 0L else _state.value.schedules.size.toLong()
        val page = tasksFor(ctx).listSchedules(offset, "active")
        val items = if (reset) page.items else _state.value.schedules + page.items
        commit(ctx) { it.copy(schedules = items, schedulesHasMore = items.size < page.totalCount.toInt()) }
        if (reset && stillValid(ctx)) {
            refreshTasks(ctx)
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
        commit(ctx) {
            it.copy(
                selectedDevice = latest,
                desired = desired,
                recovery = recovery,
                syncStatus = DeviceSync.status(latest.desiredRevision, latest.appliedRevision, latest.latestReport),
            )
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
            _state.value = signedOut(signedIn = signedIn)
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
        try {
            tasksFor(ctx).session()
            commit(ctx) { it.copy(ready = true, signedIn = true, householdOk = true, message = null, logoutIncomplete = false) }
            if (stillValid(ctx)) refreshStudents(ctx)
        } catch (error: CancellationException) {
            throw error
        } catch (error: TasksHttpException) {
            if (!stillValid(ctx)) return
            if (error.statusCode == 401 || error.statusCode == 403) {
                fence()
                session = null
                _state.value = signedOut(signedIn = true, message = controlMessage(error))
            } else {
                commit(ctx) { it.copy(ready = true, signedIn = true, householdOk = false, message = controlMessage(error)) }
            }
        } catch (error: Exception) {
            if (stillValid(ctx)) commit(ctx) { it.copy(ready = true, signedIn = true, householdOk = false, message = controlMessage(error)) }
        }
    }

    private fun fence() {
        epoch.bump()
        workJob.cancel()
        workJob = SupervisorJob(viewModelScope.coroutineContext.job)
    }

    private fun stillValid(ctx: AuthContext): Boolean {
        val live = session
        return epoch.isCurrent(ctx.epoch) && live != null && live.sessionId == ctx.sessionId
    }

    private fun tasksFor(ctx: AuthContext) = tasksFactory(CredentialProvider { ctx.token })
    private fun devicesFor(ctx: AuthContext) = devicesFactory(CredentialProvider { ctx.token })

    private fun signedOut(signedIn: Boolean = false, message: String? = null) = ControlUiState(
        ready = true,
        signedIn = signedIn,
        householdOk = false,
        email = _state.value.email,
        message = message,
    )

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
