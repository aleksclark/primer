package com.aleksclark.primer.student.management

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.aleksclark.primer.security.AndroidKeystoreKeyset
import com.aleksclark.primer.security.RecoveryHpke
import com.google.crypto.tink.KeysetHandle
import kotlinx.coroutines.flow.first
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.spec.GCMParameterSpec

internal val Context.managementDataStore by preferencesDataStore("student_management")

data class ManagementBinding(
    val token: String,
    val origin: String,
    val deviceId: String,
    val keyId: String,
    val replace: Boolean = false,
)

interface ManagementSecrets {
    suspend fun save(binding: ManagementBinding)
    suspend fun read(): ManagementBinding?
    suspend fun publicEnrollment(): Pair<String, String>
    suspend fun privateHandle(): KeysetHandle?
    suspend fun clearTokenOnly()
    suspend fun stableDeviceKey(): String
    suspend fun expectedToken(): String?
}

class ManagementCredentialStore(private val context: Context) : ManagementSecrets {
    private val tokenKey = stringPreferencesKey("encrypted_bearer")
    private val originKey = stringPreferencesKey("origin")
    private val deviceIdKey = stringPreferencesKey("device_id")
    private val keyIdKey = stringPreferencesKey("key_id")
    private val privateKeysetKey = stringPreferencesKey("encrypted_private_keyset")
    private val stableKey = stringPreferencesKey("stable_device_key")
    private val wrapping = AndroidKeystoreKeyset()

    override suspend fun save(binding: ManagementBinding) {
        context.managementDataStore.edit {
            it[tokenKey] = encrypt(binding.token)
            it[originKey] = binding.origin
            it[deviceIdKey] = binding.deviceId
            it[keyIdKey] = binding.keyId
        }
    }

    override suspend fun read(): ManagementBinding? {
        val values = context.managementDataStore.data.first()
        val encoded = values[tokenKey] ?: return null
        val token = runCatching { decrypt(encoded) }.getOrNull() ?: return null
        val origin = values[originKey].orEmpty()
        val deviceId = values[deviceIdKey].orEmpty()
        val keyId = values[keyIdKey].orEmpty()
        if (origin.isBlank() || deviceId.isBlank() || keyId.isBlank()) return null
        return ManagementBinding(token, origin, deviceId, keyId)
    }

    suspend fun savePrivateHandle(handle: KeysetHandle) {
        val wrapped = wrapping.wrap(handle)
        context.managementDataStore.edit {
            it[privateKeysetKey] = Base64.encodeToString(wrapped, Base64.NO_WRAP)
        }
    }

    override suspend fun privateHandle(): KeysetHandle? {
        val encoded = context.managementDataStore.data.first()[privateKeysetKey] ?: return null
        val wrapped = Base64.decode(encoded, Base64.NO_WRAP)
        return wrapping.unwrap(wrapped)
    }

    override suspend fun publicEnrollment(): Pair<String, String> {
        val existing = privateHandle()
        val handle = existing ?: RecoveryHpke.generatePrivateHandle().also { savePrivateHandle(it) }
        val json = RecoveryHpke.publicKeysetJson(handle)
        return RecoveryHpke.encodePublicEnrollmentKey(json) to RecoveryHpke.keyId(json)
    }

    override suspend fun clearTokenOnly() {
        context.managementDataStore.edit {
            it.remove(tokenKey)
        }
    }

    override suspend fun expectedToken(): String? = read()?.token

    override suspend fun stableDeviceKey(): String {
        val existing = context.managementDataStore.data.first()[stableKey]
        if (!existing.isNullOrBlank()) return existing
        val generated = java.util.UUID.randomUUID().toString()
        context.managementDataStore.edit { it[stableKey] = generated }
        return generated
    }

    private fun encrypt(value: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())
        val encrypted = cipher.iv + cipher.doFinal(value.toByteArray(Charsets.UTF_8))
        return Base64.encodeToString(encrypted, Base64.NO_WRAP)
    }

    private fun decrypt(encoded: String): String {
        val encrypted = Base64.decode(encoded, Base64.NO_WRAP)
        require(encrypted.size > GCM_IV_BYTES)
        val iv = encrypted.copyOfRange(0, GCM_IV_BYTES)
        val ciphertext = encrypted.copyOfRange(GCM_IV_BYTES, encrypted.size)
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(GCM_TAG_BITS, iv))
        return cipher.doFinal(ciphertext).toString(Charsets.UTF_8)
    }

    private fun key() = synchronized(LOCK) {
        KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }.getKey(KEY_ALIAS, null)
            ?: KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE).run {
                init(
                    KeyGenParameterSpec.Builder(
                        KEY_ALIAS,
                        KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                    )
                        .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                        .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                        .setUserAuthenticationRequired(false)
                        .build(),
                )
                generateKey()
            }
    }

    private companion object {
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val KEY_ALIAS = "primer_student_management_device"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val GCM_IV_BYTES = 12
        const val GCM_TAG_BITS = 128
        val LOCK = Any()
    }
}
