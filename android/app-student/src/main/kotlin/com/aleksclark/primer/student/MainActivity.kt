package com.aleksclark.primer.student

import android.app.ActivityManager
import android.app.KeyguardManager
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.provider.Settings
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.lifecycleScope
import com.aleksclark.primer.devicepolicy.ApprovedApp
import com.aleksclark.primer.ui.PrimerButton
import com.aleksclark.primer.ui.PrimerButtonVariant
import com.aleksclark.primer.ui.PrimerCheckboxRow
import com.aleksclark.primer.ui.PrimerRecordRow
import com.aleksclark.primer.ui.PrimerRule
import com.aleksclark.primer.ui.PrimerSectionHeader
import com.aleksclark.primer.ui.PrimerStatus
import com.aleksclark.primer.ui.PrimerStatusTone
import com.aleksclark.primer.ui.PrimerTextField
import com.aleksclark.primer.ui.PrimerTheme
import com.aleksclark.primer.student.tasks.PayloadQrScanner
import com.aleksclark.primer.student.tasks.QrImageImporter
import com.aleksclark.primer.student.tasks.StudentTasksRoute
import com.aleksclark.primer.student.tasks.TasksDeepLinkRouting
import com.aleksclark.primer.student.tasks.TasksNavState
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

open class MainActivity : ComponentActivity() {
    protected val runtime by lazy { StudentRuntime(this) }
    protected open val enforceKiosk = true
    private var lifecycleFailure by mutableStateOf<String?>(null)
    private var lockVerification: Job? = null
    private var deepLink by mutableStateOf<Uri?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Recovery material must never appear in screenshots, recents thumbnails or recordings.
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        deepLink = intent?.data
        setContent {
            StudentTheme {
                StudentScreen(
                    activity = this,
                    runtime = runtime,
                    lifecycleFailure = lifecycleFailure,
                    deepLink = deepLink,
                    onConfigured = { onParentSetupComplete() },
                )
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        deepLink = intent.data
    }
    override fun onResume() {
        super.onResume()
        try {
            runtime.reconcile()
            lifecycleFailure = null
            if (enforceKiosk) requestKiosk()
        } catch (e: RuntimeException) {
            lifecycleFailure = "Policy enforcement failed: ${e.javaClass.simpleName}"
            runtime.policy.store.record(lifecycleFailure!!)
        }
    }
    override fun onPause() {
        lockVerification?.cancel()
        super.onPause()
    }
    protected open fun onParentSetupComplete() { requestKiosk() }

    private fun requestKiosk() {
        lockVerification?.cancel()
        if (!runtime.policy.isOwner || !runtime.policy.store.configured) return
        lockVerification = lifecycleScope.launch {
            // A HOME activity can resume behind keyguard during boot without another
            // onResume when keyguard goes away. Wait while resumed; never dismiss it.
            try {
                val keyguard = getSystemService(KeyguardManager::class.java)
                while (keyguard.isKeyguardLocked) delay(250)
                if (!runtime.policy.enterLockTask(this@MainActivity)) return@launch
                repeat(20) {
                    if (runtime.policy.lockTaskState == ActivityManager.LOCK_TASK_MODE_LOCKED) return@launch
                    delay(100)
                }
                lifecycleFailure = "Android has not entered managed lock task; reopen Student to retry."
                runtime.policy.store.record(lifecycleFailure!!)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: RuntimeException) {
                // The request runs asynchronously; the onResume try/catch cannot catch
                // a DPM/storage failure here. Keep recovery UI alive and fail visibly.
                lifecycleFailure = "Managed lock-task request failed: ${error.javaClass.simpleName}"
            }
        }
    }
}

@Composable
private fun StudentTheme(content: @Composable () -> Unit) {
    PrimerTheme(darkTheme = true) {
        Surface(Modifier.fillMaxSize(), color = PrimerTheme.colors.surface, content = content)
    }
}

@Composable
private fun StudentScreen(
    activity: MainActivity,
    runtime: StudentRuntime,
    lifecycleFailure: String?,
    deepLink: Uri?,
    onConfigured: () -> Unit,
) {
    var tick by remember { mutableIntStateOf(0) }
    var message by remember { mutableStateOf<String?>(null) }
    var codes by remember { mutableStateOf<List<String>?>(null) }
    var savedCodes by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf(false) }
    var showRecovery by remember { mutableStateOf(false) }
    var recoveryInput by remember { mutableStateOf("") }
    var available by remember { mutableStateOf<List<ApprovedApp>>(emptyList()) }
    var selected by remember { mutableStateOf(runtime.policy.store.apps().map { it.packageName }.toSet()) }
    var size by remember { mutableStateOf("") }
    var digest by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var confirmRemoval by remember { mutableStateOf(false) }
    var enrollmentQr by remember { mutableStateOf("") }
    var replaceEnrollment by remember { mutableStateOf(false) }
    var scanningEnrollment by remember { mutableStateOf(false) }
    var tasksNav by remember { mutableStateOf(TasksDeepLinkRouting.incoming(TasksNavState(), deepLink)) }
    val scope = rememberCoroutineScope()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_PAUSE) recoveryInput = ""
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }

    fun action(block: () -> Unit) {
        try {
            block()
            tick++
        } catch (e: Exception) {
            message = "Operation failed: ${e.message ?: e.javaClass.simpleName}"
        }
    }
    LaunchedEffect(Unit) {
        var wasMaintenance = runtime.policy.inMaintenance
        while (true) {
            delay(1000)
            val maintenance = runtime.policy.inMaintenance
            if (wasMaintenance && !maintenance) {
                codes = null
                savedCodes = false
                editing = false
                confirmRemoval = false
                action { runtime.closeMaintenance(); runtime.policy.enterLockTask(activity) }
            }
            wasMaintenance = maintenance
            tick++
        }
    }
    // Reading tick makes public Android/readback state refresh without pretending it is app-owned.
    @Suppress("UNUSED_VARIABLE") val refresh = tick
    val owner = runtime.policy.isOwner
    val configured = runtime.policy.store.configured
    val maintenance = owner && runtime.policy.inMaintenance
    val setup = owner && !configured
    val enrollmentImagePicker = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        val pairing = runtime.policy.pairingCapability()
        if (!pairing.canImportImage) {
            message = pairing.importMessage
            return@rememberLauncherForActivityResult
        }
        busy = true
        scope.launch {
            try {
                when (val decoded = QrImageImporter(activity.contentResolver).decode(uri)) {
                    is QrImageImporter.Result.Decoded -> {
                        message = runtime.enrollManagement(decoded.payload, replaceEnrollment).message
                        replaceEnrollment = false
                    }
                    is QrImageImporter.Result.Failure -> message = when (decoded.reason) {
                        QrImageImporter.Failure.UNREADABLE -> "Couldn't read that image. Choose another QR image."
                        QrImageImporter.Failure.TOO_LARGE -> "That image is too large to scan safely. Choose a smaller QR image."
                        QrImageImporter.Failure.NO_QR -> "No valid Primer management QR code was found in that image."
                    }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                message = "Management enrollment failed: ${e.message ?: e.javaClass.simpleName}"
            } finally {
                busy = false
                tick++
            }
        }
    }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) {
            busy = true
            scope.launch {
                try {
                    message = withContext(Dispatchers.IO) {
                        runtime.updater.install(uri, size.toLongOrNull() ?: 0, digest.trim()) {
                            runtime.policy.isOwner && runtime.policy.inMaintenance
                        }
                    }
                } catch (e: Exception) {
                    message = "Update could not start: ${e.javaClass.simpleName}"
                } finally {
                    busy = false
                    tick++
                }
            }
        }
    }
    LaunchedEffect(deepLink) {
        tasksNav = TasksDeepLinkRouting.incoming(tasksNav, deepLink)
    }
    if (tasksNav.showTasks) {
        val pending = tasksNav.pendingOccurrenceLink?.let(Uri::parse)
        StudentTasksRoute(
            deepLink = pending,
            onLeave = { tasksNav = TasksDeepLinkRouting.leave(tasksNav) },
            onDeepLinkConsumed = { tasksNav = TasksDeepLinkRouting.consumed(tasksNav) },
            pairing = runtime.policy.pairingCapability(),
            onRequestParentCameraGrant = {
                action {
                    message = runtime.policy.grantCameraForPairing()
                }
            },
            modifier = Modifier.fillMaxSize().windowInsetsPadding(WindowInsets.safeDrawing),
        )
        return
    }
    if (scanningEnrollment && (setup || maintenance)) {
        val pairing = runtime.policy.pairingCapability()
        PayloadQrScanner(
            title = "Scan management enrollment QR",
            onQr = { raw ->
                scanningEnrollment = false
                busy = true
                scope.launch {
                    try {
                        message = runtime.enrollManagement(raw, replaceEnrollment).message
                        replaceEnrollment = false
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        message = "Management enrollment failed: ${e.message ?: e.javaClass.simpleName}"
                    } finally {
                        busy = false
                        tick++
                    }
                }
            },
            onCancel = { scanningEnrollment = false },
            pairing = pairing,
            onRequestParentCameraGrant = {
                action { message = runtime.policy.grantCameraForPairing() }
            },
        )
        return
    }

    LazyColumn(
        Modifier.fillMaxSize().windowInsetsPadding(WindowInsets.safeDrawing),
        contentPadding = PaddingValues(20.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        item {
            PrimerSectionHeader(
                label = "Primer / Student",
                title = "Device qualification",
                description = "Supervised build. Debugging remains enabled. Not certified for unsupervised use.",
            )
        }
        item {
            PrimerRecordRow(
                label = "Device owner",
                value = if (owner) "Primer Student" else "Not enrolled",
                status = if (owner) "ENROLLED" else "UNMANAGED",
                statusTone = if (owner) PrimerStatusTone.Accent else PrimerStatusTone.Attention,
            )
            PrimerRecordRow(label = "Version", value = runtime.updater.version.toString())
            PrimerRecordRow(label = "Policy", value = runtime.policy.store.report)
            PrimerRecordRow(
                label = "Lock task",
                value = when (runtime.policy.lockTaskState) {
                    ActivityManager.LOCK_TASK_MODE_LOCKED -> "LOCKED (managed)"
                    ActivityManager.LOCK_TASK_MODE_PINNED -> "PINNED (not managed)"
                    else -> "Not active"
                },
            )
            PrimerRecordRow(label = "Update", value = runtime.updater.status)
            if (lifecycleFailure != null) PrimerStatus(lifecycleFailure, tone = PrimerStatusTone.Attention)
            if (message != null) PrimerStatus(message!!, tone = PrimerStatusTone.Accent)
        }
        if (!owner) item {
            Text(
                "Install alone does not grant device ownership. Enroll this package with Android's managed-device provisioning. No reset or ownership change is initiated by this screen.",
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.textMuted,
            )
            PrimerButton(text = "Refresh status", onClick = { action { runtime.reconcile() } })
        }
        if (setup) item {
            Text("PARENT SETUP", style = PrimerTheme.typography.label, color = PrimerTheme.colors.textMuted)
            Text(
                "Before enabling kiosk: save one-use recovery codes somewhere off this phone and choose approved apps. Recovery opens a five-minute maintenance window.",
                style = PrimerTheme.typography.body,
            )
            if (!runtime.canScheduleExpiry()) {
                Text(
                    "Allow exact alarms before enabling kiosk so maintenance can expire while another app is open.",
                    style = PrimerTheme.typography.body,
                    color = PrimerTheme.colors.attention,
                )
                PrimerButton(
                    text = "Open alarm permission",
                    onClick = {
                        action {
                            activity.startActivity(
                                Intent(
                                    Settings.ACTION_REQUEST_SCHEDULE_EXACT_ALARM,
                                    Uri.parse("package:${activity.packageName}"),
                                ),
                            )
                        }
                    },
                )
            }
            PrimerButton(
                text = "Generate recovery codes",
                onClick = {
                    action {
                        check(!runtime.policy.store.configured)
                        codes = runtime.recovery.prepareCodes()
                        savedCodes = false
                        available = runtime.policy.availableApps()
                        editing = true
                    }
                },
            )
            Text(
                "Parent-only enrollment. Scan or import the management QR during setup. Pasting the payload is a fallback, not a scan.",
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.textMuted,
            )
            PrimerButton(
                text = "Scan management QR",
                enabled = !busy,
                onClick = {
                    val pairing = runtime.policy.pairingCapability()
                    if (!pairing.canScan) {
                        message = pairing.message
                    } else {
                        scanningEnrollment = true
                    }
                },
            )
            PrimerButton(
                text = "Import management QR image",
                enabled = !busy,
                onClick = {
                    val pairing = runtime.policy.pairingCapability()
                    if (!pairing.canImportImage) {
                        message = pairing.importMessage
                    } else {
                        enrollmentImagePicker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                    }
                },
            )
            PrimerTextField(
                value = enrollmentQr,
                onValueChange = { enrollmentQr = it },
                label = "Paste management enrollment payload (not a scan)",
            )
            PrimerButton(
                text = "Enroll pasted payload (parent only)",
                enabled = !busy,
                onClick = {
                    val raw = enrollmentQr
                    enrollmentQr = ""
                    busy = true
                    scope.launch {
                        try {
                            message = runtime.enrollManagement(raw).message
                        } catch (e: CancellationException) {
                            throw e
                        } catch (e: Exception) {
                            message = "Management enrollment failed: ${e.message ?: e.javaClass.simpleName}"
                        } finally {
                            busy = false
                            tick++
                        }
                    }
                },
            )
        }
        if (codes != null && (setup || maintenance)) item {
            Text("PARENT RECOVERY — STORE OFF DEVICE", style = PrimerTheme.typography.label, color = PrimerTheme.colors.attention)
            codes!!.forEachIndexed { index, code ->
                Text("${index + 1}. $code", style = PrimerTheme.typography.mono)
            }
            Text(
                "Prepared codes are not active yet. Save them off-device, then explicitly activate them. Closing, expiry or reboot discards this preparation; previous unused codes stay valid.",
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.textMuted,
            )
            PrimerCheckboxRow(
                text = "I saved these recovery codes off this device",
                checked = savedCodes,
                onCheckedChange = { savedCodes = it },
            )
        }
        if ((setup && editing) || (maintenance && editing)) {
            item {
                PrimerRule()
                Text("APPROVED APPS", style = PrimerTheme.typography.label, color = PrimerTheme.colors.textMuted)
                Text(
                    "Approve trusted apps only. A browser approval does not restrict its websites. System dependencies still require qualification.",
                    style = PrimerTheme.typography.body,
                    color = PrimerTheme.colors.textMuted,
                )
            }
            items(available.size) { index ->
                val app = available[index]
                PrimerCheckboxRow(
                    text = "${app.label} · ${app.packageName}",
                    checked = app.packageName in selected,
                    onCheckedChange = {
                        selected = if (it) selected + app.packageName else selected - app.packageName
                    },
                )
            }
            item {
                PrimerButton(
                    text = if (setup) "Save codes and enable kiosk" else "Save approved apps",
                    enabled = !busy && (!setup || (codes != null && savedCodes && runtime.canScheduleExpiry())),
                    onClick = {
                        action {
                            check(runtime.policy.isOwner)
                            check(!runtime.policy.store.configured || runtime.policy.inMaintenance)
                            if (!runtime.policy.store.configured) {
                                check(savedCodes) { "Save recovery codes off-device first" }
                                runtime.activateRecoveryCodes(checkNotNull(codes))
                            }
                            check(runtime.recovery.read().verifiers.isNotEmpty()) { "Recovery codes are required" }
                            check(runtime.canScheduleExpiry()) { "Exact expiry alarm support is required" }
                            runtime.policy.store.configure(available.filter { it.packageName in selected })
                            runtime.reconcile()
                            codes = null
                            savedCodes = false
                            editing = false
                            onConfigured()
                            message = "Policy saved. Verify actual lock-task and approved-app behavior."
                        }
                    },
                )
            }
        }
        if (configured) {
            item {
                PrimerRule()
                Text("APPROVED APPLICATIONS", style = PrimerTheme.typography.label, color = PrimerTheme.colors.textMuted)
            }
            val apps = runtime.policy.store.apps()
            items(apps.size) { index ->
                val app = apps[index]
                PrimerButton(text = "Open ${app.label}", onClick = { action { runtime.policy.launchApproved(app) } })
            }
            item {
                PrimerButton(text = "Open Tasks", onClick = { tasksNav = TasksNavState(showTasks = true) })
                Text(
                    "Old Primer Tasks (com.aleksclark.primertasks) pairings cannot be copied. Request a new Student QR, then revoke the old pairing. Tasks failures never clear device owner or recovery.",
                    style = PrimerTheme.typography.body,
                    color = PrimerTheme.colors.textMuted,
                )
                if (!maintenance) {
                    PrimerButton(
                        text = "Parent maintenance",
                        onClick = {
                            recoveryInput = ""
                            showRecovery = !showRecovery
                        },
                    )
                }
            }
        }
        if (configured && !maintenance && showRecovery) item {
            PrimerTextField(
                value = recoveryInput,
                onValueChange = { recoveryInput = it },
                label = "One-use recovery code",
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, autoCorrectEnabled = false),
            )
            PrimerButton(
                text = "Open maintenance",
                onClick = {
                    action {
                        val code = recoveryInput
                        recoveryInput = ""
                        message = runtime.openMaintenance(code)
                        showRecovery = !runtime.policy.inMaintenance
                    }
                },
            )
        }
        if (maintenance) item {
            PrimerRule()
            val seconds = ((runtime.recovery.read().leaseUntilElapsedMs - runtime.recovery.clock().elapsedMs) / 1000).coerceAtLeast(0)
            PrimerStatus("Parent maintenance · ${seconds}s", tone = PrimerStatusTone.Attention)
            Text(
                "Policy returns on timeout or reboot. APK updates require exact size, checksum, package and the same signing key.",
                style = PrimerTheme.typography.body,
            )
            PrimerButton(
                text = "Close maintenance",
                onClick = {
                    action {
                        runtime.closeMaintenance()
                        codes = null
                        editing = false
                        confirmRemoval = false
                    }
                },
            )
            Text(
                "Parent-only enrollment. Scan or import the management QR during maintenance. Pasting the payload is a fallback, not a scan.",
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.textMuted,
            )
            PrimerCheckboxRow(
                text = "Replace existing management enrollment",
                checked = replaceEnrollment,
                onCheckedChange = { replaceEnrollment = it },
            )
            PrimerButton(
                text = "Scan management QR",
                enabled = !busy,
                onClick = {
                    check(runtime.policy.inMaintenance)
                    val pairing = runtime.policy.pairingCapability()
                    if (!pairing.canScan) {
                        message = pairing.message
                    } else {
                        scanningEnrollment = true
                    }
                },
            )
            PrimerButton(
                text = "Import management QR image",
                enabled = !busy,
                onClick = {
                    check(runtime.policy.inMaintenance)
                    val pairing = runtime.policy.pairingCapability()
                    if (!pairing.canImportImage) {
                        message = pairing.importMessage
                    } else {
                        enrollmentImagePicker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                    }
                },
            )
            PrimerTextField(
                value = enrollmentQr,
                onValueChange = { enrollmentQr = it },
                label = "Paste management enrollment payload (not a scan)",
            )
            PrimerButton(
                text = "Enroll pasted payload (parent only)",
                enabled = !busy,
                onClick = {
                    val raw = enrollmentQr
                    enrollmentQr = ""
                    busy = true
                    scope.launch {
                        try {
                            check(runtime.policy.inMaintenance)
                            message = runtime.enrollManagement(raw, replaceEnrollment).message
                            replaceEnrollment = false
                        } catch (e: CancellationException) {
                            throw e
                        } catch (e: Exception) {
                            message = "Management enrollment failed: ${e.message ?: e.javaClass.simpleName}"
                        } finally {
                            busy = false
                            tick++
                        }
                    }
                },
            )
            PrimerButton(
                text = "Edit approved apps",
                onClick = {
                    action {
                        check(runtime.policy.inMaintenance)
                        available = runtime.policy.availableApps()
                        editing = true
                    }
                },
            )
            PrimerButton(
                text = "Grant camera for pairing",
                onClick = { action { message = runtime.policy.grantCameraForPairing() } },
            )
            PrimerButton(text = "Open device settings", onClick = { action { runtime.policy.openSettings() } })
            Text(
                "${runtime.recovery.read().verifiers.size} unused recovery codes remain. Renew before spending the last code.",
                style = PrimerTheme.typography.body,
                color = PrimerTheme.colors.textMuted,
            )
            PrimerButton(
                text = "Prepare replacement recovery codes",
                onClick = {
                    action {
                        check(runtime.policy.inMaintenance)
                        codes = runtime.recovery.prepareCodes()
                        savedCodes = false
                    }
                },
            )
            if (codes != null) {
                PrimerButton(
                    text = "Activate saved recovery codes",
                    enabled = savedCodes,
                    onClick = {
                        action {
                            runtime.activateRecoveryCodes(checkNotNull(codes))
                            codes = null
                            savedCodes = false
                            message = "New recovery codes activated; previous unused codes invalidated."
                        }
                    },
                )
            }
            PrimerTextField(
                value = size,
                onValueChange = { size = it.filter(Char::isDigit) },
                label = "Expected APK size in bytes",
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            )
            PrimerTextField(value = digest, onValueChange = { digest = it }, label = "Expected APK SHA-256")
            PrimerButton(
                text = "Select and install verified APK",
                enabled = !busy && size.toLongOrNull() != null && digest.trim().matches(Regex("[0-9a-fA-F]{64}")),
                onClick = {
                    action {
                        check(runtime.policy.inMaintenance)
                        runtime.policy.reconcile()
                        picker.launch(arrayOf("*/*"))
                    }
                },
            )
            Text("Recent recovery audit (last 200 events)", style = PrimerTheme.typography.label, color = PrimerTheme.colors.textMuted)
            runtime.recovery.audit().takeLast(6).forEach {
                Text(it, style = PrimerTheme.typography.mono, color = PrimerTheme.colors.textMuted)
            }
            PrimerButton(
                text = "End qualification / remove owner",
                variant = PrimerButtonVariant.Attention,
                onClick = { confirmRemoval = true },
            )
            if (confirmRemoval) {
                Text(
                    "This removes Primer device ownership and its restrictions without erasing the phone, if Android permits it. This is not ownership transfer.",
                    style = PrimerTheme.typography.body,
                    color = PrimerTheme.colors.attention,
                )
                PrimerButton(
                    text = "Confirm remove device owner",
                    variant = PrimerButtonVariant.Attention,
                    onClick = {
                        action {
                            runtime.policy.removeOwnership(activity)
                            codes = null
                            editing = false
                            confirmRemoval = false
                            message = "Device ownership removed; phone is unmanaged."
                        }
                    },
                )
            }
        }
    }
}
