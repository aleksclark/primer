package com.aleksclark.primer.tv.core.presentation

/**
 * User-facing copy that stays free of Android resources so presentation
 * reducers remain JVM-testable. Call sites can later bridge to localized
 * strings without changing model shapes.
 */
@JvmInline
value class UiText(val value: String) {
    override fun toString(): String = value

    companion object {
        fun of(value: String): UiText = UiText(value)
    }
}
