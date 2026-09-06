package com.aleksclark.primertasks.client

import com.aleksclark.primertasks.generated.AuthKind
import com.aleksclark.primertasks.generated.GeneratedTasksApi
import java.io.ByteArrayOutputStream
import java.io.OutputStream
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
    maxBinaryBytes: Long = DEFAULT_MAX_BINARY_BYTES,
    maxInMemoryBinaryBytes: Long = DEFAULT_MAX_IN_MEMORY_BINARY_BYTES,
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
        maxBinaryBytes = maxBinaryBytes,
    )
    private val inMemoryCap = maxInMemoryBinaryBytes

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

    suspend fun listManagedReleases(): ReleasePage = io { api.managedReleasesList() }
    suspend fun getManagedRelease(id: String): Release = io { api.managedReleasesGet(id) }
    suspend fun setManagedDeviceRelease(id: String, body: ReleaseTargetInput): ReleaseTarget =
        io { api.managedDevicesReleaseTarget(id, body) }
    suspend fun managementDeviceReleaseReceipt(body: ReleaseReceiptInput): ReleaseReceipt =
        io { api.managementDeviceReleaseReceipt(body) }
    suspend fun managementDeviceArtifact(id: String, sink: OutputStream): Long =
        io { api.managementDeviceArtifact(id, sink) }
    suspend fun downloadManagedReleaseArtifact(id: String, sink: OutputStream): Long =
        io { api.managedReleasesApk(id, sink) }
    suspend fun managementDeviceArtifact(id: String): ByteArray = io {
        val out = ByteArrayOutputStream()
        val written = api.managementDeviceArtifact(id, out)
        check(written <= inMemoryCap) { "in-memory artifact cap exceeded" }
        out.toByteArray()
    }
    suspend fun downloadManagedReleaseArtifact(id: String): ByteArray = io {
        val out = ByteArrayOutputStream()
        val written = api.managedReleasesApk(id, out)
        check(written <= inMemoryCap) { "in-memory artifact cap exceeded" }
        out.toByteArray()
    }

    fun authHeader(token: String) = "Bearer $token"

    private suspend fun <T> io(block: () -> T): T = withContext(Dispatchers.IO) { block() }

    companion object {
        const val DEFAULT_MAX_BINARY_BYTES: Long = 200L * 1024L * 1024L
        const val DEFAULT_MAX_IN_MEMORY_BINARY_BYTES: Long = 1024L * 1024L

        internal fun apiOrigin(raw: String): String {
            val trimmed = raw.trim()
            require(trimmed.isNotEmpty()) { "Tasks API origin is required" }
            val uri = runCatching { URI(trimmed) }.getOrElse {
                throw IllegalArgumentException("Tasks API origin is not an absolute URL")
            }
            val scheme = uri.scheme?.lowercase()
            require(scheme == "https" || scheme == "http") { "Tasks API origin must be http or https" }
            require(!uri.host.isNullOrBlank()) { "Tasks API origin must include a host" }
            require(uri.userInfo == null) { "Tasks API origin must not include userinfo" }
            require(uri.rawQuery.isNullOrEmpty() && uri.query == null) { "Tasks API origin must not include a query" }
            require(uri.fragment == null) { "Tasks API origin must not include a fragment" }
            val path = (uri.rawPath ?: "").trimEnd('/')
            require(path.isEmpty() || path == "/tasks" || path == "/api" || path == "/tasks/api") {
                "Tasks API origin path must be empty, /tasks, /api, or /tasks/api"
            }
            val origin = buildString {
                append(scheme).append("://").append(uri.host)
                if (uri.port > 0) append(':').append(uri.port)
            }
            return when (path) {
                "/tasks/api" -> "$origin/tasks/api"
                "/tasks" -> "$origin/tasks/api"
                "/api" -> "$origin/api"
                else -> "$origin/api"
            }
        }
    }
}
