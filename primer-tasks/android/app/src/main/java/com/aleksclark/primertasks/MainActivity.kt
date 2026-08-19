package com.aleksclark.primertasks

import android.os.Bundle
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.spec.GCMParameterSpec
import android.util.Base64
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

private class SecureTokenStore {
    private val alias = "primer_tasks_student_device"
    private val prefs = ApplicationProviderHolder.prefs
    private fun key() = (KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.getKey(alias,null) ?: run { val g=KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES,"AndroidKeyStore");g.init(KeyGenParameterSpec.Builder(alias,KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT).setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).setUserAuthenticationRequired(false).build());g.generateKey() })
    fun save(token:String){val c=Cipher.getInstance("AES/GCM/NoPadding");c.init(Cipher.ENCRYPT_MODE,key());prefs.edit().putString("token",Base64.encodeToString(c.iv+ c.doFinal(token.toByteArray()),Base64.NO_WRAP)).apply()}
    fun hasToken()=prefs.contains("token")
    fun exchange(code:String): Boolean { return try { val body=JSONObject().put("code",code).toString().toRequestBody("application/json".toMediaType()); val req=Request.Builder().url("http://10.0.2.2:8080/api/device/pair").post(body).build(); OkHttpClient().newCall(req).execute().use { response -> if(!response.isSuccessful)return false; val token=JSONObject(response.body!!.string()).getString("token"); save(token); true } } catch(_:Exception){false} }
}
private object ApplicationProviderHolder { lateinit var prefs: android.content.SharedPreferences }

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) { super.onCreate(savedInstanceState); ApplicationProviderHolder.prefs=getSharedPreferences("student_metadata",MODE_PRIVATE); setContent { PrimerTasksApp() } }
}

@Composable private fun PrimerTasksApp(){
    var paired by remember { mutableStateOf(ApplicationProviderHolder.prefs.contains("token")) }
    var code by remember { mutableStateOf("") }
    var message by remember { mutableStateOf("") }
    MaterialTheme(colorScheme=darkColorScheme(primary=androidx.compose.ui.graphics.Color(0xFF3DE0F0))){
        Surface(Modifier.fillMaxSize()) { Column(Modifier.padding(24.dp), verticalArrangement=Arrangement.spacedBy(16.dp)) {
            Text("PRIMER TASKS", style=MaterialTheme.typography.labelLarge); Text(if(paired) "Student checklist" else "Pair this device", style=MaterialTheme.typography.headlineMedium)
            if(paired){ Text("Bound student", style=MaterialTheme.typography.titleLarge); Text("Nothing assigned yet. Your checklist is empty."); Text("One student · one device", style=MaterialTheme.typography.bodySmall) }
            else { Text("Scan the QR shown by your parent, or enter its one-use code."); OutlinedTextField(code,{code=it.uppercase()},label={Text("Pairing code")},singleLine=true); Button(enabled=code.isNotBlank(),onClick={ if(code.length>=4){ if(SecureTokenStore().exchange(code)){ paired=true; message="Paired successfully" } else message="Pairing failed or code expired" }}){Text("Pair device")}; if(message.isNotBlank()) Text(message) }
        } }
    }
}
