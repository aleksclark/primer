package com.aleksclark.primer.tv.app.ui.navigation

import com.aleksclark.primer.tv.app.ui.Destination
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class NavigationStateTest {

    @Test
    fun `selecting top-level destinations updates route and selected chrome`() {
        var state = NavigationState()

        state = state.selectTopLevel(TopLevelDestination.CHANNEL)
        assertEquals(Route.TopLevel(TopLevelDestination.CHANNEL), state.route)
        assertEquals(TopLevelDestination.CHANNEL, state.topLevel)
        assertTrue(state.showsChrome)

        state = state.selectTopLevel(TopLevelDestination.GUIDE)
        assertEquals(TopLevelDestination.GUIDE, state.topLevel)

        state = state.selectTopLevel(TopLevelDestination.SETTINGS)
        assertEquals(TopLevelDestination.SETTINGS, state.topLevel)

        state = state.selectTopLevel(TopLevelDestination.HOME)
        assertEquals(TopLevelDestination.HOME, state.topLevel)
    }

    @Test
    fun `details preserve origin for back restoration`() {
        val state = NavigationState()
            .selectTopLevel(TopLevelDestination.CHANNEL)
            .openDetails("media-9")

        assertEquals(
            Route.Details(mediaItemId = "media-9", origin = TopLevelDestination.CHANNEL),
            state.route,
        )
        assertFalse(state.showsChrome)

        val back = state.back()
        assertEquals(Route.TopLevel(TopLevelDestination.CHANNEL), back?.route)
        assertEquals(TopLevelDestination.CHANNEL, back?.topLevel)
    }

    @Test
    fun `player returns to the route it was opened from`() {
        val details = NavigationState()
            .selectTopLevel(TopLevelDestination.HOME)
            .openDetails("media-1")
        val player = details.openPlayer(playbackId = "grant-1")

        assertTrue(player.route is Route.Player)
        assertFalse(player.showsChrome)

        val back = player.back()
        assertEquals(
            Route.Details(mediaItemId = "media-1", origin = TopLevelDestination.HOME),
            back?.route,
        )
    }

    @Test
    fun `back from non-home top-level returns home and home exits`() {
        val fromGuide = NavigationState().selectTopLevel(TopLevelDestination.GUIDE).back()
        assertEquals(Route.TopLevel(TopLevelDestination.HOME), fromGuide?.route)

        assertNull(NavigationState().back())
    }

    @Test
    fun `pairing back depends on paired state`() {
        val unpaired = NavigationState().openPairing().back(isPaired = false)
        assertNull(unpaired)

        val paired = NavigationState().openPairing().back(isPaired = true)
        assertEquals(Route.TopLevel(TopLevelDestination.HOME), paired?.route)
    }

    @Test
    fun `settings is last in the top-level enum order`() {
        assertEquals(TopLevelDestination.SETTINGS, TopLevelDestination.entries.last())
    }

    @Test
    fun `legacy destination bridge maps both directions`() {
        assertEquals(TopLevelDestination.HOME, Destination.CATALOG.toTopLevelOrNull())
        assertEquals(TopLevelDestination.CHANNEL, Destination.CHANNEL.toTopLevelOrNull())
        assertEquals(TopLevelDestination.GUIDE, Destination.EPG.toTopLevelOrNull())
        assertEquals(TopLevelDestination.SETTINGS, Destination.SETTINGS.toTopLevelOrNull())
        assertNull(Destination.DETAIL.toTopLevelOrNull())

        assertEquals(
            Route.TopLevel(TopLevelDestination.HOME),
            Destination.CATALOG.toRoute(selectedMediaItemId = null),
        )
        assertEquals(
            Destination.EPG,
            Route.TopLevel(TopLevelDestination.GUIDE).toLegacyDestination(),
        )
        assertEquals(
            Destination.DETAIL,
            Route.Details("x", TopLevelDestination.HOME).toLegacyDestination(),
        )
    }
}
