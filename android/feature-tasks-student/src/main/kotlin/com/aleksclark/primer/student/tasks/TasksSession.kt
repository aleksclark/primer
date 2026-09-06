package com.aleksclark.primer.student.tasks

import android.content.Context
import android.net.Uri
import com.aleksclark.primertasks.client.ChecklistItem
import com.aleksclark.primertasks.client.OccurrenceResponse
import com.aleksclark.primertasks.client.TasksClient
import com.aleksclark.primertasks.client.TasksHttpException

/**
 * Tasks pairing/session state. Failures and revocation clear only Tasks
 * DataStore/Keystore material. Device-owner recovery is owned by StudentRuntime.
 */
class TasksSession(
    context: Context,
    private val tokenStore: EncryptedTokenStore = EncryptedTokenStore(context),
    private val metadataStore: MetadataStore = MetadataStore(context),
    private val clientFactory: (String) -> TasksClient = { TasksClient(it) },
    private val configuredHttpsOrigin: String = BuildConfig.CONFIGURED_API_ORIGIN,
    private val allowEmulatorOrigin: Boolean = BuildConfig.DEBUG,
) {
    suspend fun restore(): TasksRestoreResult {
        val savedToken = tokenStore.read()
        val savedMetadata = metadataStore.read()
        if (savedToken == null || savedMetadata == null || savedMetadata.origin.isBlank()) {
            tokenStore.clear()
            metadataStore.clear()
            return TasksRestoreResult.Unpaired()
        }
        return load(savedToken, savedMetadata)
    }

    suspend fun pair(rawQr: String): TasksRestoreResult {
        val qr = PairingQrParser.parse(rawQr)
        val origin = qr?.let {
            ServerOriginPolicy.allowedOrigin(
                origin = it.origin,
                configuredHttpsOrigin = configuredHttpsOrigin,
                allowEmulatorOrigin = allowEmulatorOrigin,
            )
        }
        if (qr == null || origin == null) {
            return TasksRestoreResult.Unpaired("That QR code is not a valid Primer pairing code. Choose a Primer pairing QR.")
        }
        return try {
            val client = clientFactory(origin)
            val paired = client.pairDevice(qr.code)
            val profile = client.studentProfile(paired.token)
            val metadata = StudentMetadata(paired.studentId, profile.displayName, origin, qr.pairingId)
            tokenStore.save(paired.token)
            metadataStore.save(metadata)
            load(paired.token, metadata)
        } catch (error: TasksHttpException) {
            TasksRestoreResult.Unpaired(
                if (error.statusCode == 401 || error.statusCode == 403) {
                    "That pairing QR is expired or has already been used. Request a new QR code."
                } else {
                    "Pairing failed. Check the server and try again."
                },
            )
        } catch (_: Exception) {
            TasksRestoreResult.Unpaired("Unable to reach the Primer server. Check your connection and try again.")
        }
    }

    suspend fun importImage(context: Context, uri: Uri): TasksRestoreResult {
        return when (val result = QrImageImporter(context.contentResolver).decode(uri)) {
            is QrImageImporter.Result.Decoded -> pair(result.payload)
            is QrImageImporter.Result.Failure -> TasksRestoreResult.Unpaired(
                when (result.reason) {
                    QrImageImporter.Failure.UNREADABLE -> "Couldn't read that image. Choose another QR image."
                    QrImageImporter.Failure.TOO_LARGE -> "That image is too large to scan safely. Choose a smaller QR image."
                    QrImageImporter.Failure.NO_QR -> "No valid Primer pairing QR code was found in that image."
                },
            )
        }
    }

    suspend fun loadOccurrence(token: String, origin: String, id: String): OccurrenceLookup {
        return try {
            OccurrenceLookup.Found(clientFactory(origin).studentOccurrence(token, id))
        } catch (_: TasksHttpException) {
            OccurrenceLookup.Unavailable
        } catch (_: Exception) {
            OccurrenceLookup.Unavailable
        }
    }

    suspend fun start(token: String, origin: String, id: String): OccurrenceActionResult = mutate(token, origin, id) { client ->
        client.startStudentOccurrence(token, id)
    }

    suspend fun submit(token: String, origin: String, id: String): OccurrenceActionResult = mutate(token, origin, id) { client ->
        client.submitStudentOccurrence(token, id)
    }

    suspend fun clearPairing() {
        tokenStore.clear()
        metadataStore.clear()
    }

    private suspend fun load(token: String, metadata: StudentMetadata): TasksRestoreResult {
        return try {
            val client = clientFactory(metadata.origin)
            val profile = client.studentProfile(token)
            TasksRestoreResult.Paired(
                token = token,
                metadata = metadata.copy(displayName = profile.displayName, studentId = profile.id),
                checklist = client.studentChecklist(token).items,
                today = client.studentToday(token).items,
                upcoming = client.studentUpcoming(token).items,
            )
        } catch (error: TasksHttpException) {
            if (error.statusCode == 401 || error.statusCode == 403) {
                clearPairing()
                TasksRestoreResult.Unpaired("This device pairing is no longer active. Scan a new QR code.")
            } else {
                TasksRestoreResult.Unpaired("Unable to load the checklist. Try again.", token, metadata)
            }
        } catch (_: Exception) {
            TasksRestoreResult.Unpaired("Unable to reach the Primer server. Try again.", token, metadata)
        }
    }

    private suspend fun mutate(
        token: String,
        origin: String,
        id: String,
        write: suspend (TasksClient) -> Unit,
    ): OccurrenceActionResult {
        val client = clientFactory(origin)
        return try {
            write(client)
            OccurrenceActionResult.Updated(client.studentOccurrence(token, id))
        } catch (error: TasksHttpException) {
            OccurrenceActionResult.Failed(
                if (error.statusCode == 403) "This task is not available to this student." else "Unable to update this task.",
            )
        } catch (_: Exception) {
            OccurrenceActionResult.Failed("Unable to update this task.")
        }
    }
}

sealed interface TasksRestoreResult {
    data class Paired(
        val token: String,
        val metadata: StudentMetadata,
        val checklist: List<ChecklistItem>,
        val today: List<OccurrenceResponse>,
        val upcoming: List<OccurrenceResponse>,
        val message: String? = null,
    ) : TasksRestoreResult

    data class Unpaired(
        val message: String? = null,
        val retainedToken: String? = null,
        val retainedMetadata: StudentMetadata? = null,
    ) : TasksRestoreResult
}

sealed interface OccurrenceLookup {
    data class Found(val occurrence: OccurrenceResponse) : OccurrenceLookup
    data object Unavailable : OccurrenceLookup
}

sealed interface OccurrenceActionResult {
    data class Updated(val occurrence: OccurrenceResponse) : OccurrenceActionResult
    data class Failed(val message: String) : OccurrenceActionResult
}
