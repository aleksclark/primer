package com.aleksclark.primer.tv.app.ui.navigation

import com.aleksclark.primer.tv.app.ui.Destination

/**
 * Temporary bridge between the legacy flat [Destination] enum used by
 * TvViewModel and the Phase A [Route] model. Later phases can retire the enum.
 */
fun Destination.toRoute(
    selectedMediaItemId: String?,
    topLevelHint: TopLevelDestination = TopLevelDestination.HOME,
): Route = when (this) {
    Destination.PAIRING -> Route.Pairing
    Destination.CATALOG -> Route.TopLevel(TopLevelDestination.HOME)
    Destination.CHANNEL -> Route.TopLevel(TopLevelDestination.CHANNEL)
    Destination.EPG -> Route.TopLevel(TopLevelDestination.GUIDE)
    Destination.SETTINGS -> Route.TopLevel(TopLevelDestination.SETTINGS)
    Destination.DETAIL -> Route.Details(
        mediaItemId = selectedMediaItemId.orEmpty(),
        origin = topLevelHint,
    )
    Destination.PLAYER -> Route.Player(
        playbackId = selectedMediaItemId.orEmpty(),
        returnTo = Route.TopLevel(topLevelHint),
    )
}

fun Route.toLegacyDestination(): Destination = when (this) {
    Route.Pairing -> Destination.PAIRING
    is Route.TopLevel -> when (destination) {
        TopLevelDestination.HOME -> Destination.CATALOG
        TopLevelDestination.CHANNEL -> Destination.CHANNEL
        TopLevelDestination.GUIDE -> Destination.EPG
        TopLevelDestination.SETTINGS -> Destination.SETTINGS
    }
    is Route.Details -> Destination.DETAIL
    is Route.Player -> Destination.PLAYER
}

fun Destination.toTopLevelOrNull(): TopLevelDestination? = when (this) {
    Destination.CATALOG -> TopLevelDestination.HOME
    Destination.CHANNEL -> TopLevelDestination.CHANNEL
    Destination.EPG -> TopLevelDestination.GUIDE
    Destination.SETTINGS -> TopLevelDestination.SETTINGS
    Destination.PAIRING, Destination.DETAIL, Destination.PLAYER -> null
}
