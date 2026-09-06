# Primer core-ui

Shared System C Compose library for Student, Control, and TV. Tokens are
generated once from `design-system/generated/PrimerTokens.kt` into
`com.aleksclark.primer.ui.tokens`. Do not copy palettes or components into
product apps.

Public package: `com.aleksclark.primer.ui`

| API | Role |
|---|---|
| `PrimerTheme(darkTheme: Boolean = true, content)` | Dark-default theme; light is full parity |
| `PrimerTheme.colors` / `.spacing` / `.typography` | Token accessors inside the theme |
| `PrimerButton(text, onClick, modifier, enabled, variant)` | Square ruled action |
| `PrimerTextField(value, onValueChange, label, …)` | Ruled input with focus/error |
| `PrimerStatus(text, modifier, tone)` | System-voice state label |
| `PrimerRecordRow(label, value, …)` | Ruled labelled row |
| `PrimerRuledSurface` / `PrimerRule` / `PrimerSectionHeader` | Square structure |
| `PrimerCheckboxRow` / `PrimerEmptyState` | Form and empty states |

Fonts: Instrument Sans and IBM Plex Mono, SIL OFL 1.1, bundled under `res/font`
with licenses in `res/raw`. TV form-factor/focus adapters stay in `:app`.
