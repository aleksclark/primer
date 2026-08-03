package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.ChannelNow
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.Programme
import com.aleksclark.primer.tv.core.testing.T0
import com.aleksclark.primer.tv.core.testing.catalog
import com.aleksclark.primer.tv.core.testing.entry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class HomePresenterTest {

    @Test
    fun `on-air tunable programme leads the hero as Live`() {
        val state = HomePresenter.present(
            catalog = catalog(
                entry("edu-1", title = "Inertia", mediaClass = MediaClass.EDUCATIONAL),
                entry("ent-1", title = "Apollo 13", mediaClass = MediaClass.ENTERTAINMENT),
            ),
            channelNow = onAirNow(title = "Live Science"),
        )

        val hero = state.hero as HeroModel.Live
        assertEquals("Live Science", hero.title)
        assertEquals(HeroAction.WATCH_LIVE, hero.primaryAction)
        assertTrue(hero.tunable)
        assertEquals(listOf(RailId.LEARN, RailId.ENTERTAINMENT), state.rails.map { it.id })
    }

    @Test
    fun `catalog feature leads when the channel is in a gap`() {
        val state = HomePresenter.present(
            catalog = catalog(
                entry("edu-1", title = "Inertia", mediaClass = MediaClass.EDUCATIONAL),
                entry("mix-1", title = "Workshop", mediaClass = MediaClass.MIXED),
            ),
            channelNow = gapNow(nextTitle = "Evening Block"),
        )

        val hero = state.hero as HeroModel.Featured
        assertEquals("Inertia", hero.title)
        assertEquals(HeroAction.PLAY, hero.primaryAction)
        assertEquals("Evening Block", hero.next?.title)
        assertTrue(hero.next!!.startsInLabel.isNotBlank())
    }

    @Test
    fun `empty service shows intentional empty hero without rail headings`() {
        val state = HomePresenter.present(
            catalog = catalog(),
            channelNow = gapNow(nextTitle = "Morning Math"),
        )

        val hero = state.hero as HeroModel.Empty
        assertEquals("Nothing is available right now", hero.message.value)
        assertEquals("Morning Math", hero.next?.title)
        assertTrue(state.rails.isEmpty())
        assertTrue(state.isEmpty)
    }

    @Test
    fun `non-direct-play on-air falls through to catalog feature`() {
        val state = HomePresenter.present(
            catalog = catalog(entry("edu-1", title = "Inertia")),
            channelNow = onAirNow(title = "Broken Codec", directPlayOk = false),
        )

        val hero = state.hero as HeroModel.Featured
        assertEquals("Inertia", hero.title)
    }

    @Test
    fun `rails follow Learn Worth Watching Entertainment and optional Leaving Soon`() {
        val state = HomePresenter.present(
            catalog = catalog(
                entry("e1", title = "Film", mediaClass = MediaClass.ENTERTAINMENT, windowEndsAt = T0.plusSeconds(3600)),
                entry("m1", title = "Mixed", mediaClass = MediaClass.MIXED),
                entry("l1", title = "Lesson", mediaClass = MediaClass.EDUCATIONAL, windowEndsAt = T0.plusSeconds(7200)),
            ),
            channelNow = null,
        )

        assertEquals(
            listOf(RailId.LEARN, RailId.WORTH_WATCHING, RailId.ENTERTAINMENT, RailId.LEAVING_SOON),
            state.rails.map { it.id },
        )
        val leaving = state.rails.first { it.id == RailId.LEAVING_SOON }
        assertEquals(listOf("l1", "e1"), leaving.itemIds())
    }

    @Test
    fun `leaving soon rail deduplicates across source rails`() {
        val rails = listOf(
            RailModel(
                RailId.LEARN,
                "Learn",
                listOf(card("a", leavingSoon = true), card("b", leavingSoon = false)),
            ),
            RailModel(
                RailId.ENTERTAINMENT,
                "Entertainment",
                listOf(card("a", leavingSoon = true, oneViewing = true)),
            ),
        )
        val leaving = HomePresenter.leavingSoonRail(rails)!!
        assertEquals(listOf("a"), leaving.itemIds())
    }

    @Test
    fun `one-viewing and watched labels are plain language`() {
        val fresh = MediaLabels.statusLabels(oneViewing = true, watched = false, leavingSoon = false)
        val watched = MediaLabels.statusLabels(oneViewing = true, watched = true, leavingSoon = true)

        assertEquals(listOf(MediaLabels.ONE_VIEWING), fresh)
        assertEquals(listOf(MediaLabels.WATCHED, MediaLabels.LEAVING_SOON), watched)
        assertEquals("Educational · 30m · One viewing", MediaLabels.metadataLine(
            MediaClass.EDUCATIONAL,
            1800,
            MediaLabels.ONE_VIEWING,
        ))
    }

    @Test
    fun `consumed ids mark cards watched and sink them in rails`() {
        val state = HomePresenter.present(
            catalog = catalog(
                entry("ent-1", title = "Alpha", mediaClass = MediaClass.ENTERTAINMENT),
                entry("ent-2", title = "Bravo", mediaClass = MediaClass.ENTERTAINMENT),
            ),
            channelNow = null,
            consumedMediaItemIds = setOf("ent-1"),
        )

        val entertainment = state.rails.first { it.id == RailId.ENTERTAINMENT }
        assertEquals(listOf("ent-2", "ent-1"), entertainment.itemIds())
        assertTrue(entertainment.items.last().watched)
        assertEquals(MediaLabels.WATCHED, entertainment.items.last().statusLabel)
        assertFalse(entertainment.items.last().playable)

        val hero = state.hero as HeroModel.Featured
        assertEquals("ent-2", hero.mediaItemId)
    }

    @Test
    fun `initial load without data shows structure-preserving skeletons`() {
        val state = HomePresenter.present(
            catalog = null,
            channelNow = null,
            loading = true,
        )

        assertEquals(HeroModel.Loading, state.hero)
        assertTrue(state.showSkeletons)
        assertEquals(3, state.rails.size)
        assertTrue(state.rails.all { it.items.isEmpty() })
    }

    @Test
    fun `begin refresh preserves content and sets refreshing`() {
        val loaded = HomePresenter.present(
            catalog = catalog(entry("edu-1", title = "Inertia")),
            channelNow = null,
        )
        val refreshing = HomePresenter.beginRefresh(loaded)

        assertTrue(refreshing.refreshing)
        assertFalse(refreshing.loading)
        assertEquals(loaded.contentIds(), refreshing.contentIds())
        assertTrue(refreshing.hero is HeroModel.Featured)
    }

    @Test
    fun `refresh failure keeps existing content and surfaces a non-blocking error`() {
        val loaded = HomePresenter.present(
            catalog = catalog(entry("edu-1", title = "Inertia")),
            channelNow = onAirNow(),
        )
        val failed = HomePresenter.applyRefreshFailure(loaded, "network down")

        assertEquals("network down", failed.error?.value)
        assertTrue(failed.hero is HeroModel.Live)
        assertEquals(loaded.rails.map { it.id }, failed.rails.map { it.id })
        assertFalse(failed.refreshing)
    }

    @Test
    fun `refresh failure without content becomes a full error state`() {
        val loading = HomePresenter.present(catalog = null, channelNow = null, loading = true)
        val failed = HomePresenter.applyRefreshFailure(loading, "offline")

        assertEquals("offline", failed.error?.value)
        assertTrue(failed.hero is HeroModel.Empty)
        assertTrue(failed.rails.isEmpty())
    }

    @Test
    fun `successful refresh replaces models in place for the same ids`() {
        val previous = HomePresenter.present(
            catalog = catalog(entry("edu-1", title = "Inertia")),
            channelNow = null,
        )
        val next = HomePresenter.applyRefreshSuccess(
            previous = previous,
            catalog = catalog(entry("edu-1", title = "Inertia (updated)")),
            channelNow = null,
        )

        assertEquals(previous.contentIds(), next.contentIds())
        val hero = next.hero as HeroModel.Featured
        assertEquals("Inertia (updated)", hero.title)
        assertNull(next.error)
        assertFalse(next.refreshing)
    }

    private fun card(
        id: String,
        leavingSoon: Boolean = false,
        oneViewing: Boolean = false,
        watched: Boolean = false,
    ) = MediaCardModel(
        id = MediaId(id),
        title = id,
        overview = "",
        mediaClass = MediaClass.EDUCATIONAL,
        runtimeSeconds = 1800,
        runtimeLabel = "30m",
        imagePath = "/images/$id/Primary",
        oneViewing = oneViewing,
        watched = watched,
        leavingSoon = leavingSoon,
        playable = !watched,
        classificationLabel = "Educational",
        statusLabel = MediaLabels.statusLine(oneViewing, watched, leavingSoon),
        metadataLabel = "Educational · 30m",
    )

    private fun onAirNow(
        title: String = "On Air",
        directPlayOk: Boolean = true,
    ): ChannelNow {
        val programme = Programme(
            scheduleEntryId = "sched-1",
            mediaItemId = "live-1",
            title = title,
            overview = "Live overview",
            mediaClass = MediaClass.EDUCATIONAL,
            subjectTags = emptyList(),
            runtimeSeconds = 1800,
            airsAt = T0.minusSeconds(600),
            endsAt = T0.plusSeconds(1200),
            block = "morning",
            joinInProgress = true,
            directPlayOk = directPlayOk,
            imagePath = "/images/live-1/Primary",
        )
        return ChannelNow(
            onAir = programme,
            offsetSeconds = 600,
            startOffsetSeconds = 600,
            next = null,
            nextStartsInSeconds = 0,
            serverTime = T0,
        )
    }

    private fun gapNow(nextTitle: String = "Next Up"): ChannelNow {
        val next = Programme(
            scheduleEntryId = "sched-2",
            mediaItemId = "next-1",
            title = nextTitle,
            overview = "",
            mediaClass = MediaClass.MIXED,
            subjectTags = emptyList(),
            runtimeSeconds = 2400,
            airsAt = T0.plusSeconds(1800),
            endsAt = T0.plusSeconds(4200),
            block = "afternoon",
            joinInProgress = true,
            directPlayOk = true,
            imagePath = "/images/next-1/Primary",
        )
        return ChannelNow(
            onAir = null,
            offsetSeconds = 0,
            startOffsetSeconds = 0,
            next = next,
            nextStartsInSeconds = 1800,
            serverTime = T0,
        )
    }
}
