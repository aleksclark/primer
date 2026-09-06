package com.aleksclark.primer.control

import android.app.Activity
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleOwner

sealed class UserActionPresentation {
    data object ShownOnActivity : UserActionPresentation()
    data object Notified : UserActionPresentation()
    data class Deferred(val reason: String) : UserActionPresentation()

    val shown: Boolean
        get() = this is ShownOnActivity || this is Notified
}

data class UserActionAttempt(
    val hasResumedActivity: Boolean,
    val activityStarted: Boolean? = null,
    val notificationPermissionGranted: Boolean,
    val notificationsEnabled: Boolean,
    val channelEnabled: Boolean,
    val notificationPosted: Boolean? = null,
)

fun interface ControlUserActionPresenter {
    fun present(confirmation: Intent): UserActionPresentation
}

object UserActionDispatch {
    fun outcome(attempt: UserActionAttempt): UserActionPresentation {
        if (attempt.hasResumedActivity && attempt.activityStarted == true) {
            return UserActionPresentation.ShownOnActivity
        }
        if (!attempt.notificationPermissionGranted) {
            return UserActionPresentation.Deferred("Notification permission is denied")
        }
        if (!attempt.notificationsEnabled) {
            return UserActionPresentation.Deferred("Notifications are disabled")
        }
        if (!attempt.channelEnabled) {
            return UserActionPresentation.Deferred("Update notifications are disabled")
        }
        if (attempt.notificationPosted == true) {
            return UserActionPresentation.Notified
        }
        return UserActionPresentation.Deferred("Install confirmation was not shown")
    }
}

class AndroidUserActionPresenter(
    private val context: Context,
    private val resumedActivity: () -> Activity?,
) : ControlUserActionPresenter {
    override fun present(confirmation: Intent): UserActionPresentation {
        val activity = resumedActivity()?.takeIf { it.isResumedForInstall() }
        val started = if (activity != null) {
            runCatching {
                activity.startActivity(confirmation)
                true
            }.getOrDefault(false)
        } else {
            null
        }
        if (started == true) {
            return UserActionDispatch.outcome(
                UserActionAttempt(
                    hasResumedActivity = true,
                    activityStarted = true,
                    notificationPermissionGranted = true,
                    notificationsEnabled = true,
                    channelEnabled = true,
                    notificationPosted = null,
                ),
            )
        }
        val manager = context.getSystemService(NotificationManager::class.java)
        val permissionGranted = notificationPermissionGranted()
        val notificationsEnabled = manager?.areNotificationsEnabled() == true
        val channelEnabled = manager?.let { notificationsEnabled && ensureChannel(it) } == true
        val posted = if (permissionGranted && notificationsEnabled && channelEnabled) {
            post(manager!!, confirmation)
        } else {
            false
        }
        return UserActionDispatch.outcome(
            UserActionAttempt(
                hasResumedActivity = activity != null,
                activityStarted = started,
                notificationPermissionGranted = permissionGranted,
                notificationsEnabled = notificationsEnabled,
                channelEnabled = channelEnabled,
                notificationPosted = posted,
            ),
        )
    }

    private fun notificationPermissionGranted(): Boolean {
        if (Build.VERSION.SDK_INT < 33) return true
        return context.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
    }

    private fun ensureChannel(manager: NotificationManager): Boolean {
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL, "Control updates", NotificationManager.IMPORTANCE_HIGH),
        )
        val channel = manager.getNotificationChannel(CHANNEL) ?: return false
        return channel.importance != NotificationManager.IMPORTANCE_NONE
    }

    private fun post(manager: NotificationManager, confirmation: Intent): Boolean = runCatching {
        val pending = PendingIntent.getActivity(
            context,
            1,
            confirmation,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        manager.notify(
            41,
            Notification.Builder(context, CHANNEL)
                .setSmallIcon(android.R.drawable.stat_sys_download_done)
                .setContentTitle("Confirm Control update")
                .setContentText("Android needs approval to replace Primer Control.")
                .setContentIntent(pending)
                .setAutoCancel(true)
                .build(),
        )
        manager.areNotificationsEnabled()
    }.getOrDefault(false)

    private fun Activity.isResumedForInstall(): Boolean {
        if (isFinishing || isDestroyed) return false
        val owner = this as? LifecycleOwner ?: return false
        return owner.lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)
    }

    companion object {
        const val CHANNEL = "control-self-update"
    }
}
