package com.aleksclark.primer.tv.app.update

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import java.io.File

class UpdateInstallReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION_INSTALL_RESULT) return
        when (intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_SUCCESS -> apk(intent)?.delete()
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirmation = intent.getParcelableExtra<Intent>(Intent.EXTRA_INTENT)
                confirmation?.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                if (confirmation != null) context.startActivity(confirmation)
                else apk(intent)?.let { InteractiveApkInstaller.launch(context, it) }
            }
            else -> Unit
        }
    }

    private fun apk(intent: Intent): File? =
        intent.getStringExtra(EXTRA_APK_PATH)?.let(::File)?.takeIf(File::exists)

    companion object {
        const val ACTION_INSTALL_RESULT = "com.aleksclark.primer.tv.action.INSTALL_RESULT"
        const val EXTRA_APK_PATH = "apk_path"
    }
}
