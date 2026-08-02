package com.aleksclark.primer.tv.app.ui

import com.aleksclark.primer.tv.core.domain.CatalogCard
import com.aleksclark.primer.tv.core.domain.CatalogView

/** Which top-level destination the shell shows. */
enum class Destination {
    PAIRING,
    CATALOG,
    CHANNEL,
    EPG,
    DETAIL,
    PLAYER,
    SETTINGS,
}

/** State of the pairing screen. */
data class PairingUiState(
    val baseUrlInput: String = "",
    val codeInput: String = "",
    val submitting: Boolean = false,
    val error: String? = null,
) {
    /**
     * Pairing codes are short and the parent reads them off a TV, so the only
     * gate is that both fields carry something.
     */
    val canSubmit: Boolean
        get() = !submitting && baseUrlInput.isNotBlank() && codeInput.isNotBlank()
}

/** State of the catalog screen. */
data class CatalogUiState(
    val loading: Boolean = false,
    val view: CatalogView? = null,
    val error: String? = null,
) {
    val isEmpty: Boolean get() = !loading && error == null && (view == null || view.isEmpty)

    fun card(mediaItemId: String): CatalogCard? =
        view?.cards?.firstOrNull { it.entry.mediaItemId == mediaItemId }
}
