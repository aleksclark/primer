package com.aleksclark.primer.control.tasks

import com.aleksclark.primertasks.client.TasksHttpException

enum class ControlLoadState { Loading, Ready, Empty, Denied, Conflict, Error }

fun controlState(error: Throwable?): ControlLoadState = when (error) {
    is TasksHttpException -> when {
        error.statusCode == 403 || error.code == "denied" -> ControlLoadState.Denied
        error.statusCode == 409 || error.code == "conflict" -> ControlLoadState.Conflict
        else -> ControlLoadState.Error
    }
    null -> ControlLoadState.Ready
    else -> ControlLoadState.Error
}

fun controlMessage(error: Throwable): String = when (error) {
    is TasksHttpException -> when (error.statusCode) {
        401 -> "Sign in again. The parent session is missing or expired."
        403 -> "This household does not include your account."
        409 -> "The record changed. Refresh and try again."
        else -> error.detail ?: error.message ?: "The Tasks service could not complete that request."
    }
    else -> "The Tasks service could not complete that request."
}
