package com.aleksclark.primer.control

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.aleksclark.primer.control.device.ApprovedAppDraft
import com.aleksclark.primer.control.device.DeviceRepository
import com.aleksclark.primer.control.device.DeviceSync
import com.aleksclark.primer.control.device.DeviceSyncStatus
import com.aleksclark.primer.control.device.RecoveryBinding
import com.aleksclark.primer.control.tasks.AuthEpoch
import com.aleksclark.primer.control.tasks.ConflictRefresh
import com.aleksclark.primer.control.tasks.ConflictResource
import com.aleksclark.primer.control.tasks.FailClosedLogout
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
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

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
)

class ControlViewModel(
    private val identity: ParentIdentity,
    private val apiBase: String?,
) : ViewModel() {
    private val tokenMutex = Mutex()
    private val epoch = AuthEpoch()
    private val mutations = MutationGate()
    private val studentQueryGen = AtomicLong(0)
    private val taskQueryGen = AtomicLong(0)
    private val token = CredentialProvider { cachedToken }
    private var cachedToken: String? = null
    private val tasks by lazy { ParentTasksRepository(requireNotNull(apiBase), token) }
    private val devices by lazy { DeviceRepository(requireNotNull(apiBase), token) }

    private val _state = MutableStateFlow(ControlUiState())
    val state: StateFlow<ControlUiState> = _state

    init {
        viewModelScope.launch { refreshAuth() }
    }

    fun onResume() {
        if (_state.value.householdOk) viewModelScope.launch { refreshToken(skipCache = true) }
    }

    fun clearPassword() {
        if (_state.value.password.isNotEmpty()) _state.value = _state.value.copy(password = "")
    }

    fun signIn() = viewModelScope.launch {
        val email = _state.value.email.trim()
        val password = _state.value.password
        _state.value = _state.value.copy(password = "", message = null)
        when (val outcome = identity.signIn(email, password)) {
            is SignInOutcome.SignedIn -> {
                epoch.bump()
                refreshAuth()
            }
            is SignInOutcome.Incomplete -> _state.value = _state.value.copy(ready = true, signedIn = false, householdOk = false, message = outcome.message)
            is SignInOutcome.Failed -> _state.value = _state.value.copy(ready = true, signedIn = false, householdOk = false, message = outcome.message)
        }
    }

    fun signOut() = viewModelScope.launch {
        val tokenPresent = cachedToken != null && apiBase != null
        val server = if (tokenPresent) {
            runCatching {
                refreshToken(skipCache = true)
                tasks.logout()
                true to null
            }.getOrElse { false to controlMessage(it) }
        } else {
            true to null
        }
        val provider = if (server.first) {
            when (val out = identity.signOutProvider()) {
                SignOutOutcome.SignedOut -> true to null
                is SignOutOutcome.Failed -> false to out.message
            }
        } else {
            false to null
        }
        when (val decision = FailClosedLogout.decide(server.first, provider.first, server.second, provider.second)) {
            LogoutDecision.ClearSession -> {
                epoch.bump()
                cachedToken = null
                _state.value = signedOut(message = null)
            }
            is LogoutDecision.Incomplete -> {
                _state.value = _state.value.copy(message = decision.message, logoutIncomplete = true)
            }
        }
    }

    fun setStudentQuery(value: String) {
        _state.value = _state.value.copy(studentQuery = value)
        loadStudents()
    }

    fun loadStudents(reset: Boolean = true) = act { refreshStudents(reset) }

    private suspend fun refreshStudents(reset: Boolean = true) {
        val query = _state.value.studentQuery
        val gen = studentQueryGen.incrementAndGet()
        val snap = epoch.current()
        val offset = if (reset) 0L else _state.value.students.size.toLong()
        val page = tasks.listStudents(query, offset)
        if (!epoch.isCurrent(snap) || gen != studentQueryGen.get()) return
        val items = if (reset) page.items else _state.value.students + page.items
        _state.value = _state.value.copy(students = items, studentsHasMore = items.size < page.totalCount.toInt(), message = null)
    }

    fun openStudent(student: Student) = act {
        val found = tasks.getStudent(student.id)
        commit { it.copy(selectedStudent = found, studentName = found.displayName, pairing = null, creatingStudent = false) }
    }

    fun saveStudent() = mutate {
        val student = _state.value.selectedStudent
        val name = _state.value.studentName.trim()
        if (student == null) tasks.createStudent(name) else tasks.updateStudent(student.id, name)
        refreshStudents()
        commit { it.copy(selectedStudent = null, studentName = "", creatingStudent = false) }
    }

    fun archiveStudent() = mutate(ConflictResource.Student) {
        val id = _state.value.selectedStudent?.id ?: return@mutate
        tasks.archiveStudent(id)
        refreshStudents()
        commit { it.copy(selectedStudent = null, creatingStudent = false) }
    }

    fun issuePairing() = mutate {
        val id = _state.value.selectedStudent?.id ?: return@mutate
        val pairing = tasks.issuePairing(id)
        commit { it.copy(pairing = pairing) }
    }

    fun loadTasks(reset: Boolean = true) = act { refreshTasks(reset) }

    private suspend fun refreshTasks(reset: Boolean = true) {
        val query = _state.value.taskQuery
        val gen = taskQueryGen.incrementAndGet()
        val snap = epoch.current()
        val offset = if (reset) 0L else _state.value.tasks.size.toLong()
        val page = tasks.listTasks(query, offset, "active")
        if (!epoch.isCurrent(snap) || gen != taskQueryGen.get()) return
        val items = if (reset) page.items else _state.value.tasks + page.items
        commit { it.copy(tasks = items, tasksHasMore = items.size < page.totalCount.toInt()) }
    }

    fun saveTask() = mutate(ConflictResource.Task) {
        val editing = _state.value.editingTask
        val title = _state.value.taskTitle.trim()
        val instructions = _state.value.taskInstructions
        if (editing == null) tasks.createTask(title, instructions)
        else tasks.reviseTask(editing.templateId, title, instructions, editing.requirements)
        refreshTasks()
        commit { it.copy(editingTask = null, taskTitle = "", taskInstructions = "Complete the task, then ask a parent to check it.") }
    }

    fun publish(task: TaskRevision) = mutate(ConflictResource.Task) { tasks.publishTask(task.id); refreshTasks() }
    fun retire(task: TaskRevision) = mutate(ConflictResource.Task) { tasks.retireTask(task.templateId); refreshTasks() }

    fun loadSchedules(reset: Boolean = true) = act { refreshSchedules(reset) }

    private suspend fun refreshSchedules(reset: Boolean = true) {
        val offset = if (reset) 0L else _state.value.schedules.size.toLong()
        val page = tasks.listSchedules(offset, "active")
        val items = if (reset) page.items else _state.value.schedules + page.items
        commit { it.copy(schedules = items, schedulesHasMore = items.size < page.totalCount.toInt()) }
        if (reset) {
            refreshTasks()
            refreshStudents()
        }
    }

    fun saveSchedule() = mutate(ConflictResource.Schedule) {
        val state = _state.value
        val revision = ScheduleIdentity.fromPublishedTask(
            templateId = state.tasks.firstOrNull { it.id == state.scheduleTaskId }?.templateId ?: state.editingSchedule?.templateId,
            revisionId = state.tasks.firstOrNull { it.id == state.scheduleTaskId }?.id ?: state.editingSchedule?.revisionId,
        ) ?: error("Choose a published task revision from the server list.")
        val studentId = state.scheduleStudentId.ifBlank { state.editingSchedule?.studentId.orEmpty() }
        require(studentId.isNotBlank()) { "Choose a student." }
        val body = state.scheduleDraft.toInput(studentId, revision.templateId, revision.revisionId)
        if (state.editingSchedule == null) tasks.createSchedule(body) else tasks.updateSchedule(state.editingSchedule.id, body)
        refreshSchedules()
        commit { it.copy(editingSchedule = null, scheduleStudentId = "", scheduleTaskId = "", creatingSchedule = false) }
    }

    fun cancelSchedule(schedule: Schedule) = mutate(ConflictResource.Schedule) { tasks.retireSchedule(schedule.id); refreshSchedules() }

    fun loadOccurrences(reset: Boolean = true) = act { refreshOccurrences(reset) }

    private suspend fun refreshOccurrences(reset: Boolean = true) {
        val offset = if (reset) 0L else _state.value.occurrences.size.toLong()
        val page = tasks.listOccurrences("", "asc", offset)
        val items = if (reset) page.items else _state.value.occurrences + page.items
        commit { it.copy(occurrences = items, occurrencesHasMore = items.size < page.totalCount.toInt()) }
    }

    fun decide(accepted: Boolean) = mutate(ConflictResource.Occurrence) {
        val id = _state.value.selectedOccurrence?.id ?: return@mutate
        tasks.decide(id, accepted, _state.value.decisionReason)
        val occurrence = tasks.getOccurrence(id)
        refreshOccurrences()
        commit { it.copy(selectedOccurrence = occurrence) }
    }

    fun retryOccurrence() = mutate(ConflictResource.Occurrence) {
        val id = _state.value.selectedOccurrence?.id ?: return@mutate
        tasks.retry(id)
        val occurrence = tasks.getOccurrence(id)
        refreshOccurrences()
        commit { it.copy(selectedOccurrence = occurrence) }
    }

    fun skipOccurrence() = mutate(ConflictResource.Occurrence) {
        val id = _state.value.selectedOccurrence?.id ?: return@mutate
        tasks.skip(id)
        val occurrence = tasks.getOccurrence(id)
        refreshOccurrences()
        commit { it.copy(selectedOccurrence = occurrence) }
    }

    fun cancelOccurrence() = mutate(ConflictResource.Occurrence) {
        val id = _state.value.selectedOccurrence?.id ?: return@mutate
        tasks.cancel(id)
        val occurrence = tasks.getOccurrence(id)
        refreshOccurrences()
        commit { it.copy(selectedOccurrence = occurrence) }
    }

    fun loadDevices() = act { refreshDevices() }

    private suspend fun refreshDevices() {
        val listed = devices.list().items
        val published = devices.releases().items
        commit { it.copy(devices = listed, releases = published) }
    }

    fun openDevice(device: ManagedDevice) = act { reloadDevice(device) }

    private suspend fun reloadDevice(device: ManagedDevice) {
        resetRecoveryIfDeviceChanged(device)
        val desired = devices.desired(device.id)
        val latest = devices.get(device.id)
        commit {
            it.copy(
                selectedDevice = latest,
                desired = desired,
                syncStatus = DeviceSync.status(latest.desiredRevision, latest.appliedRevision, latest.latestReport),
            )
        }
    }

    fun issueEnrollment() = mutate {
        val enrollment = devices.issue()
        commit { it.copy(enrollment = enrollment) }
    }

    fun targetRelease(requeue: Boolean = false) = mutate(ConflictResource.Device) {
        val device = _state.value.selectedDevice ?: return@mutate
        val release = _state.value.releases.firstOrNull { it.id == _state.value.selectedReleaseId }
            ?: error("Choose a published release.")
        val desired = devices.desired(device.id)
        devices.target(device.id, release, desired, requeue)
        reloadDevice(device)
    }

    fun quarantineDevice() = mutate(ConflictResource.Device) {
        val device = _state.value.selectedDevice ?: return@mutate
        devices.quarantine(device.id, "parent quarantine")
        refreshDevices()
        reloadDevice(device)
    }

    fun revokeDevice() = mutate(ConflictResource.Device) {
        val device = _state.value.selectedDevice ?: return@mutate
        devices.revoke(device.id, "parent revoke")
        refreshDevices()
        val latest = devices.get(device.id)
        commit { it.copy(selectedDevice = latest, recovery = null) }
    }

    fun addApprovedApp() = mutate(ConflictResource.Device) {
        val device = _state.value.selectedDevice ?: return@mutate
        val desired = devices.desired(device.id)
        devices.addApprovedApp(device.id, desired, _state.value.approvedAppDraft)
        commit { it.copy(approvedAppDraft = ApprovedAppDraft()) }
        reloadDevice(device)
    }

    fun removeApprovedApp(packageName: String) = mutate(ConflictResource.Device) {
        val device = _state.value.selectedDevice ?: return@mutate
        devices.removeApprovedApp(device.id, devices.desired(device.id), packageName)
        reloadDevice(device)
    }

    fun setParentUnlock(allow: Boolean) = mutate(ConflictResource.Device) {
        val device = _state.value.selectedDevice ?: return@mutate
        devices.setParentUnlock(device.id, devices.desired(device.id), allow)
        reloadDevice(device)
    }

    fun prepareRecovery() = mutate {
        val device = _state.value.selectedDevice ?: return@mutate
        val publicKey = device.enrollmentPublicKey
        if (publicKey.isNullOrBlank()) {
            commit { it.copy(message = "This device has no enrollment public key yet.") }
            return@mutate
        }
        val prepared = devices.prepareRotation(device.id, publicKey)
        commit { it.copy(recovery = prepared, message = null) }
    }

    fun acknowledgeRecovery(value: Boolean) {
        val recovery = _state.value.recovery ?: return
        _state.value = _state.value.copy(recovery = recovery.copy(acknowledged = value))
    }

    fun rotateRecovery() = mutate(ConflictResource.Device) {
        val device = _state.value.selectedDevice ?: return@mutate
        devices.rotateRecovery(device.id, _state.value.recovery, _state.value.recovery?.acknowledged == true)
        commit { it.copy(recovery = null) }
    }

    fun closeDevice() {
        _state.value = _state.value.copy(selectedDevice = null, desired = null, recovery = null, syncStatus = null, approvedAppDraft = ApprovedAppDraft())
    }

    fun update(transform: (ControlUiState) -> ControlUiState) {
        _state.value = transform(_state.value)
    }

    private suspend fun refreshAuth() {
        val snap = epoch.current()
        val ready = runCatching { identity.ready() }.getOrDefault(false)
        if (!ready) {
            if (epoch.isCurrent(snap)) _state.value = _state.value.copy(ready = true, signedIn = false, householdOk = false, message = "Clerk did not become ready. Check the network and try again.")
            return
        }
        val signedIn = identity.isSignedIn()
        val token = if (signedIn) refreshToken(skipCache = true) else null
        if (!epoch.isCurrent(snap)) return
        if (!signedIn || apiBase == null || token == null) {
            _state.value = signedOut(signedIn = signedIn)
            return
        }
        try {
            tasks.session()
            if (!epoch.isCurrent(snap)) return
            _state.value = _state.value.copy(ready = true, signedIn = true, householdOk = true, message = null, logoutIncomplete = false)
            loadStudents()
        } catch (error: TasksHttpException) {
            if (!epoch.isCurrent(snap)) return
            if (error.statusCode == 401 || error.statusCode == 403) {
                epoch.bump()
                cachedToken = null
                _state.value = signedOut(signedIn = true, message = controlMessage(error))
            } else {
                _state.value = _state.value.copy(ready = true, signedIn = true, householdOk = false, message = controlMessage(error))
            }
        } catch (error: Exception) {
            if (epoch.isCurrent(snap)) _state.value = _state.value.copy(ready = true, signedIn = true, householdOk = false, message = controlMessage(error))
        }
    }

    private suspend fun refreshToken(skipCache: Boolean): String? = tokenMutex.withLock {
        val token = identity.sessionToken(skipCache)
        cachedToken = token
        token
    }

    private fun resetRecoveryIfDeviceChanged(device: ManagedDevice) {
        val recovery = _state.value.recovery ?: return
        if (!recovery.matches(device.id, device.enrollmentPublicKey)) {
            _state.value = _state.value.copy(recovery = null)
        }
    }

    private fun signedOut(signedIn: Boolean = false, message: String? = null) = ControlUiState(
        ready = true,
        signedIn = signedIn,
        householdOk = false,
        email = _state.value.email,
        message = message,
    )

    private fun commit(transform: (ControlUiState) -> ControlUiState) {
        _state.value = transform(_state.value)
    }

    private fun mutate(resource: ConflictResource = ConflictResource.None, block: suspend () -> Unit) {
        viewModelScope.launch {
            val snap = epoch.current()
            if (!mutations.tryBegin()) {
                _state.value = _state.value.copy(message = "Wait for the current change to finish.")
                return@launch
            }
            _state.value = _state.value.copy(mutating = true)
            try {
                runAct(resource, block)
            } finally {
                mutations.end()
                if (epoch.isCurrent(snap)) _state.value = _state.value.copy(mutating = false)
            }
        }
    }

    private fun act(resource: ConflictResource = ConflictResource.None, block: suspend () -> Unit) {
        viewModelScope.launch { runAct(resource, block) }
    }

    private suspend fun runAct(resource: ConflictResource, block: suspend () -> Unit) {
        val snap = epoch.current()
        try {
            if (refreshToken(skipCache = true) == null && _state.value.householdOk) {
                if (epoch.isCurrent(snap)) {
                    epoch.bump()
                    _state.value = signedOut(message = "Sign in again. The parent session is missing or expired.")
                }
                return
            }
            if (!epoch.isCurrent(snap)) return
            block()
        } catch (error: Exception) {
            if (!epoch.isCurrent(snap)) return
            if (error is TasksHttpException && (error.statusCode == 401 || error.statusCode == 403)) {
                epoch.bump()
                cachedToken = null
                _state.value = signedOut(signedIn = true, message = controlMessage(error))
                return
            }
            if (error is TasksHttpException && error.statusCode == 409) {
                handleConflict(resource, error)
                return
            }
            _state.value = _state.value.copy(message = controlMessage(error))
        }
    }

    private suspend fun handleConflict(resource: ConflictResource, error: TasksHttpException) {
        val message = controlMessage(error)
        when {
            ConflictRefresh.shouldRefreshOccurrence(resource) -> {
                val id = _state.value.selectedOccurrence?.id
                if (id != null) {
                    runCatching { tasks.getOccurrence(id) }.onSuccess { occurrence ->
                        _state.value = _state.value.copy(selectedOccurrence = occurrence, message = message)
                        return
                    }
                }
            }
            ConflictRefresh.shouldRefreshSchedule(resource) -> refreshSchedules()
            ConflictRefresh.shouldRefreshDevice(resource) -> {
                val device = _state.value.selectedDevice
                if (device != null) reloadDevice(device)
            }
        }
        _state.value = _state.value.copy(message = message)
    }
}
