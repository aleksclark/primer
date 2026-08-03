package com.aleksclark.primer.tv.app.ui

/**
 * Legacy layout constants retained only where a caller still needs a coarse
 * phone-vs-TV size bag. Prefer [com.aleksclark.primer.tv.app.ui.designsystem.PrimerTheme]
 * spacing/typography tokens for new UI.
 *
 * Pairing, Catalog, Settings, Channel, Guide, Details, and Player bodies now
 * live under their feature packages.
 */
data class ShellMetrics(
    val screenPadding: Int,
    val titleSize: Int,
    val bodySize: Int,
    val cardWidth: Int,
) {
    companion object {
        val Tablet = ShellMetrics(screenPadding = 24, titleSize = 28, bodySize = 16, cardWidth = 180)

        /** Wide enough to clear TV overscan, with 10-foot legible type. */
        val Television = ShellMetrics(screenPadding = 48, titleSize = 40, bodySize = 22, cardWidth = 260)
    }
}
