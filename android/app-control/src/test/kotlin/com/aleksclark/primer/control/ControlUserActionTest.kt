package com.aleksclark.primer.control

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ControlUserActionTest {
    @Test
    fun resumedActivityStartIsTheOnlyForegroundSuccess() {
        val shown = UserActionDispatch.outcome(
            UserActionAttempt(
                hasResumedActivity = true,
                activityStarted = true,
                notificationPermissionGranted = false,
                notificationsEnabled = false,
                channelEnabled = false,
            ),
        )
        assertEquals(UserActionPresentation.ShownOnActivity, shown)
        assertTrue(shown.shown)
    }

    @Test
    fun startActivityWithoutResumedActivityIsNotPresentation() {
        val background = UserActionDispatch.outcome(
            UserActionAttempt(
                hasResumedActivity = false,
                activityStarted = true,
                notificationPermissionGranted = false,
                notificationsEnabled = false,
                channelEnabled = false,
            ),
        )
        assertEquals(UserActionPresentation.Deferred("Notification permission is denied"), background)
        assertFalse(background.shown)
    }

    @Test
    fun silentStartActivityIsNotPresentation() {
        val blocked = UserActionDispatch.outcome(
            UserActionAttempt(
                hasResumedActivity = false,
                activityStarted = null,
                notificationPermissionGranted = true,
                notificationsEnabled = true,
                channelEnabled = true,
                notificationPosted = false,
            ),
        )
        assertEquals(UserActionPresentation.Deferred("Install confirmation was not shown"), blocked)
        assertFalse(blocked.shown)
    }

    @Test
    fun notificationDeniedIsObservable() {
        val denied = UserActionDispatch.outcome(
            UserActionAttempt(
                hasResumedActivity = false,
                notificationPermissionGranted = false,
                notificationsEnabled = true,
                channelEnabled = true,
            ),
        )
        assertEquals(UserActionPresentation.Deferred("Notification permission is denied"), denied)
        assertFalse(denied.shown)
    }

    @Test
    fun disabledNotificationsAndChannelAreDeferred() {
        assertEquals(
            UserActionPresentation.Deferred("Notifications are disabled"),
            UserActionDispatch.outcome(
                UserActionAttempt(
                    hasResumedActivity = false,
                    notificationPermissionGranted = true,
                    notificationsEnabled = false,
                    channelEnabled = true,
                ),
            ),
        )
        assertEquals(
            UserActionPresentation.Deferred("Update notifications are disabled"),
            UserActionDispatch.outcome(
                UserActionAttempt(
                    hasResumedActivity = false,
                    notificationPermissionGranted = true,
                    notificationsEnabled = true,
                    channelEnabled = false,
                ),
            ),
        )
    }

    @Test
    fun postedNotificationIsFallbackPresentation() {
        val notified = UserActionDispatch.outcome(
            UserActionAttempt(
                hasResumedActivity = false,
                notificationPermissionGranted = true,
                notificationsEnabled = true,
                channelEnabled = true,
                notificationPosted = true,
            ),
        )
        assertEquals(UserActionPresentation.Notified, notified)
        assertTrue(notified.shown)
    }
}
