package com.aleksclark.primer.student.tasks

import android.Manifest
import android.content.pm.PackageManager
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.platform.testTag
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
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
import com.aleksclark.primer.ui.PrimerTextField
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
    pairing: com.aleksclark.primer.devicepolicy.PairingCapability? = null,
    onRequestParentCameraGrant: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val session = remember { TasksSession(context) }
    StudentTasksApp(
        session = session,
        deepLink = deepLink,
        onLeave = onLeave,
        onDeepLinkConsumed = onDeepLinkConsumed,
        pairing = pairing,
        onRequestParentCameraGrant = onRequestParentCameraGrant,
        modifier = modifier,
    )
}

@Composable
fun StudentTasksApp(
    session: TasksSession,
    deepLink: Uri? = null,
    onLeave: (() -> Unit)? = null,
    onDeepLinkConsumed: (() -> Unit)? = null,
    pairing: com.aleksclark.primer.devicepolicy.PairingCapability? = null,
    onRequestParentCameraGrant: (() -> Unit)? = null,
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
    val occurrenceView = remember { OccurrenceViewGate() }
    var restoreGeneration by remember { mutableStateOf(0L) }

    suspend fun restoreCurrent(): TasksRestoreResult {
        val generation = restoreGeneration
        val result = session.restore()
        return if (generation == restoreGeneration) result else TasksRestoreResult.Superseded
    }
    var deepLinkUnavailable by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(true) }
    var scanning by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf<String?>(null) }

    fun apply(result: TasksRestoreResult) {
        when (result) {
            TasksRestoreResult.Superseded -> Unit
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
                    restoreGeneration += 1
                    occurrenceView.invalidate()
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
            is TasksRestoreResult.Unavailable -> {
                token = result.token
                metadata = result.metadata
                message = result.message
                busy = false
            }
        }
    }

    fun applyLookup(lookup: OccurrenceLookup, request: OccurrenceViewRequest) {
        if (token != request.token || metadata?.origin != request.origin) return
        // Revocation of this pairing still clears Tasks after Back; a stale
        // success/error cannot reopen detail or replace a different selection.
        if (lookup !is OccurrenceLookup.Revoked && !occurrenceView.accepts(request)) return
        when (lookup) {
            is OccurrenceLookup.Found -> {
                selectedOccurrence = lookup.occurrence
                occurrences = replaceOccurrence(occurrences, lookup.occurrence)
                upcoming = replaceOccurrence(upcoming, lookup.occurrence)
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
                restoreGeneration += 1
                occurrenceView.invalidate()
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

    fun applyAction(result: OccurrenceActionResult, request: OccurrenceViewRequest) {
        if (token != request.token || metadata?.origin != request.origin) return
        if (result !is OccurrenceActionResult.Revoked && !occurrenceView.accepts(request)) return
        when (result) {
            is OccurrenceActionResult.Updated -> {
                selectedOccurrence = result.occurrence
                occurrences = replaceOccurrence(occurrences, result.occurrence)
                upcoming = replaceOccurrence(upcoming, result.occurrence)
                message = null
            }
            is OccurrenceActionResult.Conflict -> {
                selectedOccurrence = result.occurrence
                occurrences = replaceOccurrence(occurrences, result.occurrence)
                upcoming = replaceOccurrence(upcoming, result.occurrence)
                message = result.message
            }
            is OccurrenceActionResult.Failed -> message = result.message
            is OccurrenceActionResult.Revoked -> {
                restoreGeneration += 1
                occurrenceView.invalidate()
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
        apply(restoreCurrent())
    }

    LaunchedEffect(deepLink) {
        val occurrenceId = occurrenceIdFromDeepLink(deepLink) ?: return@LaunchedEffect
        val savedToken = token
        val savedMetadata = metadata
        if (savedToken != null && savedMetadata != null) {
            val request = occurrenceView.begin(savedToken, savedMetadata.origin, occurrenceId)
            applyLookup(session.loadOccurrence(request.token, request.origin, request.occurrenceId), request)
        } else {
            val restored = restoreCurrent()
            apply(restored)
            val paired = restored as? TasksRestoreResult.Paired
            if (paired != null) {
                val request = occurrenceView.begin(paired.token, paired.metadata.origin, occurrenceId)
                applyLookup(session.loadOccurrence(request.token, request.origin, request.occurrenceId), request)
            }
        }
        onDeepLinkConsumed?.invoke()
    }

    fun pair(raw: String) {
        restoreGeneration += 1
        occurrenceView.invalidate()
        scanning = false
        busy = true
        scope.launch { apply(session.pair(raw)) }
    }

    val imagePicker = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.PickVisualMedia(),
    ) { uri ->
        if (uri != null) {
            restoreGeneration += 1
            occurrenceView.invalidate()
            busy = true
            scope.launch { apply(session.importImage(context, uri)) }
        }
    }

    BackHandler(enabled = scanning || selectedOccurrence != null || deepLinkUnavailable) {
        when {
            scanning -> scanning = false
            deepLinkUnavailable -> {
                occurrenceView.invalidate()
                deepLinkUnavailable = false
                message = null
            }
            selectedOccurrence != null -> {
                occurrenceView.invalidate()
                selectedOccurrence = null
                message = null
            }
        }
    }

    Column(modifier.fillMaxSize()) {
        when {
            busy && metadata == null -> LoadingScreen()
            metadata != null && token != null && deepLinkUnavailable -> UnavailableOccurrenceScreen(
                onBack = {
                    occurrenceView.invalidate()
                    deepLinkUnavailable = false
                    message = null
                },
            )
            metadata != null && token != null && selectedOccurrence != null -> OccurrenceDetailScreen(
                occurrence = selectedOccurrence!!,
                message = message,
                onBack = {
                    occurrenceView.invalidate()
                    selectedOccurrence = null
                    message = null
                },
                onRefresh = {
                    val request = occurrenceView.begin(token!!, metadata!!.origin, selectedOccurrence!!.id)
                    scope.launch {
                        applyLookup(session.loadOccurrence(request.token, request.origin, request.occurrenceId), request)
                    }
                },
                onStart = {
                    val request = occurrenceView.begin(token!!, metadata!!.origin, selectedOccurrence!!.id)
                    scope.launch {
                        applyAction(session.start(request.token, request.origin, request.occurrenceId), request)
                    }
                },
                onSubmit = {
                    val request = occurrenceView.begin(token!!, metadata!!.origin, selectedOccurrence!!.id)
                    scope.launch {
                        applyAction(session.submit(request.token, request.origin, request.occurrenceId), request)
                    }
                },
            )
            metadata != null && token != null -> ChecklistScreen(
                name = metadata!!.displayName,
                items = checklist,
                occurrences = occurrences,
                upcoming = upcoming,
                message = message,
                onOpen = {
                    occurrenceView.invalidate()
                    selectedOccurrence = it
                },
                onRefresh = {
                    busy = true
                    scope.launch { apply(restoreCurrent()) }
                },
                onBack = onLeave,
            )
            scanning -> PayloadQrScanner(
                title = "Scan pairing QR",
                onQr = ::pair,
                onCancel = { scanning = false },
                pairing = pairing,
                onRequestParentCameraGrant = onRequestParentCameraGrant,
            )
            else -> PairingScreen(
                message = message,
                pairing = pairing,
                onScan = {
                    if (pairing?.canScan == false) {
                        message = pairing.message
                    } else {
                        message = null
                        scanning = true
                    }
                },
                onImportImage = {
                    if (pairing != null && !pairing.canImportImage) {
                        message = pairing.importMessage
                    } else {
                        message = null
                        imagePicker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                    }
                },
                onPaste = ::pair,
                onRequestParentCameraGrant = onRequestParentCameraGrant,
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

internal object PairingActions {
    const val SCAN = "Scan parent’s QR code"
    const val IMPORT_IMAGE = "Use a saved QR image"
    const val FALLBACK = "Can’t scan the code?"
    const val PASTE_LABEL = "Pairing information"
    const val PASTE_ACTION = "Connect this device"
    const val PASTE_HELP =
        "Paste the pairing information from your parent only when scanning and image import are unavailable."
}

@Composable
internal fun PairingScreen(
    message: String?,
    onScan: () -> Unit,
    onImportImage: () -> Unit,
    onPaste: (String) -> Unit = {},
    pairing: com.aleksclark.primer.devicepolicy.PairingCapability? = null,
    onRequestParentCameraGrant: (() -> Unit)? = null,
    onBack: (() -> Unit)? = null,
) {
    var payload by remember { mutableStateOf("") }
    var showFallback by rememberSaveable { mutableStateOf(false) }
    Column(
        Modifier.fillMaxSize().testTag("tasks.pair").padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        PrimerSectionHeader(
            label = "Primer Tasks",
            title = "Connect this device",
            description = "Scan the one-use QR code shown by your parent.",
            trailing = if (onBack != null) {
                {
                    PrimerButton(text = "Back", onClick = onBack, variant = PrimerButtonVariant.Quiet)
                }
            } else {
                null
            },
        )
        PrimerButton(text = PairingActions.SCAN, onClick = onScan)
        PrimerButton(
            text = PairingActions.IMPORT_IMAGE,
            onClick = onImportImage,
            variant = PrimerButtonVariant.Secondary,
            modifier = Modifier.semantics { contentDescription = PairingActions.IMPORT_IMAGE },
        )
        PrimerButton(
            text = PairingActions.FALLBACK,
            onClick = { showFallback = !showFallback },
            variant = PrimerButtonVariant.Quiet,
        )
        if (showFallback) {
            Text(PairingActions.PASTE_HELP, style = PrimerTheme.typography.body, color = PrimerTheme.colors.textMuted)
            PrimerTextField(
                value = payload,
                onValueChange = { payload = it },
                label = PairingActions.PASTE_LABEL,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Ascii, autoCorrectEnabled = false),
                modifier = Modifier.semantics { contentDescription = PairingActions.PASTE_LABEL },
            )
            PrimerButton(
                text = PairingActions.PASTE_ACTION,
                onClick = {
                    val raw = payload
                    payload = ""
                    onPaste(raw)
                },
                enabled = payload.isNotBlank(),
                variant = PrimerButtonVariant.Secondary,
                modifier = Modifier.semantics { contentDescription = PairingActions.PASTE_ACTION },
            )
        }
        if (pairing != null && !pairing.canScan) {
            PrimerStatus(pairing.message, tone = PrimerStatusTone.Attention)
            if (pairing.parentCanGrantCamera && onRequestParentCameraGrant != null) {
                PrimerButton(text = "Grant camera for pairing", onClick = onRequestParentCameraGrant)
            }
        }
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
    }
}

@Composable
fun PayloadQrScanner(
    title: String,
    onQr: (String) -> Unit,
    onCancel: () -> Unit,
    pairing: com.aleksclark.primer.devicepolicy.PairingCapability? = null,
    onRequestParentCameraGrant: (() -> Unit)? = null,
) {
    val context = LocalContext.current
    val granted = pairing?.canScan
        ?: (ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED)
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        PrimerSectionHeader(label = "Primer", title = title)
        if (granted) {
            CameraQrScanner(onQr = onQr, modifier = Modifier.fillMaxWidth().weight(1f))
        } else {
            Spacer(Modifier.weight(1f))
            PrimerStatus(
                pairing?.message ?: "Camera access is needed to scan a pairing QR. Ask a parent to open maintenance and grant camera; the system permission screen is blocked in lock-task.",
                tone = PrimerStatusTone.Attention,
            )
            if (pairing?.parentCanGrantCamera == true && onRequestParentCameraGrant != null) {
                PrimerButton(text = "Grant camera for pairing", onClick = onRequestParentCameraGrant)
            }
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
internal fun ChecklistScreen(
    name: String,
    items: List<ChecklistItem>,
    occurrences: List<OccurrenceResponse>,
    upcoming: List<OccurrenceResponse>,
    message: String?,
    onOpen: (OccurrenceResponse) -> Unit,
    onRefresh: () -> Unit,
    onBack: (() -> Unit)?,
) {
    var showCompleted by rememberSaveable { mutableStateOf(false) }
    val sections = checklistSections(occurrences.size, upcoming.size)
    val approved = occurrences.count { it.status == "completed" }
    val waiting = occurrences.count { it.status == "awaiting_verification" }
    val actionRequired = occurrences.count { presentOccurrence(it).studentActionRequired } + items.count { it.status == "pending" || it.status == "in_progress" }
    val terminalCount = occurrences.count { terminalOccurrenceStatus(it.status) } + items.count { terminalOccurrenceStatus(it.status) }
    val visibleOccurrences = sortOccurrencesForStudent(
        occurrences.filter { showCompleted || !terminalOccurrenceStatus(it.status) },
    )
    val visibleItems = items
        .filter { showCompleted || !terminalOccurrenceStatus(it.status) }
        .withIndex()
        .sortedWith(compareBy<IndexedValue<ChecklistItem>> { statusPriority(it.value.status, false) }.thenBy { it.index })
        .map { it.value }
    val summary = buildList {
        add("$actionRequired active")
        add("$approved approved")
        if (waiting > 0) add("$waiting sent to your parent")
    }.joinToString(" · ")
    LazyColumn(
        Modifier.fillMaxSize().testTag("tasks.checklist").padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        contentPadding = PaddingValues(bottom = 32.dp),
    ) {
        item {
            PrimerSectionHeader(
                label = "Today",
                title = name,
                description = summary,
                trailing = if (onBack != null) {
                    {
                        PrimerButton(text = "Back", onClick = onBack, variant = PrimerButtonVariant.Quiet)
                    }
                } else {
                    null
                },
            )
        }
        if (!sections.showToday && items.isEmpty()) {
            item {
                PrimerEmptyState(
                    title = "You don’t have any tasks today",
                    message = "You’re all caught up. Check back later or ask your parent.",
                )
            }
        }
        items(visibleOccurrences, key = { it.id }) { occurrence ->
            val presented = presentOccurrence(occurrence)
            StudentTaskRow(
                title = occurrence.title,
                detail = occurrence.instructions,
                status = presented.statusLabel,
                statusColor = studentStatusColor(occurrence.status, occurrence.attemptNumber > 0),
                studentActionRequired = presented.studentActionRequired,
                contentDescription = "Open ${occurrence.title}, ${presented.statusLabel}",
                modifier = Modifier.testTag("tasks.checklist.today.${occurrence.id}"),
                onClick = { onOpen(occurrence) },
            )
        }
        if (occurrences.isEmpty()) {
            items(visibleItems, key = { it.id }) { checklistItem ->
                StudentTaskRow(
                    title = checklistItem.title,
                    detail = checklistItem.description ?: "",
                    status = occurrenceStatusLabel(checklistItem.status, false),
                    statusColor = studentStatusColor(checklistItem.status, false),
                    studentActionRequired = checklistItem.status == "pending" || checklistItem.status == "in_progress",
                    contentDescription = "${checklistItem.title}, ${occurrenceStatusLabel(checklistItem.status, false)}",
                    onClick = null,
                )
            }
        }
        if (sections.showUpcoming) {
            item { Text("Coming up", style = PrimerTheme.typography.sectionTitle) }
            items(upcoming.take(10), key = { "upcoming-${it.id}" }) { occurrence ->
                StudentTaskRow(
                    title = occurrence.title,
                    detail = "Scheduled ${studentScheduleTime(occurrence.nominalAt)}",
                    status = "Coming up",
                    statusColor = PrimerTheme.colors.textMuted,
                    studentActionRequired = false,
                    contentDescription = "Open ${occurrence.title}, coming up ${studentScheduleTime(occurrence.nominalAt)}",
                    modifier = Modifier.testTag("tasks.checklist.upcoming.${occurrence.id}"),
                    onClick = { onOpen(occurrence) },
                )
            }
        }
        if (terminalCount > 0) {
            item {
                PrimerButton(
                    text = if (showCompleted) "Hide completed tasks" else "Show completed tasks ($terminalCount)",
                    onClick = { showCompleted = !showCompleted },
                    variant = PrimerButtonVariant.Quiet,
                )
            }
        }
        item { PrimerButton(text = StudentOccurrenceCopy.REFRESH, onClick = onRefresh, variant = PrimerButtonVariant.Quiet) }
        if (message != null) item { PrimerStatus(message, tone = PrimerStatusTone.Attention) }
    }
}

@Composable
private fun StudentTaskRow(
    title: String,
    detail: String,
    status: String,
    statusColor: Color,
    studentActionRequired: Boolean,
    contentDescription: String,
    onClick: (() -> Unit)?,
    modifier: Modifier = Modifier,
) {
    val interactive = if (onClick == null) modifier else modifier.clickable(onClick = onClick)
    Column(
        interactive
            .fillMaxWidth()
            .semantics {
                this.contentDescription = contentDescription
                if (onClick != null) role = Role.Button
            }
            .padding(vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(title, style = PrimerTheme.typography.sectionTitle, color = PrimerTheme.colors.text)
                if (detail.isNotBlank()) Text(detail, style = PrimerTheme.typography.body, color = PrimerTheme.colors.textMuted)
            }
            Text(if (onClick == null) "" else "Open →", style = PrimerTheme.typography.label, color = PrimerTheme.colors.accent)
        }
        Text(
            status.uppercase(),
            style = PrimerTheme.typography.label,
            color = if (status == "Ready to start") PrimerTheme.colors.onAccent else statusColor,
            modifier = Modifier
                .background(if (status == "Ready to start") statusColor else Color.Transparent)
                .border(PrimerTheme.spacing.rule, statusColor)
                .padding(horizontal = 8.dp, vertical = 4.dp),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        HorizontalDivider(color = if (studentActionRequired) statusColor else PrimerTheme.colors.rule)
    }
}

@Composable
private fun studentStatusColor(status: String, retried: Boolean): Color = when {
    status == "in_progress" -> PrimerTheme.colors.statusInProgress
    status == "awaiting_verification" -> PrimerTheme.colors.statusSent
    status == "pending" && retried -> PrimerTheme.colors.attention
    status == "pending" -> PrimerTheme.colors.accent
    else -> PrimerTheme.colors.ruleStrong
}

private fun studentScheduleTime(value: String): String = value
    .replace("T", " at ")
    .removeSuffix("Z")
    .substringBeforeLast(":")

@Composable
internal fun UnavailableOccurrenceScreen(onBack: () -> Unit) {
    Column(Modifier.fillMaxSize().testTag("tasks.occurrence.unavailable").padding(24.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        PrimerSectionHeader(label = "Task unavailable", title = "This task is unavailable.")
        Text("The requested task could not be opened.", style = PrimerTheme.typography.body)
        PrimerButton(text = "Back to today", onClick = onBack)
    }
}

@Composable
internal fun OccurrenceDetailScreen(
    occurrence: OccurrenceResponse,
    message: String?,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onStart: () -> Unit,
    onSubmit: () -> Unit,
) {
    val presented = presentOccurrence(occurrence)
    Column(Modifier.fillMaxSize().testTag("tasks.occurrence").padding(24.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        PrimerButton(text = "← Back to today", onClick = onBack, variant = PrimerButtonVariant.Quiet)
        PrimerSectionHeader(label = "Your tasks", title = occurrence.title)
        PrimerStatus(
            presented.statusLabel,
            tone = when (presented.tone) {
                OccurrenceStatusTone.Filled -> PrimerStatusTone.Filled
                OccurrenceStatusTone.Attention -> PrimerStatusTone.Attention
                OccurrenceStatusTone.Accent -> PrimerStatusTone.Accent
                OccurrenceStatusTone.InProgress -> PrimerStatusTone.Accent
                OccurrenceStatusTone.Sent -> PrimerStatusTone.Accent
                OccurrenceStatusTone.Neutral -> PrimerStatusTone.Neutral
            },
        )
        Text("WHAT TO DO", style = PrimerTheme.typography.label, color = PrimerTheme.colors.textMuted)
        Text(occurrence.instructions, style = PrimerTheme.typography.body)
        if (message != null) PrimerStatus(message, tone = PrimerStatusTone.Attention)
        if (presented.explanation != null) {
            Text(
                presented.explanation,
                style = PrimerTheme.typography.body,
                color = if (presented.supported) PrimerTheme.colors.text else PrimerTheme.colors.attention,
            )
        }
        if (presented.canStart) {
            PrimerButton(
                text = if (occurrence.attemptNumber > 0) StudentOccurrenceCopy.RETRY else StudentOccurrenceCopy.START,
                onClick = onStart,
                modifier = Modifier.fillMaxWidth().testTag("tasks.occurrence.start"),
            )
        }
        if (presented.canSubmit) {
            Text("Your work will stay saved while your parent checks it.", style = PrimerTheme.typography.body, color = PrimerTheme.colors.textMuted)
            PrimerButton(text = StudentOccurrenceCopy.SUBMIT, onClick = onSubmit, modifier = Modifier.fillMaxWidth().testTag("tasks.occurrence.submit"))
        }
        PrimerButton(text = StudentOccurrenceCopy.REFRESH, onClick = onRefresh, variant = PrimerButtonVariant.Quiet, modifier = Modifier.testTag("tasks.occurrence.refresh"))
        if (!presented.canStart && !presented.canSubmit) {
            PrimerButton(text = "Back to today’s tasks", onClick = onBack, variant = PrimerButtonVariant.Secondary)
        }
    }
}
