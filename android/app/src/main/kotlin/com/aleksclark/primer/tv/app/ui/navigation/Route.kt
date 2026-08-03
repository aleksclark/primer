package com.aleksclark.primer.tv.app.ui.navigation

/**
 * Top-level destinations exposed by the persistent navigation chrome.
 * Settings is intentionally last so TV focus order reaches content first.
 */
enum class TopLevelDestination {
    HOME,
    CHANNEL,
    GUIDE,
    SETTINGS,
    ;

    val label: String
        get() = when (this) {
            HOME -> "Home"
            CHANNEL -> "Channel"
            GUIDE -> "Guide"
            SETTINGS -> "Settings"
        }
}

/** Typed app routes. Details and Player are children, never nav-rail items. */
sealed interface Route {
    data class TopLevel(val destination: TopLevelDestination) : Route
    data class Details(val mediaItemId: String, val origin: TopLevelDestination) : Route
    data class Player(val playbackId: String, val returnTo: Route? = null) : Route
    data object Pairing : Route
}

/**
 * Navigation coordinator that keeps origin destinations for back restoration.
 * Pure and unit-testable; the ViewModel adapts existing Destination values to it.
 */
data class NavigationState(
    val route: Route = Route.TopLevel(TopLevelDestination.HOME),
    val topLevel: TopLevelDestination = TopLevelDestination.HOME,
) {
    val showsChrome: Boolean
        get() = route is Route.TopLevel

    fun selectTopLevel(destination: TopLevelDestination): NavigationState =
        copy(route = Route.TopLevel(destination), topLevel = destination)

    fun openDetails(mediaItemId: String, origin: TopLevelDestination = topLevel): NavigationState =
        copy(route = Route.Details(mediaItemId = mediaItemId, origin = origin), topLevel = origin)

    fun openPlayer(playbackId: String, returnTo: Route = route): NavigationState =
        copy(route = Route.Player(playbackId = playbackId, returnTo = returnTo))

    fun openPairing(): NavigationState = copy(route = Route.Pairing)

    /**
     * Pops one level. Returns null when the shell should exit (root Home, or
     * unpaired pairing screen).
     */
    fun back(isPaired: Boolean = true): NavigationState? = when (val current = route) {
        is Route.TopLevel -> when (current.destination) {
            TopLevelDestination.HOME -> null
            else -> selectTopLevel(TopLevelDestination.HOME)
        }

        is Route.Details -> selectTopLevel(current.origin)

        is Route.Player -> when (val returnTo = current.returnTo) {
            is Route.Details -> copy(route = returnTo, topLevel = returnTo.origin)
            is Route.TopLevel -> selectTopLevel(returnTo.destination)
            is Route.Player -> copy(route = returnTo)
            Route.Pairing -> openPairing()
            null -> selectTopLevel(topLevel)
        }

        Route.Pairing -> if (isPaired) selectTopLevel(TopLevelDestination.HOME) else null
    }
}
