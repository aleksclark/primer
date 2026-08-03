package com.aleksclark.primer.tv.app.ui

import com.aleksclark.primer.tv.core.domain.Catalog
import com.aleksclark.primer.tv.core.domain.CatalogCard
import com.aleksclark.primer.tv.core.domain.CatalogView
import com.aleksclark.primer.tv.core.presentation.PairingPresenter

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
     * gate is that both fields carry something. Delegates to [PairingPresenter]
     * so UI and JVM tests share one rule.
     */
    val canSubmit: Boolean
        get() = PairingPresenter.canSubmit(
            baseUrl = baseUrlInput,
            code = codeInput,
            submitting = submitting,
        )
}

/** Transient non-blocking feedback shown as a banner/snackbar. */
data class StatusMessage(
    val text: String,
    val isError: Boolean = false,
)

/** State of the catalog screen. */
data class CatalogUiState(
    val loading: Boolean = false,
    val view: CatalogView? = null,
    val error: String? = null,
    /** Raw catalog retained so Home can re-present after consume marks. */
    val catalog: Catalog? = null,
) {
    val isEmpty: Boolean get() = !loading && error == null && (view == null || view.isEmpty)

    fun card(mediaItemId: String): CatalogCard? =
        view?.cards?.firstOrNull { it.entry.mediaItemId == mediaItemId }
}
