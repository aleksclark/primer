package com.aleksclark.primer.student.update

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.aleksclark.primer.student.StudentRuntime

class InstallResultReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        StudentRuntime(context).updater.handleResult(intent)
    }
}
