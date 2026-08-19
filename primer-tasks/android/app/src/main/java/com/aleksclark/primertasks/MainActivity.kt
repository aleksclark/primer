package com.aleksclark.primertasks

import android.Manifest
import android.content.pm.PackageManager
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import com.aleksclark.primertasks.client.ChecklistItem
import com.aleksclark.primertasks.client.TasksClient
import com.aleksclark.primertasks.client.TasksHttpException
import kotlinx.coroutines.launch
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { PrimerTasksApp(applicationContext) }
    }
}

@Composable
private fun PrimerTasksApp(context: android.content.Context) {
    val tokenStore = remember { EncryptedTokenStore(context) }
    val metadataStore = remember { MetadataStore(context) }
    val scope = rememberCoroutineScope()
    var token by remember { mutableStateOf<String?>(null) }
    var metadata by remember { mutableStateOf<StudentMetadata?>(null) }
    var checklist by remember { mutableStateOf<List<ChecklistItem>>(emptyList()) }
    var busy by remember { mutableStateOf(true) }
    var scanning by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf<String?>(null) }

    fun clearPairing() {
        scope.launch {
            tokenStore.clear()
            metadataStore.clear()
            token = null
            metadata = null
            checklist = emptyList()
            scanning = false
        }
    }

    suspend fun loadSession(savedToken: String, savedMetadata: StudentMetadata) {
        try {
            val client = TasksClient(savedMetadata.origin)
            val profile = client.studentProfile(savedToken)
            checklist = client.studentChecklist(savedToken).items
            metadata = savedMetadata.copy(displayName = profile.displayName, studentId = profile.id)
            busy = false
        } catch (error: TasksHttpException) {
            if (error.statusCode == 401 || error.statusCode == 403) clearPairing()
            message = if (error.statusCode == 401 || error.statusCode == 403) {
                "This device pairing is no longer active. Scan a new QR code."
            } else {
                "Unable to load the checklist. Try again."
            }
            busy = false
        } catch (_: Exception) {
            message = "Unable to reach the Primer server. Try again."
            busy = false
        }
    }

    LaunchedEffect(Unit) {
        val savedToken = tokenStore.read()
        val savedMetadata = metadataStore.read()
        if (savedToken != null && savedMetadata != null && savedMetadata.origin.isNotBlank()) {
            token = savedToken
            metadata = savedMetadata
            loadSession(savedToken, savedMetadata)
        } else {
            tokenStore.clear()
            metadataStore.clear()
            busy = false
        }
    }

    fun pair(raw: String) {
        scanning = false
        val qr = PairingQrParser.parse(raw)
        val origin = qr?.let {
            ServerOriginPolicy.allowedOrigin(
                origin = it.origin,
                configuredHttpsOrigin = BuildConfig.CONFIGURED_API_ORIGIN,
                allowEmulatorOrigin = BuildConfig.DEBUG,
            )
        }
        if (qr == null || origin == null) {
            message = "That QR code is not a valid Primer pairing code."
            return
        }
        scope.launch {
            busy = true
            scanning = false
            message = null
            try {
                val client = TasksClient(origin)
                val paired = client.pairDevice(qr.code)
                val profile = client.studentProfile(paired.token)
                checklist = client.studentChecklist(paired.token).items
                tokenStore.save(paired.token)
                val savedMetadata = StudentMetadata(paired.studentId, profile.displayName, origin, qr.pairingId)
                metadataStore.save(savedMetadata)
                token = paired.token
                metadata = savedMetadata
                busy = false
            } catch (error: TasksHttpException) {
                busy = false
                message = if (error.statusCode == 401 || error.statusCode == 403) {
                    "This pairing is not authorized. Scan a new QR code."
                } else {
                    "Pairing failed or the QR code has expired."
                }
            } catch (_: Exception) {
                busy = false
                message = "Unable to reach the Primer server. Try again."
            }
        }
    }

    MaterialTheme(colorScheme = androidx.compose.material3.darkColorScheme(primary = androidx.compose.ui.graphics.Color(0xFF3DE0F0))) {
        Surface(Modifier.fillMaxSize()) {
            when {
                busy && metadata == null -> LoadingScreen()
                metadata != null && token != null -> ChecklistScreen(metadata!!.displayName, checklist, message)
                scanning -> PairingScanner(onQr = ::pair, onCancel = { scanning = false })
                else -> PairingScreen(message, onScan = { message = null; scanning = true })
            }
        }
    }
}

@Composable
private fun LoadingScreen() {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center) {
        CircularProgressIndicator()
    }
}

@Composable
private fun PairingScreen(message: String?, onScan: () -> Unit) {
    Column(
        Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text("PRIMER TASKS", style = MaterialTheme.typography.labelLarge)
        Text("Pair this device", style = MaterialTheme.typography.headlineMedium)
        Text("Scan the one-use QR code shown by your parent.")
        Button(onClick = onScan) { Text("Scan pairing QR") }
        if (message != null) Text(message, color = MaterialTheme.colorScheme.error)
    }
}

@Composable
private fun PairingScanner(onQr: (String) -> Unit, onCancel: () -> Unit) {
    val context = LocalContext.current
    var cameraGranted by remember {
        mutableStateOf(ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED)
    }
    val permissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        cameraGranted = it
    }
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("Scan pairing QR", style = MaterialTheme.typography.headlineMedium)
        if (cameraGranted) {
            CameraQrScanner(onQr = onQr, modifier = Modifier.fillMaxWidth().weight(1f))
        } else {
            Spacer(Modifier.weight(1f))
            Text("Camera access is needed to scan a pairing QR.")
            Button(onClick = { permissionLauncher.launch(Manifest.permission.CAMERA) }) { Text("Allow camera") }
        }
        OutlinedButton(onClick = onCancel) { Text("Cancel") }
    }
}

@Composable
private fun CameraQrScanner(onQr: (String) -> Unit, modifier: Modifier = Modifier) {
    val lifecycleOwner = LocalLifecycleOwner.current
    val context = LocalContext.current
    val preview = remember { PreviewView(context) }
    DisposableEffect(lifecycleOwner) {
        val executor = Executors.newSingleThreadExecutor()
        val providerFuture = ProcessCameraProvider.getInstance(context)
        val listener = Runnable {
            val provider = providerFuture.get()
            val cameraPreview = Preview.Builder().build().also { it.surfaceProvider = preview.surfaceProvider }
            val analysis = ImageAnalysis.Builder()
                // Decode the actual packed CameraX frame instead of assuming a tightly packed Y plane.
                .setOutputImageFormat(ImageAnalysis.OUTPUT_IMAGE_FORMAT_RGBA_8888)
                .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                .build()
            val mainExecutor = ContextCompat.getMainExecutor(context)
            analysis.setAnalyzer(executor, QrAnalyzer { raw -> mainExecutor.execute { onQr(raw) } })
            provider.unbindAll()
            provider.bindToLifecycle(lifecycleOwner, CameraSelector.DEFAULT_BACK_CAMERA, cameraPreview, analysis)
        }
        providerFuture.addListener(listener, ContextCompat.getMainExecutor(context))
        onDispose {
            if (providerFuture.isDone) providerFuture.get().unbindAll()
            executor.shutdown()
        }
    }
    AndroidView(factory = { preview }, modifier = modifier)
}

private class QrAnalyzer(private val onQr: (String) -> Unit) : ImageAnalysis.Analyzer {
    private val delivered = AtomicBoolean(false)

    override fun analyze(image: ImageProxy) {
        try {
            if (!delivered.get()) {
                val plane = image.planes.singleOrNull()
                if (plane != null) {
                    // Copy the complete plane from offset zero. Decoder offsets are based on
                    // CameraX's row/pixel stride and crop metadata, not ByteBuffer's current cursor.
                    val buffer = plane.buffer.duplicate().apply { position(0) }
                    val bytes = ByteArray(buffer.remaining()).also { buffer.get(it) }
                    val crop = image.cropRect
                    val text = QrFrameDecoder.decode(
                        RgbaFrame(
                            bytes = bytes,
                            width = crop.width(),
                            height = crop.height(),
                            rowStride = plane.rowStride,
                            pixelStride = plane.pixelStride,
                            cropLeft = crop.left,
                            cropTop = crop.top,
                        ),
                        image.imageInfo.rotationDegrees,
                    )
                    if (text != null && delivered.compareAndSet(false, true)) onQr(text)
                }
            }
        } catch (_: Exception) {
            // Frames can be closed or malformed while the camera is rebinding. Drop only this frame.
        } finally {
            image.close()
        }
    }
}

@Composable
private fun ChecklistScreen(name: String, items: List<ChecklistItem>, message: String?) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("PRIMER TASKS", style = MaterialTheme.typography.labelLarge)
        Text(name, style = MaterialTheme.typography.headlineMedium)
        Text("Today’s checklist", style = MaterialTheme.typography.titleLarge)
        if (items.isEmpty()) {
            Text("Nothing assigned yet. Your checklist is empty.")
        } else {
            items.forEach { item ->
                Text("• ${item.title}", style = MaterialTheme.typography.bodyLarge)
                if (item.description.isNotBlank()) Text(item.description, style = MaterialTheme.typography.bodySmall)
            }
        }
        if (message != null) Text(message, color = MaterialTheme.colorScheme.error)
        Text("One student · one device", style = MaterialTheme.typography.bodySmall)
    }
}
