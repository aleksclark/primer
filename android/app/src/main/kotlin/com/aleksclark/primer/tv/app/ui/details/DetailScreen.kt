package com.aleksclark.primer.tv.app.ui.details

import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import com.aleksclark.primer.tv.app.ui.components.MediaDetailPane
import com.aleksclark.primer.tv.app.ui.components.MediaDetailUnavailable
import com.aleksclark.primer.tv.core.domain.CatalogCard
import com.aleksclark.primer.tv.core.presentation.DetailPresenter
import com.aleksclark.primer.tv.core.presentation.MediaDetailModel

/**
 * Details destination. Builds a [MediaDetailModel] from the catalog card and
 * hands layout to [MediaDetailPane].
 */
@Composable
fun DetailScreen(
    card: CatalogCard?,
    baseUrl: String,
    onPlay: () -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    resumable: Boolean = false,
    showBackButton: Boolean = true,
) {
    if (card == null) {
        MediaDetailUnavailable(
            onBack = onBack,
            modifier = modifier,
            showBackButton = showBackButton,
        )
        return
    }

    val detail = remember(card, resumable) {
        DetailPresenter.present(card = card, resumable = resumable)
    }

    MediaDetailPane(
        detail = detail,
        baseUrl = baseUrl,
        onPlay = {
            // Belt-and-braces: watched entertainment must never fire play.
            if (detail.primaryEnabled) onPlay()
        },
        onBack = onBack,
        modifier = modifier,
        showBackButton = showBackButton,
    )
}
