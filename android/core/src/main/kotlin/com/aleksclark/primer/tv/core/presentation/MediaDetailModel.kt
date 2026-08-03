package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.CatalogCard
import com.aleksclark.primer.tv.core.domain.MediaClass

/** Primary CTA on the media detail surface. */
enum class DetailPrimaryAction {
    PLAY,
    RESUME,
    WATCHED,
}

/**
 * Presentation model for the details surface. Pure data — layout chooses how
 * to compose backdrop, poster, chips, and actions.
 */
data class MediaDetailModel(
    val id: MediaId,
    val title: String,
    val overview: String,
    val mediaClass: MediaClass,
    val runtimeSeconds: Int,
    val imagePath: String,
    /** Optional landscape artwork; poster [imagePath] is the required fallback. */
    val backdropPath: String?,
    val oneViewing: Boolean,
    val watched: Boolean,
    val leavingSoon: Boolean,
    val playable: Boolean,
    val subjectTags: List<String>,
    val classificationLabel: String,
    val runtimeLabel: String,
    val availabilityLabel: String?,
    val metadataLabel: String,
    val oneViewingWarning: String?,
    val primaryAction: DetailPrimaryAction,
    val primaryActionLabel: String,
    val primaryEnabled: Boolean,
) {
    val mediaItemId: String get() = id.value

    /** Artwork used for backdrop/hero layers: dedicated backdrop, else poster. */
    val backdropOrPosterPath: String
        get() = backdropPath?.takeIf { it.isNotBlank() } ?: imagePath
}

/**
 * Pure reducer for media details. Maps a catalog card (and optional resumable
 * grant flag) into labels and action state the UI can render without domain
 * branching.
 */
object DetailPresenter {

    const val ONE_VIEWING_WARNING =
        "This title may be watched once. Finishing it uses up the single viewing."

    fun present(
        card: CatalogCard,
        resumable: Boolean = false,
        backdropPath: String? = null,
    ): MediaDetailModel {
        val entry = card.entry
        val watched = card.alreadyWatched
        val oneViewing = card.consumesPlay
        val playable = card.playable
        val availability = availabilityLabel(watched = watched, leavingSoon = card.expiringSoon)
        val action = primaryAction(playable = playable, watched = watched, resumable = resumable)

        return MediaDetailModel(
            id = MediaId(entry.mediaItemId),
            title = entry.title,
            overview = entry.overview,
            mediaClass = entry.mediaClass,
            runtimeSeconds = entry.runtimeSeconds,
            imagePath = entry.imagePath,
            backdropPath = backdropPath,
            oneViewing = oneViewing,
            watched = watched,
            leavingSoon = card.expiringSoon,
            playable = playable,
            subjectTags = entry.subjectTags,
            classificationLabel = MediaLabels.classification(entry.mediaClass),
            runtimeLabel = MediaLabels.runtime(entry.runtimeSeconds),
            availabilityLabel = availability,
            metadataLabel = MediaLabels.metadataLine(
                mediaClass = entry.mediaClass,
                runtimeSeconds = entry.runtimeSeconds,
                *listOfNotNull(availability).toTypedArray(),
            ),
            oneViewingWarning = oneViewingWarning(oneViewing = oneViewing, watched = watched),
            primaryAction = action,
            primaryActionLabel = primaryActionLabel(action),
            primaryEnabled = action != DetailPrimaryAction.WATCHED && playable,
        )
    }

    fun primaryAction(
        playable: Boolean,
        watched: Boolean,
        resumable: Boolean,
    ): DetailPrimaryAction = when {
        watched || !playable -> DetailPrimaryAction.WATCHED
        resumable -> DetailPrimaryAction.RESUME
        else -> DetailPrimaryAction.PLAY
    }

    fun primaryActionLabel(action: DetailPrimaryAction): String = when (action) {
        DetailPrimaryAction.PLAY -> MediaLabels.PLAY
        DetailPrimaryAction.RESUME -> MediaLabels.RESUME
        DetailPrimaryAction.WATCHED -> MediaLabels.WATCHED
    }

    fun oneViewingWarning(oneViewing: Boolean, watched: Boolean): String? =
        if (oneViewing && !watched) ONE_VIEWING_WARNING else null

    fun availabilityLabel(watched: Boolean, leavingSoon: Boolean): String? = when {
        watched -> MediaLabels.WATCHED
        leavingSoon -> MediaLabels.LEAVING_SOON
        else -> null
    }
}
