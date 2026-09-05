package com.aleksclark.primertasks

import android.net.Uri

internal fun occurrenceIdFromDeepLink(uri: Uri?): String? =
    occurrenceIdFromParts(uri?.scheme, uri?.host, uri?.lastPathSegment)

internal fun occurrenceIdFromParts(scheme: String?, host: String?, lastPathSegment: String?): String? =
    lastPathSegment?.takeIf { scheme == "primertasks" && host == "occurrences" && it.isNotBlank() }
