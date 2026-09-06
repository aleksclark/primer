package com.aleksclark.primer.student.tasks

import android.net.Uri

fun occurrenceIdFromDeepLink(uri: Uri?): String? =
    occurrenceIdFromParts(uri?.scheme, uri?.host, uri?.lastPathSegment)

fun occurrenceIdFromParts(scheme: String?, host: String?, lastPathSegment: String?): String? =
    lastPathSegment?.takeIf {
        it.isNotBlank() &&
            host == "occurrences" &&
            (scheme == "primertasks" || scheme == "primerstudent")
    }
