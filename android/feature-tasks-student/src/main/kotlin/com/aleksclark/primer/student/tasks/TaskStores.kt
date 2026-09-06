package com.aleksclark.primer.student.tasks

/** Tasks bearer persistence. Failures here must never touch device-owner recovery. */
interface TokenStore {
    suspend fun save(token: String)
    suspend fun read(): String?
    suspend fun clear()
}

interface BindingStore {
    suspend fun save(metadata: StudentMetadata)
    suspend fun read(): StudentMetadata?
    suspend fun clear()
}

internal class EncryptedTokenStoreAdapter(private val store: EncryptedTokenStore) : TokenStore {
    override suspend fun save(token: String) = store.save(token)
    override suspend fun read(): String? = store.read()
    override suspend fun clear() = store.clear()
}

internal class MetadataStoreAdapter(private val store: MetadataStore) : BindingStore {
    override suspend fun save(metadata: StudentMetadata) = store.save(metadata)
    override suspend fun read(): StudentMetadata? = store.read()
    override suspend fun clear() = store.clear()
}

data class TasksNavState(
    val showTasks: Boolean = false,
    val pendingOccurrenceLink: String? = null,
)

object TasksDeepLinkRouting {
    fun incoming(state: TasksNavState, uri: android.net.Uri?): TasksNavState {
        val id = occurrenceIdFromDeepLink(uri) ?: return state
        val serialized = uri.toString()
        if (serialized == state.pendingOccurrenceLink && state.showTasks) return state
        return TasksNavState(showTasks = true, pendingOccurrenceLink = serialized)
    }

    fun leave(state: TasksNavState): TasksNavState =
        TasksNavState(showTasks = false, pendingOccurrenceLink = null)

    fun consumed(state: TasksNavState): TasksNavState =
        state.copy(pendingOccurrenceLink = null)
}
