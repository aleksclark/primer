package com.aleksclark.primer.security

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import com.google.crypto.tink.Aead
import com.google.crypto.tink.KeysetHandle
import com.google.crypto.tink.TinkJsonProtoKeysetFormat
import com.google.crypto.tink.aead.AeadConfig
import com.google.crypto.tink.integration.android.AndroidKeystoreKmsClient
import java.security.KeyStore
import javax.crypto.KeyGenerator

/**
 * Persist a Tink private keyset with serializeEncryptedKeyset + Android Keystore AEAD.
 * Fail closed if Keystore is unusable. Never AndroidKeysetManager.
 */
class AndroidKeystoreKeyset(
    private val wrappingAlias: String = DEFAULT_ALIAS,
) {
    init {
        AeadConfig.register()
    }

    fun wrap(privateHandle: KeysetHandle): ByteArray {
        val aead = wrappingAead()
        return TinkJsonProtoKeysetFormat.serializeEncryptedKeyset(privateHandle, aead, EMPTY_AAD)
            .toByteArray(Charsets.UTF_8)
    }

    fun unwrap(encryptedJsonUtf8: ByteArray): KeysetHandle {
        val aead = wrappingAead()
        return TinkJsonProtoKeysetFormat.parseEncryptedKeyset(
            String(encryptedJsonUtf8, Charsets.UTF_8),
            aead,
            EMPTY_AAD,
        )
    }

    private fun wrappingAead(): Aead {
        ensureWrappingKey()
        val client = AndroidKeystoreKmsClient()
        check(client.doesSupport(keyUri())) { "Android Keystore wrapping key is unavailable" }
        return runCatching { client.getAead(keyUri()) }.getOrElse {
            error("Android Keystore AEAD is unusable; recovery encryption is fail-closed")
        }
    }

    private fun ensureWrappingKey() {
        val store = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        if (store.containsAlias(wrappingAlias)) return
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(
                wrappingAlias,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .setUserAuthenticationRequired(false)
                .build(),
        )
        generator.generateKey()
        check(store.apply { load(null) }.containsAlias(wrappingAlias)) {
            "Android Keystore wrapping key could not be created"
        }
    }

    private fun keyUri(): String = "android-keystore://$wrappingAlias"

    companion object {
        const val DEFAULT_ALIAS = "primer_management_hpke_wrap"
        private const val ANDROID_KEYSTORE = "AndroidKeyStore"
        private val EMPTY_AAD = ByteArray(0)
    }
}
