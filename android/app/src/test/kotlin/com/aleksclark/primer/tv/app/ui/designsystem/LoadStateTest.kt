package com.aleksclark.primer.tv.app.ui.designsystem

import com.aleksclark.primer.tv.core.presentation.UiText
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LoadStateTest {

    @Test
    fun `content carries refreshing without replacing the value`() {
        val state = LoadState.Content(value = listOf("a", "b"), refreshing = true)
        assertEquals(listOf("a", "b"), state.value)
        assertTrue(state.refreshing)
    }

    @Test
    fun `error can disable retry`() {
        val retryable = LoadState.Error(UiText("network down"))
        val terminal = LoadState.Error(UiText("gone"), canRetry = false)

        assertTrue(retryable.canRetry)
        assertFalse(terminal.canRetry)
        assertEquals("network down", retryable.message.value)
    }

    @Test
    fun `empty and loading are distinct terminal states`() {
        val empty: LoadState<String> = LoadState.Empty(UiText("Nothing here"))
        val loading: LoadState<String> = LoadState.Loading

        assertTrue(empty is LoadState.Empty)
        assertTrue(loading is LoadState.Loading)
    }
}
