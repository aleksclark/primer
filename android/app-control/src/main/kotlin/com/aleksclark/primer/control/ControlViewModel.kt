package com.aleksclark.primer.control

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.aleksclark.primer.control.device.DeviceRepository
import com.aleksclark.primer.control.tasks.ParentTasksRepository
import com.aleksclark.primer.control.tasks.ScheduleDraft
import com.aleksclark.primer.control.tasks.controlMessage
import com.aleksclark.primer.control.tasks.toInput
import com.aleksclark.primer.identity.ParentIdentity
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.Enrollment
import com.aleksclark.primertasks.client.ManagedDevice
import com.aleksclark.primertasks.client.Occurrence
import com.aleksclark.primertasks.client.Pairing
import com.aleksclark.primertasks.client.Release
import com.aleksclark.primertasks.client.Schedule
import com.aleksclark.primertasks.client.Student
import com.aleksclark.primertasks.client.TaskRevision
import com.aleksclark.primertasks.client.TasksHttpException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

data class ControlUiState(
    val ready: Boolean = false,
    val signedIn: Boolean = false,
    val householdOk: Boolean = false,
    val message: String? = null,
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
    val enrollment: Enrollment? = null,
    val releases: List<Release> = emptyList(),
    val selectedReleaseId: String = "",
    val email: String = "",
    val password: String = "",
    val recoveryCodes: List<String> = emptyList(),
    val recoveryAcknowledged: Boolean = false,
    val recoveryRequestId: String? = null,
    val recoveryPublicJson: ByteArray? = null,
    val creatingStudent: Boolean = false,
    val creatingSchedule: Boolean = false,
)

class ControlViewModel(
    private val identity: ParentIdentity,
    private val apiBase: String?,
) : ViewModel() {
    private val token = CredentialProvider { cachedToken }
    private var cachedToken: String? = null
    private val tasks by lazy { ParentTasksRepository(requireNotNull(apiBase), token) }
    private val devices by lazy { DeviceRepository(requireNotNull(apiBase), token) }

    private val _state = MutableStateFlow(ControlUiState())
    val state: StateFlow<ControlUiState> = _state

    init {
        viewModelScope.launch { refreshAuth() }
    }

    fun signIn() = viewModelScope.launch {
        val ok = identity.signIn(_state.value.email.trim(), _state.value.password)
        if (!ok) {
            _state.value = _state.value.copy(message = "Clerk sign-in failed. Check the account and try again.")
            return@launch
        }
        _state.value = _state.value.copy(password = "")
        refreshAuth()
    }

    fun signOut() = viewModelScope.launch {
        runCatching { if (apiBase != null && cachedToken != null) tasks.logout() }
            .onFailure { _state.value = _state.value.copy(message = controlMessage(it)) }
        identity.signOutProvider()
        cachedToken = null
        _state.value = ControlUiState(ready = true, signedIn = false, householdOk = false, message = _state.value.message)
    }

    fun setStudentQuery(value: String) {
        _state.value = _state.value.copy(studentQuery = value)
        loadStudents()
    }

    fun loadStudents() = act {
        val page = tasks.listStudents(_state.value.studentQuery, 0)
        _state.value = _state.value.copy(students = page.items, message = null)
    }

    fun openStudent(student: Student) = act {
        val found = tasks.getStudent(student.id)
        _state.value = _state.value.copy(selectedStudent = found, studentName = found.displayName, pairing = null, creatingStudent = false)
    }

    fun saveStudent() = act {
        val student = _state.value.selectedStudent
        val name = _state.value.studentName.trim()
        if (student == null) {
            tasks.createStudent(name)
        } else {
            tasks.updateStudent(student.id, name)
        }
        loadStudents()
        _state.value = _state.value.copy(selectedStudent = null, studentName = "", creatingStudent = false)
    }

    fun archiveStudent() = act {
        val id = _state.value.selectedStudent?.id ?: return@act
        tasks.archiveStudent(id)
        loadStudents()
        _state.value = _state.value.copy(selectedStudent = null)
    }

    fun issuePairing() = act {
        val id = _state.value.selectedStudent?.id ?: return@act
        _state.value = _state.value.copy(pairing = tasks.issuePairing(id))
    }

    fun loadTasks() = act {
        val page = tasks.listTasks(_state.value.taskQuery, 0, "active")
        _state.value = _state.value.copy(tasks = page.items)
    }

    fun saveTask() = act {
        val editing = _state.value.editingTask
        val title = _state.value.taskTitle.trim()
        val instructions = _state.value.taskInstructions
        if (editing == null) tasks.createTask(title, instructions)
        else tasks.reviseTask(editing.templateId, title, instructions, editing.requirements)
        loadTasks()
        _state.value = _state.value.copy(editingTask = null, taskTitle = "", taskInstructions = "Complete the task, then ask a parent to check it.")
    }

    fun publish(task: TaskRevision) = act { tasks.publishTask(task.id); loadTasks() }
    fun retire(task: TaskRevision) = act { tasks.retireTask(task.templateId); loadTasks() }

    fun loadSchedules() = act {
        val page = tasks.listSchedules(0, "active")
        _state.value = _state.value.copy(schedules = page.items)
        loadTasks()
        loadStudents()
    }

    fun saveSchedule() = act {
        val state = _state.value
        val task = state.tasks.firstOrNull { it.id == state.scheduleTaskId } ?: state.editingSchedule?.let { existing ->
            TaskRevision(id = existing.revisionId, templateId = existing.templateId, version = 1, title = existing.title.orEmpty(), instructions = "", status = "published", requirements = emptyList(), createdAt = "")
        }
        val studentId = state.scheduleStudentId.ifBlank { state.editingSchedule?.studentId.orEmpty() }
        val body = state.scheduleDraft.toInput(studentId, task?.templateId ?: state.editingSchedule?.templateId.orEmpty(), task?.id ?: state.editingSchedule?.revisionId.orEmpty())
        if (state.editingSchedule == null) tasks.createSchedule(body) else tasks.updateSchedule(state.editingSchedule.id, body)
        loadSchedules()
        _state.value = _state.value.copy(editingSchedule = null, scheduleStudentId = "", scheduleTaskId = "", creatingSchedule = false)
    }

    fun cancelSchedule(schedule: Schedule) = act { tasks.retireSchedule(schedule.id); loadSchedules() }

    fun loadOccurrences() = act {
        val page = tasks.listOccurrences("", "asc")
        _state.value = _state.value.copy(occurrences = page.items)
    }

    fun decide(accepted: Boolean) = act {
        val id = _state.value.selectedOccurrence?.id ?: return@act
        tasks.decide(id, accepted, _state.value.decisionReason)
        loadOccurrences()
        _state.value = _state.value.copy(selectedOccurrence = tasks.getOccurrence(id))
    }

    fun retryOccurrence() = act {
        val id = _state.value.selectedOccurrence?.id ?: return@act
        tasks.retry(id)
        _state.value = _state.value.copy(selectedOccurrence = tasks.getOccurrence(id))
        loadOccurrences()
    }
    fun skipOccurrence() = act {
        val id = _state.value.selectedOccurrence?.id ?: return@act
        tasks.skip(id)
        _state.value = _state.value.copy(selectedOccurrence = tasks.getOccurrence(id))
        loadOccurrences()
    }
    fun cancelOccurrence() = act {
        val id = _state.value.selectedOccurrence?.id ?: return@act
        tasks.cancel(id)
        _state.value = _state.value.copy(selectedOccurrence = tasks.getOccurrence(id))
        loadOccurrences()
    }

    fun loadDevices() = act {
        _state.value = _state.value.copy(devices = devices.list().items, releases = devices.releases().items)
    }

    fun issueEnrollment() = act { _state.value = _state.value.copy(enrollment = devices.issue()) }
    fun targetRelease() = act {
        val device = _state.value.selectedDevice ?: return@act
        devices.target(device.id, _state.value.selectedReleaseId)
        _state.value = _state.value.copy(selectedDevice = devices.get(device.id))
    }
    fun quarantineDevice() = act {
        val device = _state.value.selectedDevice ?: return@act
        _state.value = _state.value.copy(selectedDevice = devices.quarantine(device.id, "parent quarantine"))
        loadDevices()
    }
    fun revokeDevice() = act {
        val device = _state.value.selectedDevice ?: return@act
        _state.value = _state.value.copy(selectedDevice = devices.revoke(device.id, "parent revoke"))
        loadDevices()
    }
    fun prepareRecovery() = act {
        val device = _state.value.selectedDevice ?: return@act
        val publicKey = device.enrollmentPublicKey
        if (publicKey.isNullOrBlank()) {
            _state.value = _state.value.copy(message = "This device has no enrollment public key yet.")
            return@act
        }
        val prepared = devices.prepareRotation(publicKey)
        _state.value = _state.value.copy(
            recoveryCodes = prepared.codes,
            recoveryRequestId = prepared.requestId,
            recoveryPublicJson = prepared.publicJson,
            recoveryAcknowledged = false,
            message = null,
        )
    }
    fun rotateRecovery() = act {
        val device = _state.value.selectedDevice ?: return@act
        val requestId = _state.value.recoveryRequestId ?: return@act
        val publicJson = _state.value.recoveryPublicJson ?: return@act
        check(_state.value.recoveryAcknowledged) { "Store recovery codes off this device before rotating." }
        devices.rotateRecovery(
            device.id,
            DeviceRepository.PreparedRotation(requestId, _state.value.recoveryCodes, publicJson),
            acknowledged = true,
        )
        _state.value = _state.value.copy(recoveryCodes = emptyList(), recoveryAcknowledged = false, recoveryRequestId = null, recoveryPublicJson = null)
    }

    fun update(transform: (ControlUiState) -> ControlUiState) {
        _state.value = transform(_state.value)
    }

    private suspend fun refreshAuth() {
        identity.ready()
        val signedIn = identity.isSignedIn()
        cachedToken = if (signedIn) identity.sessionToken() else null
        if (!signedIn || apiBase == null || cachedToken == null) {
            _state.value = _state.value.copy(ready = true, signedIn = signedIn, householdOk = false)
            return
        }
        try {
            tasks.session()
            _state.value = _state.value.copy(ready = true, signedIn = true, householdOk = true, message = null)
            loadStudents()
        } catch (error: TasksHttpException) {
            _state.value = _state.value.copy(ready = true, signedIn = true, householdOk = false, message = controlMessage(error))
        } catch (error: Exception) {
            _state.value = _state.value.copy(ready = true, signedIn = true, householdOk = false, message = controlMessage(error))
        }
    }

    private fun act(block: suspend () -> Unit) {
        viewModelScope.launch {
            try {
                block()
            } catch (error: Exception) {
                _state.value = _state.value.copy(message = controlMessage(error))
            }
        }
    }
}
