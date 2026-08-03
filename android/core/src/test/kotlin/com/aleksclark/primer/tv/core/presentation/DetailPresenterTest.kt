package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.CatalogCard
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.testing.T0
import com.aleksclark.primer.tv.core.testing.entry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DetailPresenterTest {

    @Test
    fun `educational detail offers Play when available`() {
        val model = DetailPresenter.present(card(mediaClass = MediaClass.EDUCATIONAL))

        assertEquals(DetailPrimaryAction.PLAY, model.primaryAction)
        assertEquals(MediaLabels.PLAY, model.primaryActionLabel)
        assertTrue(model.primaryEnabled)
        assertFalse(model.oneViewing)
        assertNull(model.oneViewingWarning)
        assertEquals("Educational · 30m", model.metadataLabel)
    }

    @Test
    fun `entertainment shows one-viewing warning before play`() {
        val model = DetailPresenter.present(card(mediaClass = MediaClass.ENTERTAINMENT))

        assertTrue(model.oneViewing)
        assertEquals(DetailPresenter.ONE_VIEWING_WARNING, model.oneViewingWarning)
        assertEquals(DetailPrimaryAction.PLAY, model.primaryAction)
        assertTrue(model.primaryEnabled)
        assertTrue(model.metadataLabel.contains(MediaLabels.classification(MediaClass.ENTERTAINMENT)))
    }

    @Test
    fun `watched entertainment disables primary action and drops the warning`() {
        val model = DetailPresenter.present(
            card(mediaClass = MediaClass.ENTERTAINMENT, alreadyWatched = true),
        )

        assertEquals(DetailPrimaryAction.WATCHED, model.primaryAction)
        assertEquals(MediaLabels.WATCHED, model.primaryActionLabel)
        assertFalse(model.primaryEnabled)
        assertFalse(model.playable)
        assertNull("warning is for upcoming consumption, not after", model.oneViewingWarning)
        assertEquals(MediaLabels.WATCHED, model.availabilityLabel)
    }

    @Test
    fun `resumable grant upgrades the primary label to Resume`() {
        val model = DetailPresenter.present(
            card(mediaClass = MediaClass.MIXED),
            resumable = true,
        )

        assertEquals(DetailPrimaryAction.RESUME, model.primaryAction)
        assertEquals(MediaLabels.RESUME, model.primaryActionLabel)
        assertTrue(model.primaryEnabled)
    }

    @Test
    fun `watched beats resumable so a consumed title cannot resume`() {
        val action = DetailPresenter.primaryAction(
            playable = false,
            watched = true,
            resumable = true,
        )
        assertEquals(DetailPrimaryAction.WATCHED, action)
        assertFalse(
            DetailPresenter.present(
                card(mediaClass = MediaClass.ENTERTAINMENT, alreadyWatched = true),
                resumable = true,
            ).primaryEnabled,
        )
    }

    @Test
    fun `leaving soon appears in availability metadata`() {
        val model = DetailPresenter.present(
            card(mediaClass = MediaClass.EDUCATIONAL, expiringSoon = true),
        )
        assertEquals(MediaLabels.LEAVING_SOON, model.availabilityLabel)
        assertTrue(model.metadataLabel.contains(MediaLabels.LEAVING_SOON))
    }

    @Test
    fun `backdrop falls back to poster path`() {
        val withBackdrop = DetailPresenter.present(
            card(),
            backdropPath = "/images/x/Backdrop",
        )
        assertEquals("/images/x/Backdrop", withBackdrop.backdropOrPosterPath)

        val posterOnly = DetailPresenter.present(card())
        assertEquals(posterOnly.imagePath, posterOnly.backdropOrPosterPath)
    }

    @Test
    fun `subject tags pass through for chip row`() {
        val model = DetailPresenter.present(
            card(subjectTags = listOf("science", "physics")),
        )
        assertEquals(listOf("science", "physics"), model.subjectTags)
    }

    private fun card(
        mediaClass: MediaClass = MediaClass.EDUCATIONAL,
        alreadyWatched: Boolean = false,
        expiringSoon: Boolean = false,
        subjectTags: List<String> = emptyList(),
    ): CatalogCard {
        val base = entry(
            id = "media-1",
            title = "Inertia",
            mediaClass = mediaClass,
            windowEndsAt = T0.plusSeconds(if (expiringSoon) 3_600 else 7 * 24 * 3_600L),
        )
        val entry = if (subjectTags.isEmpty()) {
            base
        } else {
            base.copy(subjectTags = subjectTags)
        }
        return CatalogCard(
            entry = entry,
            alreadyWatched = alreadyWatched,
            consumesPlay = mediaClass.consumesPlay,
            expiringSoon = expiringSoon,
        )
    }
}
