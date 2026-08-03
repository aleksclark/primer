package com.aleksclark.primer.tv.core.presentation

/**
 * Builds the Settings surface model from device identity and update status.
 *
 * Keeps section copy and update messaging out of Compose so pairing/settings
 * BDD scenarios stay executable as JVM tests.
 */
object SettingsPresenter {

    /**
     * @param deviceName paired display name, or null when unknown
     * @param deviceKind wire kind (`tv_box`, `tablet`, …)
     * @param deviceId server-assigned id when paired
     * @param serverUrl configured base URL
     * @param installedVersionCode currently running versionCode
     * @param updateStatus simplified update pipeline status
     * @param availableVersionCode set when [updateStatus] is [UpdateStatus.AVAILABLE]
     * @param failureMessage set when [updateStatus] is [UpdateStatus.FAILED]
     */
    fun present(
        deviceName: String?,
        deviceKind: String?,
        deviceId: String?,
        serverUrl: String?,
        installedVersionCode: Int,
        updateStatus: UpdateStatus,
        availableVersionCode: Int = 0,
        failureMessage: String? = null,
    ): SettingsUiModel = SettingsUiModel(
        deviceName = deviceName?.takeIf { it.isNotBlank() } ?: "Unnamed device",
        deviceKind = humanKind(deviceKind),
        deviceId = deviceId?.takeIf { it.isNotBlank() },
        serverUrl = serverUrl?.takeIf { it.isNotBlank() } ?: "Not set",
        installedVersionLabel = "Installed version $installedVersionCode",
        update = presentUpdate(
            installedVersionCode = installedVersionCode,
            status = updateStatus,
            availableVersionCode = availableVersionCode,
            failureMessage = failureMessage,
        ),
    )

    fun presentUpdate(
        installedVersionCode: Int,
        status: UpdateStatus,
        availableVersionCode: Int = 0,
        failureMessage: String? = null,
    ): UpdateCardModel = when (status) {
        UpdateStatus.UP_TO_DATE -> UpdateCardModel.UpToDate()
        UpdateStatus.AVAILABLE -> UpdateCardModel.Available(
            versionCode = availableVersionCode,
            statusLabel = UiText.of(
                "Version $availableVersionCode is available (installed $installedVersionCode).",
            ),
        )
        UpdateStatus.DOWNLOADING -> UpdateCardModel.Downloading()
        UpdateStatus.INSTALLING -> UpdateCardModel.Installing()
        UpdateStatus.FAILED -> UpdateCardModel.Failed(
            statusLabel = UiText.of(
                failureMessage?.takeIf { it.isNotBlank() }
                    ?: "The update could not be applied. Try again.",
            ),
        )
    }

    private fun humanKind(kind: String?): String = when (kind?.lowercase()) {
        "tv_box", "tv", "television" -> "Television"
        "tablet" -> "Tablet"
        "phone" -> "Phone"
        null, "" -> "Unknown"
        else -> kind.replace('_', ' ')
            .replaceFirstChar { if (it.isLowerCase()) it.titlecase() else it.toString() }
    }
}

/** Wire-free update pipeline phases for [SettingsPresenter]. */
enum class UpdateStatus {
    UP_TO_DATE,
    AVAILABLE,
    DOWNLOADING,
    INSTALLING,
    FAILED,
}
