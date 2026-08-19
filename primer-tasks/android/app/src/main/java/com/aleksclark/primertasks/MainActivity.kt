package com.aleksclark.primertasks

import android.Manifest
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
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
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import com.aleksclark.primertasks.client.ChecklistItem
import com.aleksclark.primertasks.client.OccurrenceResponse
import com.aleksclark.primertasks.client.TasksClient
import com.aleksclark.primertasks.client.TasksHttpException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { PrimerTasksApp(applicationContext, intent?.data) }
    }
}

@Composable
private fun PrimerTasksApp(context: android.content.Context, deepLink: Uri? = null) {
    val tokenStore = remember { EncryptedTokenStore(context) }
    val metadataStore = remember { MetadataStore(context) }
    val scope = rememberCoroutineScope()
    var token by remember { mutableStateOf<String?>(null) }
    var metadata by remember { mutableStateOf<StudentMetadata?>(null) }
    var checklist by remember { mutableStateOf<List<ChecklistItem>>(emptyList()) }
    var occurrences by remember { mutableStateOf<List<OccurrenceResponse>>(emptyList()) }
    var upcoming by remember { mutableStateOf<List<OccurrenceResponse>>(emptyList()) }
    var selectedOccurrence by remember { mutableStateOf<OccurrenceResponse?>(null) }
    var deepLinkUnavailable by remember { mutableStateOf(false) }
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
            occurrences = emptyList()
            upcoming = emptyList()
            scanning = false
        }
    }

    suspend fun loadSession(savedToken: String, savedMetadata: StudentMetadata) {
        try {
            val client = TasksClient(savedMetadata.origin)
            val profile = client.studentProfile(savedToken)
            checklist = client.studentChecklist(savedToken).items
            occurrences = client.studentToday(savedToken).items
            upcoming = client.studentUpcoming(savedToken).items
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
            val occurrenceId = occurrenceIdFromDeepLink(deepLink)
            if (occurrenceId != null) {
                try { selectedOccurrence = TasksClient(savedMetadata.origin).studentOccurrence(savedToken, occurrenceId) }
                catch (_: TasksHttpException) {
                    // Keep foreign and unknown IDs indistinguishable from one another. Do not
                    // fall back to the checklist: a deep-link denial needs a visible, generic
                    // unavailable surface rather than a misleading successful navigation.
                    deepLinkUnavailable = true
                    message = "This task is unavailable."
                }
            }
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
            busy = false
            message = "That QR code is not a valid Primer pairing code. Choose a Primer pairing QR."
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
                occurrences = client.studentToday(paired.token).items
                upcoming = client.studentUpcoming(paired.token).items
                tokenStore.save(paired.token)
                val savedMetadata = StudentMetadata(paired.studentId, profile.displayName, origin, qr.pairingId)
                metadataStore.save(savedMetadata)
                token = paired.token
                metadata = savedMetadata
                busy = false
            } catch (error: TasksHttpException) {
                busy = false
                message = if (error.statusCode == 401 || error.statusCode == 403) {
                    "That pairing QR is expired or has already been used. Request a new QR code."
                } else {
                    "Pairing failed. Check the server and try again."
                }
            } catch (_: Exception) {
                busy = false
                message = "Unable to reach the Primer server. Check your connection and try again."
            }
        }
    }

    fun importPairingImage(uri: Uri) {
        scope.launch {
            busy = true
            val result = withContext(Dispatchers.IO) {
                QrImageImporter(context.contentResolver).decode(uri)
            }
            when (result) {
                is QrImageImporter.Result.Decoded -> {
                    // Feed the decoded text into the exact same parser/API path as CameraX.
                    pair(result.payload)
                }
                is QrImageImporter.Result.Failure -> {
                    busy = false
                    message = when (result.reason) {
                        QrImageImporter.Failure.UNREADABLE ->
                            "Couldn't read that image. Choose another QR image."
                        QrImageImporter.Failure.TOO_LARGE ->
                            "That image is too large to scan safely. Choose a smaller QR image."
                        QrImageImporter.Failure.NO_QR ->
                            "No valid Primer pairing QR code was found in that image."
                    }
                }
            }
        }
    }

    val imagePicker = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.PickVisualMedia(),
    ) { uri -> uri?.let(::importPairingImage) }

    MaterialTheme(colorScheme = androidx.compose.material3.darkColorScheme(primary = androidx.compose.ui.graphics.Color(0xFF3DE0F0))) {
        Surface(Modifier.fillMaxSize()) {
            when {
                busy && metadata == null -> LoadingScreen()
                metadata != null && token != null && deepLinkUnavailable -> UnavailableOccurrenceScreen(
                    onBack = {
                        deepLinkUnavailable = false
                        message = null
                    },
                )
                metadata != null && token != null && selectedOccurrence != null -> OccurrenceDetailScreen(
                    occurrence = selectedOccurrence!!,
                    onBack = { selectedOccurrence = null },
                    onRefresh = {
                        scope.launch {
                            try { selectedOccurrence = TasksClient(metadata!!.origin).studentOccurrence(token!!, selectedOccurrence!!.id) }
                            catch (_: Exception) { message = "Unable to refresh this task." }
                        }
                    },
                    onStart = {
                        scope.launch {
                            try {
                                TasksClient(metadata!!.origin).startStudentOccurrence(token!!, selectedOccurrence!!.id)
                                selectedOccurrence = TasksClient(metadata!!.origin).studentOccurrence(token!!, selectedOccurrence!!.id)
                            } catch (error: Exception) { message = if (error is TasksHttpException && error.statusCode == 403) "This task is not available to this student." else "Unable to start this task." }
                        }
                    },
                )
                metadata != null && token != null -> ChecklistScreen(metadata!!.displayName, checklist, occurrences, upcoming, message, onOpen = { selectedOccurrence = it })
                scanning -> PairingScanner(onQr = ::pair, onCancel = { scanning = false })
                else -> PairingScreen(
                    message = message,
                    onScan = { message = null; scanning = true },
                    onImportImage = {
                        message = null
                        imagePicker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                    },
                )
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
internal fun PairingScreen(
    message: String?,
    onScan: () -> Unit,
    onImportImage: () -> Unit,
) {
    Column(
        Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text("PRIMER TASKS", style = MaterialTheme.typography.labelLarge)
        Text("Pair this device", style = MaterialTheme.typography.headlineMedium)
        Text("Scan the one-use QR code shown by your parent.")
        // Camera scanning is deliberately the primary pairing action.
        Button(onClick = onScan) { Text("Scan pairing QR") }
        OutlinedButton(
            onClick = onImportImage,
            modifier = Modifier.semantics {
                contentDescription = "Import pairing QR image"
            },
        ) {
            Text("Import pairing QR image")
        }
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
            analysis.setAnalyzer(
                executor,
                QrAnalyzer(
                    onQr = { raw -> mainExecutor.execute { onQr(raw) } },
                    frameCapture = QrFrameCaptureFactory.forContext(context),
                ),
            )
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

private class QrAnalyzer(
    private val onQr: (String) -> Unit,
    private val frameCapture: QrFrameCapture = NoOpQrFrameCapture,
) : ImageAnalysis.Analyzer {
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
                    val frame = RgbaFrame(
                        bytes = bytes,
                        width = crop.width(),
                        height = crop.height(),
                        rowStride = plane.rowStride,
                        pixelStride = plane.pixelStride,
                        cropLeft = crop.left,
                        cropTop = crop.top,
                    )
                    // This is a redacted, DEBUG-only diagnostic. It receives the same real
                    // CameraX frame bytes as the decoder, but never persists those bytes.
                    frameCapture.capture(
                        frame = frame,
                        imageWidth = image.width,
                        imageHeight = image.height,
                        rotationDegrees = image.imageInfo.rotationDegrees,
                    )
                    val text = QrFrameDecoder.decode(frame, image.imageInfo.rotationDegrees)
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
private fun ChecklistScreen(name: String, items: List<ChecklistItem>, occurrences: List<OccurrenceResponse>, upcoming: List<OccurrenceResponse>, message: String?, onOpen: (OccurrenceResponse) -> Unit) {
    val sections = checklistSections(occurrences.size, upcoming.size)
    LazyColumn(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp), contentPadding = PaddingValues(bottom = 32.dp)) {
        item { Text("PRIMER TASKS", style = MaterialTheme.typography.labelLarge) }
        item { Text(name, style = MaterialTheme.typography.headlineMedium) }
        item { Text("Today", style = MaterialTheme.typography.titleLarge) }
        if (!sections.showToday && items.isEmpty()) item { Text("Nothing assigned yet. Your checklist is empty.") }
        items(occurrences, key = { it.id }) { occurrence ->
            OutlinedButton(onClick = { onOpen(occurrence) }, modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Today task ${occurrence.title}, ${occurrence.status}" }) {
                Column(Modifier.fillMaxWidth()) { Text(occurrence.title, style = MaterialTheme.typography.bodyLarge); Text(occurrence.status, style = MaterialTheme.typography.bodySmall) }
            }
        }
        if (occurrences.isEmpty()) items(items, key = { it.id }) { checklistItem -> Text("• ${checklistItem.title}", style = MaterialTheme.typography.bodyLarge) }
        if (sections.showUpcoming) {
            item { Text("Upcoming", style = MaterialTheme.typography.titleLarge) }
            items(upcoming.take(10), key = { "upcoming-${it.id}" }) { occurrence -> OutlinedButton(onClick = { onOpen(occurrence) }, modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Upcoming task ${occurrence.title}, ${occurrence.nominalAt}" }) { Text("${occurrence.title} · ${occurrence.nominalAt}") } }
        }
        if (message != null) item { Text(message, color = MaterialTheme.colorScheme.error) }
        item { Text("One student · one device", style = MaterialTheme.typography.bodySmall) }
    }
}

@Composable
private fun UnavailableOccurrenceScreen(onBack: () -> Unit) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Text("TASK UNAVAILABLE", style = MaterialTheme.typography.labelLarge)
        Text("This task is unavailable.", style = MaterialTheme.typography.headlineMedium)
        Text("The requested task could not be opened.")
        Button(onClick = onBack) { Text("Back to today") }
    }
}

@Composable
private fun OccurrenceDetailScreen(occurrence: OccurrenceResponse, onBack: () -> Unit, onRefresh: () -> Unit, onStart: () -> Unit) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Text("TASK DETAIL", style = MaterialTheme.typography.labelLarge)
        Text(occurrence.title, style = MaterialTheme.typography.headlineMedium)
        Text(occurrence.instructions)
        Text("Status: ${occurrence.status}", style = MaterialTheme.typography.titleMedium)
        if (occurrence.status == "pending") Button(onClick = onStart) { Text("Start task") }
        Button(onClick = onRefresh) { Text("Refresh from server") }
        OutlinedButton(onClick = onBack) { Text("Back to today") }
    }
}
