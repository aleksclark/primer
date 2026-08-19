package com.aleksclark.primertasks

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first

internal val Context.studentMetadataDataStore by preferencesDataStore("student_metadata")

data class StudentMetadata(
    val studentId: String,
    val displayName: String,
    val origin: String,
    val pairingId: String,
)

class MetadataStore(private val context: Context) {
    private val studentId = stringPreferencesKey("student_id")
    private val displayName = stringPreferencesKey("display_name")
    private val origin = stringPreferencesKey("origin")
    private val pairingId = stringPreferencesKey("pairing_id")

    suspend fun save(metadata: StudentMetadata) {
        context.studentMetadataDataStore.edit {
            it[studentId] = metadata.studentId
            it[displayName] = metadata.displayName
            it[origin] = metadata.origin
            it[pairingId] = metadata.pairingId
        }
    }

    suspend fun read(): StudentMetadata? {
        val values = context.studentMetadataDataStore.data.first()
        val id = values[studentId] ?: return null
        return StudentMetadata(
            studentId = id,
            displayName = values[displayName].orEmpty(),
            origin = values[origin].orEmpty(),
            pairingId = values[pairingId].orEmpty(),
        )
    }

    suspend fun clear() {
        context.studentMetadataDataStore.edit {
            it.remove(studentId)
            it.remove(displayName)
            it.remove(origin)
            it.remove(pairingId)
        }
    }
}
