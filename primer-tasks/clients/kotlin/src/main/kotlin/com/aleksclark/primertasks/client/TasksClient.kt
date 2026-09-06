package com.aleksclark.primertasks.client

import com.aleksclark.primertasks.generated.AuthKind
import com.aleksclark.primertasks.generated.GeneratedTasksApi
import java.net.URI
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient

fun interface CredentialProvider {
    fun token(): String?
}

/**
 * Stable package façade over generated Tasks methods and models.
 *
 * Parent, student-device, and management-device credentials are distinct.
 * The mounted API origin is preserved: `/tasks/api` stays `/tasks/api`.
 */
class TasksClient(
    baseUrl: String,
    http: OkHttpClient = OkHttpClient(),
    private val parentCredentials: CredentialProvider? = null,
    private val deviceCredentials: CredentialProvider? = null,
    private val managementCredentials: CredentialProvider? = null,
) {
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = false }
    private val api = GeneratedTasksApi(
        apiBaseUrl = apiOrigin(baseUrl),
        http = http,
        json = json,
        credentials = { kind ->
            when (kind) {
                AuthKind.PARENT -> parentCredentials?.token()
                AuthKind.DEVICE -> deviceCredentials?.token()
                AuthKind.MANAGEMENT -> managementCredentials?.token()
                AuthKind.NONE -> null
            }
        },
    )

    suspend fun health(): Health = io { api.health() }

    suspend fun parentSession(): Session = io { api.authSession() }
    suspend fun logout(): StatusResponse = io { api.authLogout() }

    suspend fun listStudents(query: StudentsListQuery = StudentsListQuery()): StudentPage =
        io { api.studentsList(query) }
    suspend fun getStudent(id: String): Student = io { api.studentsGet(id) }
    suspend fun createStudent(body: CreateStudent): Student = io { api.studentsCreate(body) }
    suspend fun updateStudent(id: String, body: UpdateStudent): Student = io { api.studentsUpdate(id, body) }
    suspend fun archiveStudent(id: String) = io { api.studentsArchive(id) }
    suspend fun issuePairing(id: String): Pairing = io { api.studentsPairing(id) }

    suspend fun listTasks(query: TasksListQuery = TasksListQuery()): TaskPage = io { api.tasksList(query) }
    suspend fun createTask(body: TaskInput): TaskRevision = io { api.tasksCreate(body) }
    suspend fun reviseTask(id: String, body: TaskInput): TaskRevision = io { api.tasksRevise(id, body) }
    suspend fun publishTask(id: String): TaskRevision = io { api.tasksPublish(id) }
    suspend fun retireTask(id: String) = io { api.tasksRetire(id) }

    suspend fun listSchedules(query: SchedulesListQuery = SchedulesListQuery()): SchedulePage =
        io { api.schedulesList(query) }
    suspend fun createSchedule(body: ScheduleInput): Schedule = io { api.schedulesCreate(body) }
    suspend fun updateSchedule(id: String, body: ScheduleInput): Schedule = io { api.schedulesUpdate(id, body) }
    suspend fun retireSchedule(id: String) = io { api.schedulesRetire(id) }

    suspend fun listOccurrences(query: OccurrencesListQuery = OccurrencesListQuery()): OccurrencePage =
        io { api.occurrencesList(query) }
    suspend fun getOccurrence(id: String): OccurrenceResponse = io { api.occurrenceGet(id) }
    suspend fun decideOccurrence(id: String, body: DecisionInput): OccurrenceDecision =
        io { api.occurrenceApprove(id, body) }
    suspend fun retryOccurrence(id: String): OccurrenceRetry = io { api.occurrenceRetry(id) }
    suspend fun skipOccurrence(id: String): OccurrenceAction = io { api.occurrenceSkip(id) }
    suspend fun cancelOccurrence(id: String): OccurrenceAction = io { api.occurrenceCancel(id) }

    suspend fun pairDevice(code: String): DevicePairResponse = io { api.devicePair(PairCode(code)) }
    suspend fun studentProfile(token: String): StudentProfile = io { api.deviceProfile(token) }
    suspend fun studentChecklist(token: String): ChecklistResponse = io { api.deviceChecklist(token) }
    suspend fun studentToday(token: String): OccurrencePageResponse = io { api.deviceToday(token) }
    suspend fun studentUpcoming(token: String): OccurrencePageResponse = io { api.deviceUpcoming(token) }
    suspend fun studentOccurrence(token: String, id: String): OccurrenceResponse =
        io { api.deviceOccurrence(id, token) }
    suspend fun startStudentOccurrence(token: String, id: String): OccurrenceAction =
        io { api.deviceOccurrenceStart(id, token) }
    suspend fun submitStudentOccurrence(token: String, id: String): OccurrenceAction =
        io { api.deviceOccurrenceSubmit(id, token) }

    suspend fun managementDeviceDesired(): DesiredState = io { api.managementDeviceDesired() }
    suspend fun managementDeviceEnroll(body: EnrollInput): EnrollResult = io { api.managementDeviceEnroll(body) }
    suspend fun managementDeviceReport(body: PolicyReportInput): PolicyReport =
        io { api.managementDeviceReport(body) }
    suspend fun managementDeviceConfirmRecovery(id: String, body: RecoveryConfirmInput): StatusResponse =
        io { api.managementDeviceRecoveryConfirm(id, body) }

    suspend fun listManagedDevices(): ManagedDevicePage = io { api.managedDevicesList() }
    suspend fun getManagedDevice(id: String): ManagedDevice = io { api.managedDevicesGet(id) }
    suspend fun issueManagedEnrollment(body: IssueEnrollmentInput): Enrollment =
        io { api.managedDevicesEnrollmentsCreate(body) }
    suspend fun abandonManagedEnrollment(id: String) = io { api.managedDevicesEnrollmentsAbandon(id) }
    suspend fun managedDeviceDesired(id: String): DesiredState = io { api.managedDevicesDesired(id) }
    suspend fun updateManagedDevicePolicy(id: String, body: PolicyUpdateInput): PolicyRevision =
        io { api.managedDevicesPolicy(id, body) }
    suspend fun createManagedRecovery(id: String, body: RecoveryIntentInput): RecoveryIntent =
        io { api.managedDevicesRecovery(id, body) }
    suspend fun quarantineManagedDevice(id: String, body: StateChangeInput): ManagedDevice =
        io { api.managedDevicesQuarantine(id, body) }
    suspend fun revokeManagedDevice(id: String, body: StateChangeInput): ManagedDevice =
        io { api.managedDevicesRevoke(id, body) }

    fun authHeader(token: String) = "Bearer $token"

    private suspend fun <T> io(block: () -> T): T = withContext(Dispatchers.IO) { block() }

    companion object {
        internal fun apiOrigin(raw: String): String {
            val trimmed = raw.trim().trimEnd('/')
            if (trimmed.isEmpty()) return "/api"
            val uri = runCatching { URI(trimmed) }.getOrNull()
            val path = uri?.rawPath?.trimEnd('/') ?: trimmed
            return when {
                path.endsWith("/tasks/api") || path.endsWith("/api") -> trimmed
                path.endsWith("/tasks") -> "$trimmed/api"
                else -> "$trimmed/api"
            }
        }
    }
}
