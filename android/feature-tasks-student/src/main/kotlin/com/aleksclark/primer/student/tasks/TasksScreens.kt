package com.aleksclark.primer.student.tasks

import android.Manifest
import android.content.pm.PackageManager
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
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
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import com.aleksclark.primer.ui.PrimerButton
import com.aleksclark.primer.ui.PrimerButtonVariant
import com.aleksclark.primer.ui.PrimerEmptyState
import com.aleksclark.primer.ui.PrimerRecordRow
import com.aleksclark.primer.ui.PrimerSectionHeader
import com.aleksclark.primer.ui.PrimerStatus
import com.aleksclark.primer.ui.PrimerStatusTone
import com.aleksclark.primer.ui.PrimerTheme
import com.aleksclark.primertasks.client.ChecklistItem
import com.aleksclark.primertasks.client.OccurrenceResponse
import kotlinx.coroutines.launch
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

@Composable
fun StudentTasksRoute(
    deepLink: Uri? = null,
    onLeave: (() -> Unit)? = null,
    onDeepLinkConsumed: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val session = remember { TasksSession(context) }
    StudentTasksApp(
        session = session,
        deepLink = deepLink,
        onLeave = onLeave,
        onDeepLinkConsumed = onDeepLinkConsumed,
        modifier = modifier,
    )
}

@Composable
fun StudentTasksApp(
    session: TasksSession,
    deepLink: Uri? = null,
    onLeave: (() -> Unit)? = null,
    onDeepLinkConsumed: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
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

    fun apply(result: TasksRestoreResult) {
        when (result) {
            is TasksRestoreResult.Paired -> {
                token = result.token
                metadata = result.metadata
                checklist = result.checklist
                occurrences = result.today
                upcoming = result.upcoming
                message = result.message
                busy = false
            }
            is TasksRestoreResult.Unpaired -> {
                if (result.retainedToken == null) {
                    token = null
                    metadata = null
                    checklist = emptyList()
                    occurrences = emptyList()
                    upcoming = emptyList()
                    selectedOccurrence = null
                    scanning = false
                    deepLinkUnavailable = false
                } else {
                    token = result.retainedToken
                    metadata = result.retainedMetadata
                }
                message = result.message
                busy = false
            }
        }
    }

    fun applyLookup(lookup: OccurrenceLookup) {
        when (lookup) {
            is OccurrenceLookup.Found -> {
                selectedOccurrence = lookup.occurrence
                deepLinkUnavailable = false
            }
            OccurrenceLookup.Unavailable -> {
                selectedOccurrence = null
                deepLinkUnavailable = true
                message = "This task is unavailable."
            }
            is OccurrenceLookup.Failed -> {
                deepLinkUnavailable = false
                message = lookup.message
            }
            is OccurrenceLookup.Revoked -> {
                token = null
                metadata = null
                checklist = emptyList()
                occurrences = emptyList()
                upcoming = emptyList()
                selectedOccurrence = null
                scanning = false
                deepLinkUnavailable = false
                message = lookup.message
                busy = false
            }
        }
    }

    fun applyAction(result: OccurrenceActionResult) {
        when (result) {
            is OccurrenceActionResult.Updated -> {
                selectedOccurrence = result.occurrence
                message = null
            }
            is OccurrenceActionResult.Conflict -> {
                selectedOccurrence = result.occurrence
                message = result.message
            }
            is OccurrenceActionResult.Failed -> message = result.message
            is OccurrenceActionResult.Revoked -> {
                token = null
                metadata = null
                checklist = emptyList()
                occurrences = emptyList()
                upcoming = emptyList()
                selectedOccurrence = null
                scanning = false
                deepLinkUnavailable = false
                message = result.message
                busy = false
            }
        }
    }

    LaunchedEffect(Unit) {
        apply(session.restore())
    }

    LaunchedEffect(deepLink) {
        val occurrenceId = occurrenceIdFromDeepLink(deepLink) ?: return@LaunchedEffect
        val savedToken = token
        val savedMetadata = metadata
        if (savedToken != null && savedMetadata != null) {
            applyLookup(session.loadOccurrence(savedToken, savedMetadata.origin, occurrenceId))
        } else {
            val restored = session.restore()
            apply(restored)
            val paired = restored as? TasksRestoreResult.Paired
            if (paired != null) {
                applyLookup(session.loadOccurrence(paired.token, paired.metadata.origin, occurrenceId))
            }
        }
        onDeepLinkConsumed?.invoke()
    }

    fun pair(raw: String) {
        scanning = false
        busy = true
        scope.launch { apply(session.pair(raw)) }
    }

    val imagePicker = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.PickVisualMedia(),
    ) { uri ->
        if (uri != null) {
            busy = true
            scope.launch { apply(session.importImage(context, uri)) }
        }
    }

    Column(modifier.fillMaxSize()) {
        when {
            busy && metadata == null -> LoadingScreen()
            metadata != null && token != null && deepLinkUnavailable -> UnavailableOccurrenceScreen(
                onBack = {
                    deepLinkUnavailable = false
                    message = null
                    onLeave?.invoke()
                },
            )
            metadata != null && token != null && selectedOccurrence != null -> OccurrenceDetailScreen(
                occurrence = selectedOccurrence!!,
                message = message,
                onBack = {
                    selectedOccurrence = null
                    onLeave?.invoke()
                },
                onRefresh = {
                    scope.launch {
                        applyLookup(session.loadOccurrence(token!!, metadata!!.origin, selectedOccurrence!!.id))
                    }
                },
                onStart = {
                    scope.launch {
                        applyAction(session.start(token!!, metadata!!.origin, selectedOccurrence!!.id))
                    }
                },
                onSubmit = {
                    scope.launch {
                        applyAction(session.submit(token!!, metadata!!.origin, selectedOccurrence!!.id))
                    }
                },
            )
            metadata != null && token != null -> ChecklistScreen(
                name = metadata!!.displayName,
                items = checklist,
                occurrences = occurrences,
                upcoming = upcoming,
                message = message,
                onOpen = { selectedOccurrence = it },
                onBack = onLeave,
            )
            scanning -> PairingScanner(onQr = ::pair, onCancel = { scanning = false })
            else -> PairingScreen(
                message = message,
                onScan = { message = null; scanning = true },
                onImportImage = {
                    message = null
                    imagePicker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                },
                onBack = onLeave,
            )
        }
    }
}

@Composable
private fun LoadingScreen() {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center) {
        CircularProgressIndicator(color = PrimerTheme.colors.accent)
    }
}

@Composable
internal fun PairingScreen(
    message: String?,
    onScan: () -> Unit,
    onImportImage: () -> Unit,
    onBack: (() -> Unit)? = null,
) {
    Column(
        Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        PrimerSectionHeader(
            label = "Primer Tasks",
            title = "Pair this device",
            description = "Scan the one-use QR code shown by your parent. Old Primer Tasks pairings cannot be copied; request a new Student QR.",
        )
        PrimerButton(text = "Scan pairing QR", onClick = onScan)
        PrimerButton(
            text = "Import pairing QR image",
            onClick = onImportImage,
            variant = PrimerButtonVariant.Secondary,
            modifier = Modifier.semantics { contentDescription = "Import pairing QR image" },
        )
        if (onBack != null) PrimerButton(text = "Back to Student", onClick = onBack, variant = PrimerButtonVariant.Quiet)
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
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
        PrimerSectionHeader(label = "Primer Tasks", title = "Scan pairing QR")
        if (cameraGranted) {
            CameraQrScanner(onQr = onQr, modifier = Modifier.fillMaxWidth().weight(1f))
        } else {
            Spacer(Modifier.weight(1f))
            Text("Camera access is needed to scan a pairing QR.", style = PrimerTheme.typography.body)
            PrimerButton(text = "Allow camera", onClick = { permissionLauncher.launch(Manifest.permission.CAMERA) })
        }
        PrimerButton(text = "Cancel", onClick = onCancel, variant = PrimerButtonVariant.Secondary)
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
        } finally {
            image.close()
        }
    }
}

@Composable
private fun ChecklistScreen(
    name: String,
    items: List<ChecklistItem>,
    occurrences: List<OccurrenceResponse>,
    upcoming: List<OccurrenceResponse>,
    message: String?,
    onOpen: (OccurrenceResponse) -> Unit,
    onBack: (() -> Unit)?,
) {
    val sections = checklistSections(occurrences.size, upcoming.size)
    LazyColumn(
        Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        contentPadding = PaddingValues(bottom = 32.dp),
    ) {
        item {
            PrimerSectionHeader(label = "Primer Tasks", title = name, description = "One student · one device")
        }
        item { Text("Today", style = PrimerTheme.typography.sectionTitle) }
        if (!sections.showToday && items.isEmpty()) {
            item { PrimerEmptyState(title = "Nothing assigned yet", message = "Your checklist is empty.") }
        }
        items(occurrences, key = { it.id }) { occurrence ->
            PrimerButton(
                text = occurrence.title,
                onClick = { onOpen(occurrence) },
                variant = PrimerButtonVariant.Secondary,
                modifier = Modifier.fillMaxWidth().semantics {
                    contentDescription = "Today task ${occurrence.title}, ${occurrence.status}"
                },
            )
        }
        if (occurrences.isEmpty()) {
            items(items, key = { it.id }) { checklistItem ->
                Text("• ${checklistItem.title}", style = PrimerTheme.typography.body)
            }
        }
        if (sections.showUpcoming) {
            item { Text("Upcoming", style = PrimerTheme.typography.sectionTitle) }
            items(upcoming.take(10), key = { "upcoming-${it.id}" }) { occurrence ->
                PrimerButton(
                    text = "${occurrence.title} · ${occurrence.nominalAt}",
                    onClick = { onOpen(occurrence) },
                    variant = PrimerButtonVariant.Secondary,
                    modifier = Modifier.fillMaxWidth().semantics {
                        contentDescription = "Upcoming task ${occurrence.title}, ${occurrence.nominalAt}"
                    },
                )
            }
        }
        if (message != null) item { PrimerStatus(message, tone = PrimerStatusTone.Attention) }
        if (onBack != null) item { PrimerButton(text = "Back to Student", onClick = onBack, variant = PrimerButtonVariant.Quiet) }
    }
}

@Composable
private fun UnavailableOccurrenceScreen(onBack: () -> Unit) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        PrimerSectionHeader(label = "Task unavailable", title = "This task is unavailable.")
        Text("The requested task could not be opened.", style = PrimerTheme.typography.body)
        PrimerButton(text = "Back to today", onClick = onBack)
    }
}

@Composable
private fun OccurrenceDetailScreen(
    occurrence: OccurrenceResponse,
    message: String?,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onStart: () -> Unit,
    onSubmit: () -> Unit,
) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        PrimerSectionHeader(label = "Task detail", title = occurrence.title)
        Text(occurrence.instructions, style = PrimerTheme.typography.body)
        Text("Status: ${occurrence.status}", style = PrimerTheme.typography.sectionTitle)
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        PrimerRecordRow(
            label = "Status",
            value = occurrence.status,
            status = occurrence.status,
            statusTone = if (occurrence.status == "completed") PrimerStatusTone.Filled else PrimerStatusTone.Accent,
        )
        if (occurrence.status == "pending") PrimerButton(text = "Start task", onClick = onStart)
        if (occurrence.status == "in_progress") PrimerButton(text = "Submit for parent approval", onClick = onSubmit)
        PrimerButton(text = "Refresh from server", onClick = onRefresh, variant = PrimerButtonVariant.Secondary)
        PrimerButton(text = "Back to today", onClick = onBack, variant = PrimerButtonVariant.Quiet)
    }
}
