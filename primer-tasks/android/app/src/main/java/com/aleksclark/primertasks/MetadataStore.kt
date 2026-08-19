package com.aleksclark.primertasks

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first

private val Context.studentMetadata by preferencesDataStore("student_metadata")
class MetadataStore(private val context: Context) {
    private val studentId = stringPreferencesKey("student_id")
    suspend fun saveStudent(id: String) { context.studentMetadata.edit { it[studentId] = id } }
    suspend fun readStudent(): String? = context.studentMetadata.data.first()[studentId]
}
