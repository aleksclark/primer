package com.aleksclark.primer.student.tasks

import android.content.Context
import android.net.Uri
import com.aleksclark.primertasks.client.ChecklistItem
import com.aleksclark.primertasks.client.OccurrenceResponse
import com.aleksclark.primertasks.client.TasksClient
import com.aleksclark.primertasks.client.TasksHttpException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Tasks pairing/session state. Failures and revocation clear only Tasks
 * DataStore/Keystore material. Device-owner recovery is owned by StudentRuntime.
 */
class TasksSession(
    private val tokenStore: TokenStore,
    private val bindingStore: BindingStore,
    private val clientFactory: (String) -> TasksClient,
    private val configuredHttpsOrigin: String,
    private val allowEmulatorOrigin: Boolean,
) {
    constructor(
        context: Context,
        configuredHttpsOrigin: String = BuildConfig.CONFIGURED_API_ORIGIN,
        allowEmulatorOrigin: Boolean = BuildConfig.DEBUG,
        clientFactory: (String) -> TasksClient = { TasksClient(it) },
    ) : this(
        tokenStore = EncryptedTokenStoreAdapter(EncryptedTokenStore(context)),
        bindingStore = MetadataStoreAdapter(MetadataStore(context)),
        clientFactory = clientFactory,
        configuredHttpsOrigin = configuredHttpsOrigin,
        allowEmulatorOrigin = allowEmulatorOrigin,
    )

    private val pairingMutex = Mutex()

    suspend fun restore(): TasksRestoreResult {
        val snapshot = pairingMutex.withLock {
            val savedToken = tokenStore.read()
            val savedMetadata = bindingStore.read()
            if (savedToken == null || savedMetadata == null || savedMetadata.origin.isBlank()) {
                // Snapshot and incomplete-state cleanup share the publication
                // lock. Never clear a newer pairing using an older null read.
                tokenStore.clear()
                bindingStore.clear()
                null
            } else savedToken to savedMetadata
        } ?: return TasksRestoreResult.Unpaired()
        // No network call is made under the storage mutex.
        return load(snapshot.first, snapshot.second)
    }

    suspend fun pair(rawQr: String): TasksRestoreResult {
        val qr = PairingQrParser.parse(rawQr)
        val apiBase = qr?.let {
            ServerOriginPolicy.allowedApiBase(
                origin = it.origin,
                api = it.api,
                configuredHttpsOrigin = configuredHttpsOrigin,
                allowEmulatorOrigin = allowEmulatorOrigin,
                version = it.v ?: 1,
            )
        }
        if (qr == null || apiBase == null) {
            return TasksRestoreResult.Unpaired("That QR code is not a valid Primer pairing code. Choose a Primer pairing QR.")
        }
        return try {
            val client = clientFactory(apiBase)
            val paired = client.pairDevice(qr.code)
            val binding = StudentMetadata(paired.studentId, "", apiBase, qr.pairingId)
            // Persist the one-use credential before any follow-up call so a lost
            // profile/checklist response cannot discard the newly issued bearer.
            persistPairing(paired.token, binding)
            load(paired.token, binding)
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: TasksHttpException) {
            TasksRestoreResult.Unpaired(
                if (error.statusCode == 401 || error.statusCode == 403 || error.statusCode == 410) {
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
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: TasksHttpException) {
            when (error.statusCode) {
                401, 403 -> revokeIfCurrent(token)
                404 -> OccurrenceLookup.Unavailable
                else -> OccurrenceLookup.Failed("Unable to refresh this task.")
            }
        } catch (_: Exception) {
            OccurrenceLookup.Failed("Unable to reach the Primer server. Try again.")
        }
    }

    suspend fun start(token: String, origin: String, id: String): OccurrenceActionResult =
        mutate(token, origin, id) { client -> client.startStudentOccurrence(token, id) }

    suspend fun submit(token: String, origin: String, id: String): OccurrenceActionResult =
        mutate(token, origin, id) { client -> client.submitStudentOccurrence(token, id) }

    internal suspend fun persistPairing(token: String, binding: StudentMetadata) = pairingMutex.withLock {
        tokenStore.save(token)
        bindingStore.save(binding)
    }

    suspend fun clearPairing() = pairingMutex.withLock {
        tokenStore.clear()
        bindingStore.clear()
    }

    private suspend fun ifCurrentBinding(
        token: String,
        metadata: StudentMetadata,
        block: suspend () -> TasksRestoreResult,
    ): TasksRestoreResult = pairingMutex.withLock {
        val current = bindingStore.read()
        if (tokenStore.read() != token || current?.studentId != metadata.studentId ||
            current.origin != metadata.origin || current.pairingId != metadata.pairingId
        ) TasksRestoreResult.Superseded else block()
    }

    private suspend fun load(token: String, metadata: StudentMetadata): TasksRestoreResult {
        return try {
            val client = clientFactory(metadata.origin)
            val profile = client.studentProfile(token)
            if (profile.id != metadata.studentId) {
                return ifCurrentBinding(token, metadata) {
                    TasksRestoreResult.Unpaired(
                        message = "This pairing belongs to a different student. Request a new QR code.",
                        retainedToken = token,
                        retainedMetadata = metadata,
                    )
                }
            }
            val bound = metadata.copy(displayName = profile.displayName, studentId = metadata.studentId)
            val checklist = client.studentChecklist(token).items
            val today = client.studentToday(token).items
            val upcoming = client.studentUpcoming(token).items
            ifCurrentBinding(token, metadata) {
                bindingStore.save(bound)
                TasksRestoreResult.Paired(token, bound, checklist, today, upcoming)
            }
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: TasksHttpException) {
            if (error.statusCode == 401 || error.statusCode == 403) {
                if (clearIfCurrent(token)) TasksRestoreResult.Unpaired("This device pairing is no longer active. Scan a new QR code.")
                else TasksRestoreResult.Superseded
            } else {
                ifCurrentBinding(token, metadata) {
                    TasksRestoreResult.Unavailable(token, metadata, "Unable to load the checklist. Try again.")
                }
            }
        } catch (_: Exception) {
            ifCurrentBinding(token, metadata) {
                TasksRestoreResult.Unavailable(token, metadata, "Unable to reach the Primer server. Try again.")
            }
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
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: TasksHttpException) {
            when (error.statusCode) {
                401, 403 -> {
                    clearIfCurrent(token)
                    OccurrenceActionResult.Revoked("This device pairing is no longer active. Scan a new QR code.")
                }
                409 -> {
                    val refreshed = runCatching { client.studentOccurrence(token, id) }.getOrNull()
                    if (error.code == "unsupported_task") {
                        if (refreshed != null) {
                            OccurrenceActionResult.Conflict(
                                refreshed,
                                StudentOccurrenceCopy.UNSUPPORTED,
                            )
                        } else {
                            OccurrenceActionResult.Failed(StudentOccurrenceCopy.UNSUPPORTED)
                        }
                    } else if (refreshed != null) {
                        OccurrenceActionResult.Conflict(refreshed, "This task changed. Showing the server state.")
                    } else {
                        OccurrenceActionResult.Failed("This task changed. Refresh and try again.")
                    }
                }
                404 -> OccurrenceActionResult.Failed("This task is not available to this student.")
                else -> OccurrenceActionResult.Failed("Unable to update this task.")
            }
        } catch (_: Exception) {
            OccurrenceActionResult.Failed("Unable to reach the Primer server. Try again.")
        }
    }

    private suspend fun revokeIfCurrent(token: String): OccurrenceLookup {
        clearIfCurrent(token)
        return OccurrenceLookup.Revoked("This device pairing is no longer active. Scan a new QR code.")
    }

    private suspend fun clearIfCurrent(expectedToken: String): Boolean = pairingMutex.withLock {
        if (tokenStore.read() != expectedToken) false else {
            tokenStore.clear()
            bindingStore.clear()
            true
        }
    }
}

sealed interface TasksRestoreResult {
    /** A newer revocation or pairing owns the screen; never publish old custody. */
    data object Superseded : TasksRestoreResult

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

    data class Unavailable(
        val token: String,
        val metadata: StudentMetadata,
        val message: String,
    ) : TasksRestoreResult
}

sealed interface OccurrenceLookup {
    data class Found(val occurrence: OccurrenceResponse) : OccurrenceLookup
    data object Unavailable : OccurrenceLookup
    data class Failed(val message: String) : OccurrenceLookup
    data class Revoked(val message: String) : OccurrenceLookup
}

sealed interface OccurrenceActionResult {
    data class Updated(val occurrence: OccurrenceResponse) : OccurrenceActionResult
    data class Failed(val message: String) : OccurrenceActionResult
    data class Conflict(val occurrence: OccurrenceResponse, val message: String) : OccurrenceActionResult
    data class Revoked(val message: String) : OccurrenceActionResult
}
