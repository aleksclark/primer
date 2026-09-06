package com.aleksclark.primer.student.tasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class OccurrenceDeepLinkTest {
    @Test fun acceptsOnlyOccurrenceLinks() {
        assertEquals("abc", occurrenceIdFromParts("primertasks", "occurrences", "abc"))
        assertEquals("abc", occurrenceIdFromParts("primerstudent", "occurrences", "abc"))
        assertNull(occurrenceIdFromParts("https", "example.test", "abc"))
        assertNull(occurrenceIdFromParts("primertasks", "other", "abc"))
        assertNull(occurrenceIdFromParts("primerstudent", "other", "abc"))
    }
}
