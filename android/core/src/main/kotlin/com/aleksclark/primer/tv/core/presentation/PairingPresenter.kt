package com.aleksclark.primer.tv.core.presentation

/**
 * Pairing form presentation helpers kept free of Android so uppercase
 * normalization and submit gating stay unit-testable.
 */
object PairingPresenter {

    /** Codes are drawn from an uppercase alphabet and compared case-sensitively. */
    fun normalizeCode(raw: String): String = raw.uppercase()

    fun canSubmit(baseUrl: String, code: String, submitting: Boolean): Boolean =
        !submitting && baseUrl.isNotBlank() && code.isNotBlank()

    fun failureMessage(detail: String?): UiText = UiText.of(
        detail?.takeIf { it.isNotBlank() }
            ?: "That pairing code is invalid or expired. Request a new code and try again.",
    )

    const val INLINE_ERROR_FALLBACK =
        "That pairing code is invalid or expired. Request a new code and try again."
}
