package com.aleksclark.primertasks

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class OccurrenceDeepLinkTest {
    @Test fun acceptsOnlyOccurrenceLinks() {
        assertEquals("abc", occurrenceIdFromParts("primertasks", "occurrences", "abc"))
        assertNull(occurrenceIdFromParts("https", "example.test", "abc"))
        assertNull(occurrenceIdFromParts("primertasks", "other", "abc"))
    }
}
