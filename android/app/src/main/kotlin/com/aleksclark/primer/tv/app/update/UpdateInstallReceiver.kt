package com.aleksclark.primer.tv.app.update

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.aleksclark.primer.tv.app.TvApplication

class UpdateInstallReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val app = context.applicationContext as? TvApplication ?: return
        app.container.updater?.handleResult(intent)
    }
}
