package com.aleksclark.primer.tv.core.domain

/** Which shell the app presents. */
enum class FormFactor {
    /** Touch-first Material3 UI. */
    TABLET,

    /** D-pad leanback UI built on androidx.tv. */
    TELEVISION,
    ;

    companion object {
        /**
         * `Configuration.UI_MODE_TYPE_TELEVISION`. Hardcoded so the decision is
         * testable on the JVM without the Android framework on the classpath.
         */
        const val UI_MODE_TYPE_TELEVISION = 4

        /**
         * Picks the shell from `UiModeManager.currentModeType`, per the plan.
         *
         * Only an explicit television mode selects leanback: the RK3318 box
         * reports television, while a tablet reports normal, and anything
         * unexpected (appliance, watch, undefined) is safer on the touch UI,
         * which still works with a D-pad.
         */
        fun fromUiModeType(uiModeType: Int): FormFactor =
            if (uiModeType == UI_MODE_TYPE_TELEVISION) TELEVISION else TABLET
    }
}
