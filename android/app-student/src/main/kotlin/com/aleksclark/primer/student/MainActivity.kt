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
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.selection.toggleable
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.DisposableEffect
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.aleksclark.primer.designsystem.PrimerTokens
import com.aleksclark.primer.devicepolicy.ApprovedApp
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

open class MainActivity : ComponentActivity() {
    protected val runtime by lazy { StudentRuntime(this) }
    protected open val enforceKiosk = true
    private var lifecycleFailure by mutableStateOf<String?>(null)
    private var lockVerification: Job? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Recovery material must never appear in screenshots, recents thumbnails or recordings.
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        setContent {
            StudentTheme { StudentScreen(this, runtime, lifecycleFailure) { onParentSetupComplete() } }
        }
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
    val c = PrimerTokens.Dark
    MaterialTheme(colorScheme = darkColorScheme(
        primary = c.accent, onPrimary = c.onAccent, background = c.surface,
        surface = c.surface, onSurface = c.text, onBackground = c.text,
        error = c.attention, outline = c.ruleStrong,
    )) { Surface(Modifier.fillMaxSize(), content = content) }
}

@Composable
private fun StudentScreen(activity: MainActivity, runtime: StudentRuntime, lifecycleFailure: String?, onConfigured: () -> Unit) {
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
        try { block(); tick++ } catch (e: Exception) {
            message = "Operation failed: ${e.message ?: e.javaClass.simpleName}"
        }
    }
    LaunchedEffect(Unit) {
        var wasMaintenance = runtime.policy.inMaintenance
        while (true) {
            delay(1000)
            val maintenance = runtime.policy.inMaintenance
            if (wasMaintenance && !maintenance) {
                codes = null; savedCodes = false; editing = false; confirmRemoval = false
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
                } catch (e: Exception) { message = "Update could not start: ${e.javaClass.simpleName}" }
                finally { busy = false; tick++ }
            }
        }
    }

    LazyColumn(Modifier.fillMaxSize().windowInsetsPadding(WindowInsets.safeDrawing), contentPadding = PaddingValues(20.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)) {
        item {
            Text("PRIMER / STUDENT", fontFamily = FontFamily.Monospace, fontSize = 13.sp, color = PrimerTokens.Dark.accent)
            Text("Device qualification", fontSize = 26.sp)
            Text("Supervised build. Debugging remains enabled. Not certified for unsupervised use.",
                color = PrimerTokens.Dark.attention)
        }
        item {
            Record("DEVICE OWNER", if (owner) "Primer Student" else "Not enrolled")
            Record("VERSION", runtime.updater.version.toString())
            Record("POLICY", runtime.policy.store.report)
            Record("LOCK TASK", when (runtime.policy.lockTaskState) {
                ActivityManager.LOCK_TASK_MODE_LOCKED -> "LOCKED (managed)"
                ActivityManager.LOCK_TASK_MODE_PINNED -> "PINNED (not managed)"
                else -> "Not active"
            })
            Record("UPDATE", runtime.updater.status)
            if (lifecycleFailure != null) Text(lifecycleFailure, color = PrimerTokens.Dark.attention)
            if (message != null) Text(message!!, color = PrimerTokens.Dark.accent)
        }
        if (!owner) item {
            Text("Install alone does not grant device ownership. Enroll this package with Android's managed-device provisioning. No reset or ownership change is initiated by this screen.")
            Action("Refresh status") { action { runtime.reconcile() } }
        }
        if (setup) item {
            Text("PARENT SETUP", fontFamily = FontFamily.Monospace)
            Text("Before enabling kiosk: save one-use recovery codes somewhere off this phone and choose approved apps. Recovery opens a five-minute maintenance window.")
            if (!runtime.canScheduleExpiry()) {
                Text("Allow exact alarms before enabling kiosk so maintenance can expire while another app is open.")
                Action("Open alarm permission") { action {
                    activity.startActivity(Intent(Settings.ACTION_REQUEST_SCHEDULE_EXACT_ALARM,
                        Uri.parse("package:${activity.packageName}")))
                } }
            }
            Action("Generate recovery codes") { action {
                check(!runtime.policy.store.configured)
                codes = runtime.recovery.prepareCodes(); savedCodes = false
                available = runtime.policy.availableApps(); editing = true
            } }
        }
        if (codes != null && (setup || maintenance)) item {
            Text("PARENT RECOVERY — STORE OFF DEVICE", fontFamily = FontFamily.Monospace)
            codes!!.forEachIndexed { index, code ->
                Text("${index + 1}. $code", fontFamily = FontFamily.Monospace, fontSize = 12.sp)
            }
            Text("Prepared codes are not active yet. Save them off-device, then explicitly activate them. Closing, expiry or reboot discards this preparation; previous unused codes stay valid.")
            Check("I saved these recovery codes off this device", savedCodes) { savedCodes = it }
        }
        if ((setup && editing) || (maintenance && editing)) {
            item {
                HorizontalDivider()
                Text("APPROVED APPS", fontFamily = FontFamily.Monospace)
                Text("Approve trusted apps only. A browser approval does not restrict its websites. System dependencies still require qualification.")
            }
            items(available.size) { index ->
                val app = available[index]
                Check("${app.label} · ${app.packageName}", app.packageName in selected) {
                    selected = if (it) selected + app.packageName else selected - app.packageName
                }
            }
            item {
                Action(if (setup) "Save codes and enable kiosk" else "Save approved apps",
                    !busy && (!setup || (codes != null && savedCodes && runtime.canScheduleExpiry()))) {
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
                        codes = null; savedCodes = false; editing = false
                        onConfigured()
                        message = "Policy saved. Verify actual lock-task and approved-app behavior."
                    }
                }
            }
        }
        if (configured) {
            item { HorizontalDivider(); Text("APPROVED APPLICATIONS", fontFamily = FontFamily.Monospace) }
            val apps = runtime.policy.store.apps()
            items(apps.size) { index ->
                val app = apps[index]
                Action("Open ${app.label}") { action { runtime.policy.launchApproved(app) } }
            }
            item {
                Text("Tasks pairing and educational screens arrive in the next slice; this is the device-control qualification build.")
                if (!maintenance) Action("Parent maintenance") {
                    recoveryInput = ""
                    showRecovery = !showRecovery
                }
            }
        }
        if (configured && !maintenance && showRecovery) item {
            OutlinedTextField(recoveryInput, { recoveryInput = it }, label = { Text("One-use recovery code") },
                visualTransformation = PasswordVisualTransformation(), singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, autoCorrectEnabled = false),
                shape = RectangleShape, modifier = Modifier.fillMaxWidth())
            Action("Open maintenance") { action {
                val code = recoveryInput; recoveryInput = ""
                message = runtime.openMaintenance(code)
                showRecovery = !runtime.policy.inMaintenance
            } }
        }
        if (maintenance) item {
            HorizontalDivider()
            val seconds = ((runtime.recovery.read().leaseUntilElapsedMs - runtime.recovery.clock().elapsedMs) / 1000).coerceAtLeast(0)
            Text("PARENT MAINTENANCE · ${seconds}s", fontFamily = FontFamily.Monospace, color = PrimerTokens.Dark.attention)
            Text("Policy returns on timeout or reboot. APK updates require exact size, checksum, package and the same signing key.")
            Action("Close maintenance") { action { runtime.closeMaintenance(); codes = null; editing = false; confirmRemoval = false } }
            Action("Edit approved apps") { action {
                check(runtime.policy.inMaintenance)
                available = runtime.policy.availableApps(); editing = true
            } }
            Action("Open device settings") { action { runtime.policy.openSettings() } }
            Text("${runtime.recovery.read().verifiers.size} unused recovery codes remain. Renew before spending the last code.")
            Action("Prepare replacement recovery codes") { action {
                check(runtime.policy.inMaintenance)
                codes = runtime.recovery.prepareCodes(); savedCodes = false
            } }
            if (codes != null) Action("Activate saved recovery codes", savedCodes) { action {
                runtime.activateRecoveryCodes(checkNotNull(codes))
                codes = null; savedCodes = false
                message = "New recovery codes activated; previous unused codes invalidated."
            } }
            OutlinedTextField(size, { size = it.filter(Char::isDigit) }, label = { Text("Expected APK size in bytes") },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), shape = RectangleShape, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(digest, { digest = it }, label = { Text("Expected APK SHA-256") }, shape = RectangleShape, modifier = Modifier.fillMaxWidth())
            Action("Select and install verified APK", !busy && size.toLongOrNull() != null && digest.trim().matches(Regex("[0-9a-fA-F]{64}"))) {
                action { check(runtime.policy.inMaintenance); runtime.policy.reconcile(); picker.launch(arrayOf("*/*")) }
            }
            Text("Recent recovery audit (last 200 events)")
            runtime.recovery.audit().takeLast(6).forEach { Text(it, fontFamily = FontFamily.Monospace, fontSize = 11.sp) }
            Action("End qualification / remove owner") { confirmRemoval = true }
            if (confirmRemoval) {
                Text("This removes Primer device ownership and its restrictions without erasing the phone, if Android permits it. This is not ownership transfer.")
                Action("Confirm remove device owner") { action {
                    runtime.policy.removeOwnership(activity)
                    codes = null; editing = false; confirmRemoval = false
                    message = "Device ownership removed; phone is unmanaged."
                } }
            }
        }
    }
}

@Composable
private fun Record(label: String, value: String) {
    Column(Modifier.fillMaxWidth().padding(vertical = 5.dp)) {
        Text(label, fontFamily = FontFamily.Monospace, fontSize = 11.sp, color = PrimerTokens.Dark.textMuted)
        Text(value, fontSize = 14.sp)
    }
}

@Composable
private fun Action(label: String, enabled: Boolean = true, click: () -> Unit) {
    Button(click, enabled = enabled, shape = RectangleShape,
        border = BorderStroke(1.dp, PrimerTokens.Dark.ruleStrong),
        elevation = ButtonDefaults.buttonElevation(0.dp, 0.dp, 0.dp, 0.dp, 0.dp),
        modifier = Modifier.fillMaxWidth()) { Text(label) }
}

@Composable
private fun Check(label: String, checked: Boolean, change: (Boolean) -> Unit) {
    // Expose both the label and checked state on a single touch/accessibility target.
    Row(Modifier.fillMaxWidth().toggleable(value = checked, role = Role.Checkbox, onValueChange = change)
        .padding(vertical = 8.dp)) {
        Checkbox(checked, onCheckedChange = null)
        Text(label, modifier = Modifier.weight(1f))
    }
}
