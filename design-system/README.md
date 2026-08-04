# Primer System C — Record

A single visual foundation for web clients, native apps, and communications. The system uses ruled structure, high contrast, and a monospace system voice. Dark is primary; light is a full-parity theme.

## Foundations

- **Base unit:** 4 px
- **Radius:** 0
- **Rule:** 1 px
- **Content type:** Instrument Sans, weights 400–700
- **System type:** IBM Plex Mono, weights 400–500, uppercase and tracked
- **Accent:** action, state, and focus only
- **Attention:** review and errors only
- **Elevation:** surface contrast and rules, never shadows

Components read like a structured record: aligned columns, ruled rows, explicit labels, and sparse semantic color. Hover increases contrast rather than introducing color. Focus uses a 1 px accent outline at a 2 px offset. Disabled controls drop to rule colors.

## Component reference

[`reference/system-c.html`](reference/system-c.html) is the canonical UI component specification for System C. Derive buttons, inputs, state labels, navigation, cards, selection controls, menus, tutor threads, and surface layouts from that document. Platform UIs must consume generated tokens from `generated/`; do not hard-code hex values or re-copy palette numbers from the reference.

## Iteration loop

1. Edit `tokens.json` or replace the SVG files in `assets/logo/`.
2. Run `make design-system` from the repository root.
3. Open `design-system/preview/index.html` and review both themes.
4. Consume generated files in each platform; never edit them directly.

The dependency-free build validates token structure and WCAG contrast, then creates:

- `generated/primer.css` for web clients and browser-based pitch decks
- `generated/PrimerTokens.kt` for Compose-based native apps
- `generated/presentation-theme.json` for pitch and marketing tooling
- `generated/token-reference.md` for design review

## Logo assets

| File | Use |
|---|---|
| `logo.svg` | PrimerLMS lockup on dark surfaces |
| `logo-light.svg` | PrimerLMS lockup on light surfaces |
| `logo-mark.svg` | Mark on dark surfaces |
| `logo-mark-light.svg` | Mark on light surfaces |

Clear space is half the symbol height on every side. Minimum mark size is 16 px. Below 20 px, render the accent square in monochrome. The lockup uses live text so Instrument Sans must be available; convert text to paths for production channels that cannot guarantee fonts.

## Token policy

Components consume semantic roles such as `surface`, `rule`, `text-muted`, `accent`, and `attention`, never raw colors. Dark mode is the default. Web products select light mode with `data-theme="light"`; native products map the same roles through generated platform tokens.

The system voice covers state, counts, units, timestamps, metadata, and controls. It is uppercase and tracked. Body copy and headings stay in Instrument Sans. Cards, menus, inputs, navigation, tutor transcripts, and presentation layouts remain square and ruled in both themes.

## Review gates

- Mark works at 16 px and becomes monochrome below 20 px.
- Dark and light themes preserve identical geometry and semantics.
- Body text reaches WCAG AA; focus remains visible without color alone.
- No rounded corners, shadows, chat bubbles, or decorative gradients enter components.
- Product, native, and pitch outputs read as one structured visual system.
