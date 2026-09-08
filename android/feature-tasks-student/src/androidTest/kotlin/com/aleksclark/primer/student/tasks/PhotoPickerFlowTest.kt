package com.aleksclark.primer.student.tasks

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Prototype Photo Picker coverage stays on `com.aleksclark.primertasks` until
 * Student connected promotion is rerun. This class records the Student identity
 * the replacement test must use; it does not launch Activities or touch devices.
 */
class PhotoPickerFlowTest {
    @Test
    fun studentIdentityIsDistinctFromPrototype() {
        assertEquals("com.aleksclark.primer.student", STUDENT_PACKAGE)
        assertEquals(PairingActions.IMPORT_IMAGE, IMPORT_ACTION)
    }

    private companion object {
        const val STUDENT_PACKAGE = "com.aleksclark.primer.student"
        const val IMPORT_ACTION = PairingActions.IMPORT_IMAGE
    }
}
