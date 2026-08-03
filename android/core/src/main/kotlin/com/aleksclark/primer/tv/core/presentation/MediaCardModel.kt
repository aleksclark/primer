package com.aleksclark.primer.tv.core.presentation

import com.aleksclark.primer.tv.core.domain.CatalogCard
import com.aleksclark.primer.tv.core.domain.MediaClass
import com.aleksclark.primer.tv.core.domain.Programme

/** Stable identity for a media item in rails and focus restoration. */
@JvmInline
value class MediaId(val value: String)

/**
 * Presentation model for a poster card. Contains no navigation logic — screens
 * decide what selecting the card does.
 */
data class MediaCardModel(
    val id: MediaId,
    val title: String,
    val overview: String,
    val mediaClass: MediaClass,
    val runtimeSeconds: Int,
    val runtimeLabel: String,
    val imagePath: String,
    val oneViewing: Boolean,
    val watched: Boolean,
    val leavingSoon: Boolean,
    val playable: Boolean,
    val classificationLabel: String,
    val statusLabel: String?,
    val metadataLabel: String,
) {
    val mediaItemId: String get() = id.value
}

fun CatalogCard.toMediaCardModel(): MediaCardModel {
    val status = MediaLabels.statusLine(
        oneViewing = consumesPlay,
        watched = alreadyWatched,
        leavingSoon = expiringSoon,
    )
    return MediaCardModel(
        id = MediaId(entry.mediaItemId),
        title = entry.title,
        overview = entry.overview,
        mediaClass = entry.mediaClass,
        runtimeSeconds = entry.runtimeSeconds,
        runtimeLabel = MediaLabels.runtime(entry.runtimeSeconds),
        imagePath = entry.imagePath,
        oneViewing = consumesPlay,
        watched = alreadyWatched,
        leavingSoon = expiringSoon,
        playable = playable,
        classificationLabel = MediaLabels.classification(entry.mediaClass),
        statusLabel = status,
        metadataLabel = MediaLabels.metadataLine(
            mediaClass = entry.mediaClass,
            runtimeSeconds = entry.runtimeSeconds,
            *listOfNotNull(status).toTypedArray(),
        ),
    )
}

/** Lightweight card derived from a programmed title (hero artwork fallback). */
fun Programme.toMediaCardModel(
    watched: Boolean = false,
    leavingSoon: Boolean = false,
): MediaCardModel {
    val oneViewing = mediaClass.consumesPlay
    val status = MediaLabels.statusLine(
        oneViewing = oneViewing,
        watched = watched,
        leavingSoon = leavingSoon,
    )
    return MediaCardModel(
        id = MediaId(mediaItemId),
        title = title,
        overview = overview,
        mediaClass = mediaClass,
        runtimeSeconds = runtimeSeconds,
        runtimeLabel = MediaLabels.runtime(runtimeSeconds),
        imagePath = imagePath,
        oneViewing = oneViewing,
        watched = watched,
        leavingSoon = leavingSoon,
        playable = !watched,
        classificationLabel = MediaLabels.classification(mediaClass),
        statusLabel = status,
        metadataLabel = MediaLabels.metadataLine(
            mediaClass = mediaClass,
            runtimeSeconds = runtimeSeconds,
            *listOfNotNull(status).toTypedArray(),
        ),
    )
}
