package com.aleksclark.primer.control

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

class ControlSelfUpdateReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != com.aleksclark.primer.updates.SelfUpdateSession.ACTION_RESULT) return
        val app = context.applicationContext as? ControlApp ?: return
        app.updater.handleResult(intent)
    }
}
