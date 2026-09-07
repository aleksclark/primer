package com.aleksclark.primer.control.tasks

import com.aleksclark.primertasks.client.CreateStudent
import com.aleksclark.primertasks.client.CredentialProvider
import com.aleksclark.primertasks.client.DecisionInput
import com.aleksclark.primertasks.client.OccurrencesListQuery
import com.aleksclark.primertasks.client.Requirement
import com.aleksclark.primertasks.client.ScheduleInput
import com.aleksclark.primertasks.client.SchedulesListQuery
import com.aleksclark.primertasks.client.StudentsListQuery
import com.aleksclark.primertasks.client.TaskInput
import com.aleksclark.primertasks.client.TasksClient
import com.aleksclark.primertasks.client.TasksListQuery
import com.aleksclark.primertasks.client.UpdateStudent
import kotlinx.serialization.json.JsonObject

class ParentTasksRepository(
    apiBase: String,
    token: CredentialProvider,
    httpClient: okhttp3.OkHttpClient = okhttp3.OkHttpClient(),
    private val client: TasksClient = TasksClient(
        baseUrl = apiBase,
        http = httpClient,
        parentCredentials = token,
    ),
) {
    suspend fun session() = client.parentSession()
    suspend fun logout() = client.logout()

    suspend fun listStudents(q: String, offset: Long, limit: Long = PAGE) =
        client.listStudents(StudentsListQuery(q = q.ifBlank { null }, limit = limit, offset = offset))
    suspend fun getStudent(id: String) = client.getStudent(id)
    suspend fun createStudent(name: String) = client.createStudent(CreateStudent(displayName = name))
    suspend fun updateStudent(id: String, name: String) = client.updateStudent(id, UpdateStudent(displayName = name))
    suspend fun archiveStudent(id: String) = client.archiveStudent(id)
    suspend fun issuePairing(id: String) = client.issuePairing(id)

    suspend fun listTasks(q: String, offset: Long, status: String, limit: Long = PAGE) =
        client.listTasks(
            TasksListQuery(
                q = q.ifBlank { null },
                limit = limit,
                offset = offset,
                status = status,
                view = "templates",
                sort = "title",
                dir = "asc",
            ),
        )
    suspend fun createTask(title: String, instructions: String) = client.createTask(
        TaskInput(title = title, instructions = instructions, requirements = listOf(parentApproval)),
    )
    suspend fun reviseTask(templateId: String, title: String, instructions: String, requirements: List<Requirement>?) =
        client.reviseTask(templateId, TaskInput(title = title, instructions = instructions, requirements = requirements ?: listOf(parentApproval)))
    suspend fun publishTask(id: String) = client.publishTask(id)
    suspend fun retireTask(templateId: String) = client.retireTask(templateId)

    suspend fun listSchedules(offset: Long, status: String, limit: Long = PAGE) =
        client.listSchedules(SchedulesListQuery(limit = limit, offset = offset, status = status.ifBlank { null }))
    suspend fun createSchedule(body: ScheduleInput) = client.createSchedule(body)
    suspend fun updateSchedule(id: String, body: ScheduleInput) = client.updateSchedule(id, body)
    suspend fun retireSchedule(id: String) = client.retireSchedule(id)

    suspend fun listOccurrences(status: String, dir: String, offset: Long = 0, limit: Long = 50) =
        client.listOccurrences(OccurrencesListQuery(limit = limit, offset = offset, status = status.ifBlank { null }, dir = dir, sort = "nominalAt"))
    suspend fun getOccurrence(id: String) = client.getOccurrence(id)
    suspend fun decide(id: String, accepted: Boolean, reason: String) =
        client.decideOccurrence(id, DecisionInput(accepted = accepted, reason = reason.trim()))
    suspend fun retry(id: String) = client.retryOccurrence(id)
    suspend fun skip(id: String) = client.skipOccurrence(id)
    suspend fun cancel(id: String) = client.cancelOccurrence(id)
    suspend fun downloadReleaseArtifact(id: String, sink: java.io.OutputStream): Long =
        client.downloadManagedReleaseArtifact(id, sink)

    companion object {
        const val PAGE = 20L
        val parentApproval = Requirement(
            id = "parent-approval",
            kind = "parent_approval",
            configVersion = 1,
            config = JsonObject(emptyMap()),
            interaction = "parent_action",
            executor = "human",
        )
    }
}
