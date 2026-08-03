# Primer TV UI/UX Redesign Plan

## Purpose

Replace the current utility-style Android UI with a coherent streaming
experience modeled on the interaction patterns users already know from
Netflix, Disney+, Prime Video, Apple TV, YouTube TV, and Jellyfin — without
copying any brand’s visual identity or weakening Primer’s media rules.

The redesign must work as **one product with two appropriate presentations**:

- **Phone/tablet:** touch-first, portrait and landscape, compact navigation,
  conventional Material behavior.
- **Android TV:** D-pad-first, 10-foot readability, explicit focus, predictable
  focus restoration, and no dependence on touch gestures.

The backend remains authoritative for catalog visibility, watch-once status,
programmed-channel timing, playback permissions, and reporting. This is a UI
redesign, not a new access-control system.

## Why the current UI fails

The current implementation is functional but visually and behaviorally closer
to an internal CRUD client than a streaming product:

1. **No unified home experience.** “Available now” and “The channel” are
   separate destinations, even though mainstream streaming apps lead with a
   single home page containing a featured/current item plus rows.
2. **Header action overload.** Large text buttons for Channel, Refresh, and
   Settings compete with the page title and media content.
3. **Weak visual hierarchy.** Every screen begins as a padded column; no hero,
   artwork backdrop, content-first landing state, persistent app chrome, or
   primary/secondary action distinction.
4. **Poster cards are not interaction-complete.** They have no focused/pressed/
   loading/image-failure treatments, no consistent metadata placement, and no
   focus memory for TV.
5. **Tablet and TV only differ by integer sizes.** A TV interface is not a
   tablet interface with larger padding: it needs separate navigation,
   focus traversal, focus scaling, overscan-safe composition, fewer controls,
   and different information density.
6. **Navigation is implicit.** Back-stack behavior exists in the view model,
   but the interface offers no consistent top-level model such as Home,
   Channel, Guide, and Settings.
7. **Loading/error/empty states are raw text.** They replace whole layouts and
   cause abrupt jumps instead of preserving page structure with skeletons,
   retry actions, and content-specific messages.
8. **Program guide is a plain list.** It lacks the time-axis treatment,
   current-position emphasis, and scannability users expect from linear-TV
   guides.
9. **Detail pages are rigid two-column rows.** This overflows on portrait phones
   and underuses TV screens; artwork, metadata, copy, and actions do not adapt.
10. **No reusable design system.** Colors, typography, spacing, card geometry,
    focus effects, state views, and media metadata are assembled ad hoc.

## Product principles

### 1. Familiar before novel

Use established streaming patterns:

- App opens to **Home**.
- A prominent **hero** answers “what should I watch now?”
- Horizontal **content rails** organize choices.
- Selecting a card opens a **detail surface**.
- A persistent, low-noise navigation model exposes Home, Channel, Guide, and
  Settings.
- Playback UI disappears when idle and reflects the actual control policy.

### 2. Curriculum-aware, not school-looking

Educational classification should inform metadata and curation, not make the
student feel like he opened an LMS. Subject/standard tags appear in details and
parent surfaces, while the student UI favors title, artwork, synopsis, runtime,
and a restrained “Educational / Mixed / Entertainment” label.

### 3. Rules should be visible before they surprise

- Entertainment details say **“One viewing”** before Play.
- Programmed content says **“Live — joins in progress”** and shows time left.
- Unavailable items are omitted by the server; items consumed during the
  current session transition to a clear “Watched” state before disappearing on
  refresh.
- Seeking/pause affordances appear only when allowed.

### 4. Content remains stable while data changes

Refresh, heartbeat, token renewal, and catalog reloads must not destroy scroll
or focus position. Rails keep their location; cards update in place; a small
status treatment replaces full-page flashes.

### 5. TV focus is a first-class state

Every actionable TV element needs:

- deterministic next focus,
- visible focused state (scale + border/glow + elevation),
- stable focus restoration after returning from details/player,
- no focus traps,
- focused item kept fully inside overscan-safe bounds.

### 6. Phone ergonomics are not TV ergonomics

Share domain state and component APIs, but allow form-factor-specific
compositions. Reuse behavior and tokens, not necessarily the exact layout tree.

---

# Information Architecture

## Top-level destinations

| Destination | Purpose | Phone/tablet navigation | Android TV navigation |
|---|---|---|---|
| **Home** | Featured/on-now hero plus on-demand rails | Bottom navigation (portrait), navigation rail (wide landscape) | Collapsed left navigation rail, expands on focus |
| **Channel** | Current programmed broadcast and immediate next item | Bottom/rail destination | Left-nav destination; initial focus on Watch Live |
| **Guide** | Today/tomorrow schedule | Bottom/rail destination | Left-nav destination; grid/list optimized for D-pad |
| **Settings** | Device identity, server, update, unpair | Toolbar overflow or bottom/rail destination | Left-nav destination, intentionally last |

**Details and Player are child destinations**, never top-level navigation items.
Pairing is a gated onboarding flow shown only when no valid pairing exists.

## Home composition

1. App/navigation chrome
2. **Featured hero**
   - Prefer currently airing programme when tunable.
   - Otherwise use the first curated catalog feature.
   - If neither exists, show a restrained empty hero with next scheduled item.
3. Optional **Continue Watching** rail (only when resumable grant/session data
   exists; do not fabricate it from max position alone)
4. **Learn** rail (educational)
5. **Worth Watching** rail (mixed)
6. **Entertainment** rail (watch-once, visually labeled)
7. Optional **Leaving Soon** rail (derived client-side from existing expiry
   state, deduplicated from original rails or clearly cross-listed by design)

## Detail composition

- Backdrop/hero image (landscape artwork when available; graceful poster-based
  fallback)
- Scrim/gradient for text contrast
- Title
- Metadata line: classification · runtime · availability status
- Primary action: Play / Resume / Watch Live
- Secondary action: Back (phone uses system back; TV may show subtle button)
- One-viewing warning when relevant
- Synopsis
- Subject chips (optional collapsed row)
- “Up next” information for channel content

## Player composition

| Mode | Visible controls |
|---|---|
| On-demand educational/mixed | Play/pause, seek bar, rewind/forward, elapsed/remaining, back |
| On-demand entertainment | Play/pause, non-interactive progress, elapsed/remaining, back; no seek buttons |
| Programmed | Minimal “LIVE”/channel title overlay and back only; no pause/seek transport controls |

All policies continue to be enforced at the player abstraction, not merely by
hiding Compose controls.

---

# Form-Factor Designs

## Phone / tablet

### Phone portrait

- Material 3 bottom navigation: Home, Channel, Guide.
- Settings accessed from profile/settings icon in the top app bar.
- Hero uses 16:9 backdrop, title/action content below or over lower gradient.
- Rails use poster cards approximately 132–152dp wide.
- Details become a vertical scroll: backdrop → title/meta/actions → synopsis.
- Guide is a vertical schedule grouped by time; sticky day header.

### Tablet portrait

- Same bottom navigation unless width is at least 840dp.
- Poster cards 156–180dp.
- Detail may use poster beside metadata when width permits.

### Tablet landscape

- Navigation rail at left.
- Hero may occupy 55–65% width with metadata over gradient.
- Details use adaptive two-pane layout.
- Guide can show time column + programme cards.

## Android TV

- Overscan-safe root padding: minimum 48dp horizontal / 32dp vertical,
  configurable through tokens.
- Collapsed left navigation rail; focused navigation expands to icon + label.
- Hero fills upper 55–65% of viewport with backdrop/scrim.
- Rails overlap or follow the lower hero edge, matching mainstream streaming
  layouts while keeping text readable.
- Poster cards approximately 180–220dp wide depending on resolution/density;
  focus scales to 1.06–1.10 without clipping adjacent items.
- Focused card reveals or emphasizes title/metadata; unfocused cards remain
  quiet.
- Returning from details/player restores the exact rail and card focus.
- Guide uses a time-led list first (simpler, more robust than a full horizontal
  EPG grid), with current programme highlighted and auto-scrolled into view.

---

# Visual System

## Design tokens

Create a `ui/designsystem` package rather than scattering raw values.

### Color roles

| Token | Intent |
|---|---|
| `Background` | Near-black blue/charcoal, never pure black except player |
| `Surface` | Cards, dialogs, menus |
| `SurfaceRaised` | Focused/selected elements |
| `OnSurface` | Primary text |
| `OnSurfaceMuted` | Metadata and hints |
| `Brand` | Primary action and focused navigation |
| `Live` | Current programmed content |
| `Educational` | Restrained classification accent |
| `Entertainment` | One-viewing warning accent, not danger red |
| `Error` | Actual failures only |
| `ScrimStrong/Soft` | Artwork readability gradients |

Do not color every classification card. Classification color belongs in a badge
or metadata indicator; artwork remains primary.

### Typography

Define semantic styles:

- `HeroTitle`
- `ScreenTitle`
- `RailTitle`
- `CardTitle`
- `Metadata`
- `Body`
- `Label`
- `Button`
- `GuideTime`

TV sizes and phone sizes are separate token sets selected by form factor, not
manual arithmetic such as `bodySize - 3`.

### Shape and elevation

- Media cards: 8–12dp corner radius
- Buttons/chips: consistent Material shape family
- Focused TV card: border + scale + elevated shadow, never scale alone
- Artwork skeletons use the final card shape to avoid layout jumps

### Spacing

Use an 4/8dp-based scale (`xs=4`, `sm=8`, `md=16`, `lg=24`, `xl=32`,
`xxl=48`, TV overscan token), replacing arbitrary in-file values.

---

# Reusable Components

## Foundation

### `PrimerTvTheme`

**Responsibility:** supplies color, typography, shapes, spacing, motion, and
form-factor tokens.

```kotlin
@Composable
fun PrimerTvTheme(
    formFactor: FormFactor,
    content: @Composable () -> Unit,
)
```

### `StreamingScaffold`

**Responsibility:** top-level navigation and content slot; separate phone and TV
implementations behind one API.

```kotlin
@Composable
fun StreamingScaffold(
    formFactor: FormFactor,
    selected: TopLevelDestination,
    onSelect: (TopLevelDestination) -> Unit,
    content: @Composable (PaddingValues) -> Unit,
)
```

### `TvFocusSurface`

**Responsibility:** standard focus animation, border, scale, elevation,
bring-into-view, semantics, and click/key handling.

```kotlin
@Composable
fun TvFocusSurface(
    selected: Boolean = false,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable BoxScope.(focused: Boolean) -> Unit,
)
```

### `AsyncMediaArtwork`

**Responsibility:** image loading, aspect ratio, placeholder/skeleton,
crossfade, error fallback, content description, and optional scrim.

Variants:
- Poster (2:3)
- Backdrop (16:9)
- Thumbnail (16:9)

### `ScreenStateLayout`

**Responsibility:** preserve screen structure while rendering loading, error,
empty, or content state.

```kotlin
sealed interface LoadState<out T> {
    data object Loading : LoadState<Nothing>
    data class Content<T>(val value: T, val refreshing: Boolean) : LoadState<T>
    data class Empty(val message: UiText) : LoadState<Nothing>
    data class Error(val message: UiText, val canRetry: Boolean) : LoadState<Nothing>
}
```

## Media components

### `MediaPosterCard`

Displays poster, title, metadata, badges, watched state, expiry state, and TV
focus. Contains no navigation logic.

Props/state:
- artwork URL
- title
- runtime label
- classification
- `oneViewing`
- `watched`
- `leavingSoon`
- focused/pressed semantics

### `ContentRail`

Owns rail title, horizontal lazy list, loading skeleton row, focus restoration,
and edge padding.

```kotlin
@Composable
fun ContentRail(
    id: RailId,
    title: String,
    items: ImmutableList<MediaCardModel>,
    state: LazyListState,
    onSelect: (MediaId) -> Unit,
)
```

### `FeaturedHero`

Supports:
- on-air programme,
- featured on-demand item,
- next-programme gap state,
- loading skeleton,
- artwork fallback.

Contains primary action and restrained metadata. The screen decides what model
to provide; the component does not fetch.

### `MediaMetadataRow`

Consistent runtime/classification/availability rendering for cards and details.

### `OneViewingBadge`

Reusable, plain-language warning. Must not resemble an error unless playback is
actually unavailable.

### `MediaDetailPane`

Form-factor-adaptive detail content. Phone layout is vertical; TV/wide tablet
uses layered/two-pane composition.

## Channel components

### `LiveBadge`

High-contrast but restrained “LIVE” label with accessibility semantics.

### `OnNowHero`

Channel-specific hero with offset/remaining time, Watch Live, and Up Next.

### `ProgrammeRow`

Reusable guide row with time, title, runtime, classification, current/past/
future state, and optional focused state.

### `ProgrammeGuide`

Day heading, current-time behavior, auto-scroll to current/next item, stable
focus, loading skeletons, and retry state.

## App/system components

### `PairingCard`

Centered onboarding surface with server URL, uppercase code input, inline error,
and progress. On TV, inputs and button have explicit focus order.

### `SettingsSection`

Reusable heading + content grouping for Device, Server, Updates, and Danger
Zone.

### `UpdateCard`

Shows installed version, update status, progress, and install action.

### `PlaybackErrorOverlay`

Replaces the current plain full-screen message; displays a concise error,
contextual retry/return action, and preserves player background when safe.

### `SnackbarHost` / transient feedback

Use for refresh success, update checks, and non-blocking failures. Do not replace
whole screens for transient events.

---

# State and Architecture Plan

## Split navigation from content state

Replace the single flat `Destination` enum with:

```kotlin
enum class TopLevelDestination { HOME, CHANNEL, GUIDE, SETTINGS }

sealed interface Route {
    data class TopLevel(val destination: TopLevelDestination) : Route
    data class Details(val mediaItemId: String, val origin: TopLevelDestination) : Route
    data class Player(val playbackId: String) : Route
    data object Pairing : Route
}
```

Use Navigation Compose or a small typed route coordinator, but maintain:

- origin destination,
- rail/list scroll state,
- TV focused item key,
- back-stack restoration,
- player return destination.

## Build a unified `HomeUiState`

The home screen currently combines catalog and channel data only in the UI. Add
a presentation model:

```kotlin
data class HomeUiState(
    val hero: HeroModel,
    val rails: ImmutableList<RailModel>,
    val loading: Boolean,
    val refreshing: Boolean,
    val error: UiText?,
)
```

The ViewModel combines `Catalog`, `ChannelNow`, and any persisted resume state.
The UI receives one coherent home model.

## Use explicit UI events

Prefer:

```kotlin
sealed interface HomeEvent {
    data object Refresh : HomeEvent
    data class SelectMedia(val id: String) : HomeEvent
    data object OpenChannel : HomeEvent
    data object PlayHero : HomeEvent
}
```

This makes BDD tests describe user intent instead of implementation methods and
keeps phone/TV compositions behaviorally identical.

## State persistence

- `rememberSaveable` / ViewModel saved state for selected top-level destination.
- `LazyListState` per rail keyed by `RailId`.
- `FocusRequester` and last-focused media ID per TV rail.
- Details/player return restores the originating card.
- Catalog refresh does not reset rail state when IDs remain stable.

## Image strategy

Current APIs return poster paths. For a mainstream hero, add a backend image
variant for Jellyfin backdrop/logo artwork if available:

- `/images/{mediaItemId}/Backdrop`
- `/images/{mediaItemId}/Logo` (optional)
- existing `Primary` poster fallback

This API addition should be a separate backend task; the UI must gracefully
fall back to poster artwork blurred/cropped into a 16:9 backdrop.

## Accessibility

- Touch targets ≥48dp.
- TV focus target ≥56dp high for actions.
- Every image has meaningful content description or is explicitly decorative.
- Classification and one-viewing status are text, not color-only.
- Screen reader announcements for pairing failure, update ready, and playback
  refusal.
- Respect system font scaling on phone/tablet; cap TV hero typography only when
  necessary to prevent clipping.
- Do not autoplay previews with sound.
- Respect reduced motion when available; focus motion remains subtle.

## Motion

Use short, purposeful transitions:

- Focus scale: 120–180ms
- Detail shared-axis/fade: 180–250ms
- Artwork crossfade: 200ms
- Navigation rail expand: 180–220ms
- Skeleton shimmer optional; static skeleton preferred on low-power RK3318
- No auto-playing video hero in v1 (performance, bandwidth, discipline)

---

# BDD Feature Specifications

The following are acceptance specifications, not merely examples. Implement
these as executable tests where practical (JVM presentation tests, Compose UI
tests, and instrumentation tests for focus/navigation).

## Feature: Pair a device

### Scenario: Pairing code is normalized

```gherkin
Given the app is not paired
And the pairing screen is visible
When the user types "a3c9xy" into the pairing-code field
Then the field displays "A3C9XY"
And the request body contains "A3C9XY"
```

### Scenario: Successful pairing enters Home

```gherkin
Given the server address is valid
And the pairing code is unused and unexpired
When the user chooses Pair
Then the pairing form shows an in-progress state
And the device token is persisted before the first authenticated request
And the app navigates to Home
And Home remains visible after the first catalog refresh
```

### Scenario: Pairing failure stays actionable

```gherkin
Given the pairing code is invalid or expired
When the user chooses Pair
Then the app remains on the pairing screen
And an inline error explains that a new code is required
And the entered server address remains intact
And focus returns to the pairing-code field
```

### Scenario: TV focus order is predictable

```gherkin
Given the pairing screen is open on Android TV
When the user presses Down repeatedly
Then focus moves from server address
To pairing code
To Pair
And does not leave the pairing card
```

## Feature: Home resembles a streaming service

### Scenario: On-air programme leads the home page

```gherkin
Given a direct-play programme is currently airing
And catalog rails are available
When Home loads
Then the hero shows the on-air artwork and title
And the hero is labeled Live
And the primary action is Watch Live
And on-demand rails appear below the hero
```

### Scenario: Catalog feature leads when the channel is in a gap

```gherkin
Given no programme is currently airing
And the catalog contains at least one item
When Home loads
Then the hero shows a curated catalog item
And the primary action is Play or View Details
And the next scheduled programme is shown unobtrusively if known
```

### Scenario: Empty service still feels intentional

```gherkin
Given the channel is in a gap
And the catalog is empty
When Home loads
Then the hero says that nothing is available now
And the next programme is shown when known
And the screen does not display empty rail headings
And Settings remains reachable
```

### Scenario: Refresh preserves position

```gherkin
Given the user has scrolled to the fourth card in the Learn rail
When a background refresh completes with the same media IDs
Then the Learn rail remains at the same scroll position
And the same card remains focused on TV
And no full-screen loading indicator replaces existing content
```

### Scenario: Loading uses structure-preserving skeletons

```gherkin
Given Home has no cached data
When catalog and channel requests are in flight
Then a hero skeleton is visible
And at least two rail skeletons are visible
And navigation remains usable
```

## Feature: Browse content rails

### Scenario: TV card focus is obvious

```gherkin
Given a media rail is visible on Android TV
When a card receives focus
Then the card scales slightly
And receives a high-contrast focus border or glow
And its full title and metadata are readable
And adjacent cards remain visible
```

### Scenario: Focus returns after details

```gherkin
Given the user opened Details from card 5 in the Entertainment rail
When the user presses Back
Then Home is restored
And the Entertainment rail is still scrolled to card 5
And card 5 has focus
```

### Scenario: One-viewing media is clear before play

```gherkin
Given an entertainment item has one available viewing
When its card or details are visible
Then the UI displays "One viewing"
And the label is not styled as an error
When the server reports the viewing consumed
Then the card becomes "Watched" in place
And Play is disabled
Until the next catalog refresh removes it
```

### Scenario: Artwork failure is graceful

```gherkin
Given an artwork request fails
When the card is rendered
Then the card retains its expected dimensions
And a branded placeholder is shown
And title and metadata remain usable
```

## Feature: View media details

### Scenario: Phone detail adapts vertically

```gherkin
Given the app is on a portrait phone
When the user opens media details
Then artwork appears above the metadata and actions
And the title is not clipped horizontally
And the synopsis scrolls vertically
And the primary action remains reachable without horizontal scrolling
```

### Scenario: TV detail prioritizes action and artwork

```gherkin
Given the app is on Android TV
When the user opens media details
Then backdrop artwork fills the background with a readable scrim
And initial focus is on Play
And metadata and synopsis fit inside overscan-safe bounds
And Back restores the originating card focus
```

### Scenario: Consumed item cannot be replayed

```gherkin
Given an entertainment item was consumed during this app session
When Details opens for that item
Then the primary action reads Watched
And the action is disabled
And no grant request is sent
```

## Feature: Watch the programmed channel

### Scenario: Join a live programme

```gherkin
Given a programme is currently airing and join-in-progress is allowed
When the user chooses Watch Live
Then the player requests a programmed grant
And playback starts at the server-provided offset
And the overlay displays Live and the programme title
And seek, rewind, fast-forward, and pause are unavailable
```

### Scenario: Resume re-synchronizes to broadcast

```gherkin
Given the programmed player was backgrounded
And the broadcast advanced while the app was paused
When the app returns to the foreground
Then the client asks the server for the current offset
And skips forward when behind
And never seeks backward to repeat already-aired content
```

### Scenario: Back exits live playback

```gherkin
Given programmed playback is active
When the user presses Back
Then playback stops
And completion is reported as false unless the programme ended
And the app returns to the Channel screen
And the Channel screen refreshes its on-now state
```

### Scenario: Channel gap shows what follows

```gherkin
Given no programme is airing
And a programme is scheduled later today
When the Channel screen opens
Then the hero shows an off-air state
And displays the next title and start time
And Watch Live is unavailable
And Guide remains available
```

## Feature: Use the guide

### Scenario: Guide opens around the current slot

```gherkin
Given today has past, current, and future programmes
When the user opens Guide
Then the guide scrolls to the current programme
And the current programme is labeled On now
And past programmes are visually de-emphasized
And future programme times are readable in household local time
```

### Scenario: Guide is usable when empty

```gherkin
Given nothing is scheduled today
When Guide opens
Then an empty state says Nothing scheduled today
And navigation remains usable
And Refresh is available without dominating the screen
```

## Feature: Playback controls obey content policy

### Scenario Outline: On-demand control policy

```gherkin
Given an on-demand item classified as <class>
When playback begins
Then pause is <pause>
And seeking is <seek>
And the progress control is <progress>

Examples:
  | class         | pause     | seek      | progress        |
  | educational   | available | available | interactive     |
  | mixed         | available | available | interactive     |
  | entertainment | available | absent    | non-interactive |
  | unknown       | available | absent    | non-interactive |
```

### Scenario: Hidden controls are still enforced

```gherkin
Given entertainment playback is active
When a D-pad seek key, media-session seek, or progress-bar gesture is attempted
Then the player position does not seek
And the UI does not expose a seek command
```

## Feature: Handle errors without destroying context

### Scenario: Refresh fails with cached content

```gherkin
Given Home already displays catalog content
When a refresh fails due to network loss
Then existing content stays visible
And a non-blocking message says it could not refresh
And Retry is available
And rail position and focus are unchanged
```

### Scenario: Initial load fails

```gherkin
Given no cached content exists
When the initial catalog request fails
Then Home shows a branded error state
And Retry is the primary action
And Settings remains reachable to correct the server address
```

### Scenario: Token is revoked

```gherkin
Given the stored device token is rejected with 401
When an authenticated request completes
Then credentials are cleared exactly once
And the app navigates to Pairing
And the server address is retained
And a message explains that the device must be paired again
```

## Feature: Settings and updates

### Scenario: Settings is grouped and low-risk

```gherkin
Given Settings is open
Then Device, Server, Updates, and Danger Zone are separate sections
And Unpair is visually separated from routine actions
And checking for updates does not block navigation
```

### Scenario: New update is available

```gherkin
Given the server advertises a higher version code
When update checking completes
Then the Update card shows the new version
And Install update is available
When the APK digest does not match
Then installation does not start
And a clear retryable error is shown
```

---

# Test Strategy

## 1. Pure JVM presentation tests

Target:

- hero selection from channel/catalog state,
- rail ordering/deduplication,
- metadata label generation,
- one-viewing/watched transitions,
- top-level navigation transitions,
- focus restoration state reducer,
- pairing uppercase normalization,
- error/load-state reduction.

Prefer immutable presentation models in `core` so these tests need no Android
runtime.

## 2. Compose UI tests (phone/tablet)

Add tests for:

- onboarding field order and uppercase rendering,
- bottom-nav destination switching,
- adaptive detail layout at compact/medium/expanded widths,
- skeleton/error/empty/content states,
- Play disabled for watched item,
- Settings grouping and update states,
- content descriptions and touch target sizes.

## 3. Android TV / D-pad instrumentation tests

Use Compose test semantics and injected key events to verify:

- nav rail expansion,
- card focus scale/border state,
- deterministic Up/Down/Left/Right traversal,
- rail focus restoration after Details,
- Guide current-item focus,
- programmed-player back behavior,
- no focusable hidden transport controls.

Run these on an API 28 TV emulator where possible; retain a hardware smoke
check on the T9 box for codec/player behavior.

## 4. Screenshot / golden tests

Create golden fixtures for:

- Home: phone portrait, tablet landscape, TV 1080p
- Details: educational + one-viewing entertainment
- Channel: on-air + gap
- Guide: populated + empty
- Pairing: normal + error
- Loading/error states

Use deterministic fake images and fixed clocks. Goldens guard visual hierarchy,
not pixel-level device chrome.

## 5. Accessibility checks

Automate where possible:

- no unlabeled actionable nodes,
- touch target minimums,
- text scale at 1.3×/1.5×,
- focus order snapshots,
- contrast validation for token pairs,
- status labels include text.

## 6. Physical-device acceptance

### Phone/tablet

- Install on the current Samsung phone.
- Pair with lowercase code and confirm uppercase display/request.
- Browse rails in portrait and landscape.
- Start each playback class and verify controls.
- Rotate while Details is open and verify no crash/layout loss.

### T9 Android TV box

- Navigate every destination with D-pad only.
- Verify focus never disappears.
- Verify focused cards are not clipped.
- Start H.264 and H.265 representative titles.
- Background/resume programmed playback and verify re-sync.
- Cold boot in kiosk mode and reach Home without Android desktop interaction.

---

# Implementation Phases

## Phase A — Design system and navigation skeleton ✅

**Deliverables**

- `PrimerTvTheme` and token sets
- typed top-level navigation model
- `StreamingScaffold` phone + TV implementations
- `TvFocusSurface`
- base loading/error/empty components
- component preview catalog

**Exit criteria**

- Home/Channel/Guide/Settings destinations switch correctly on phone and TV.
- D-pad navigation is visible and deterministic.
- Existing screens can still be reached behind temporary adapters.

## Phase B — Home and reusable media components ✅

**Deliverables**

- `HomeUiState` reducer combining channel/catalog data
- `FeaturedHero`
- `ContentRail`
- `MediaPosterCard`
- image placeholder/error handling
- skeleton home
- refresh preservation

**Exit criteria**

- Home presents on-air hero + rails.
- TV focus restores after Details.
- Phone portrait and tablet landscape pass golden tests.

## Phase C — Details and playback overlays ✅

**Deliverables**

- adaptive `MediaDetailPane`
- one-viewing warning treatment
- reusable playback overlays by policy
- contextual playback errors
- optional backend backdrop image variant

**Exit criteria**

- Details adapt cleanly at compact/expanded widths and TV.
- Controls exactly match policy and remain enforced at the player layer.
- Watched entertainment cannot initiate a grant.

## Phase D — Channel and guide ✅

**Deliverables**

- `OnNowHero`
- `LiveBadge`
- `ProgrammeGuide` and `ProgrammeRow`
- current-programme auto-scroll/focus
- gap/up-next state

**Exit criteria**

- Channel and Guide feel like part of the same app, not separate utility pages.
- Programmed tune-in and resume behavior remain server-authoritative.

## Phase E — Pairing, settings, polish ✅

**Deliverables**

- `PairingCard` onboarding
- grouped settings and update card
- snackbars/transient messages
- accessibility pass
- motion/reduced-motion pass
- physical phone/TV acceptance suite

**Exit criteria**

- All BDD scenarios pass.
- No raw full-page text states remain.
- T9 box is fully navigable by D-pad.
- Samsung phone layout is coherent in portrait and landscape.

---

# Proposed Package Structure

```text
android/app/src/main/kotlin/com/aleksclark/primer/tv/app/
├── navigation/
│   ├── Route.kt
│   ├── TopLevelDestination.kt
│   └── NavigationState.kt
├── ui/
│   ├── designsystem/
│   │   ├── Color.kt
│   │   ├── Theme.kt
│   │   ├── Tokens.kt
│   │   ├── Typography.kt
│   │   └── TvFocusSurface.kt
│   ├── components/
│   │   ├── AsyncMediaArtwork.kt
│   │   ├── ContentRail.kt
│   │   ├── FeaturedHero.kt
│   │   ├── MediaPosterCard.kt
│   │   ├── ScreenStateLayout.kt
│   │   └── StreamingScaffold.kt
│   ├── home/
│   ├── details/
│   ├── channel/
│   ├── guide/
│   ├── pairing/
│   ├── settings/
│   └── player/
└── update/
```

Presentation reducers/models that do not depend on Android should live in
`android/core`, for example:

```text
android/core/src/main/kotlin/com/aleksclark/primer/tv/core/presentation/
├── HeroModel.kt
├── HomePresenter.kt
├── MediaCardModel.kt
├── RailModel.kt
└── UiText.kt
```

---

# Explicit Non-Goals

- No autoplaying trailer/video hero in the first redesign.
- No recommendation algorithm; parent/server curation remains authoritative.
- No search until the available catalog is large enough to justify it.
- No profile picker; this is still one student/device context.
- No social, ratings, comments, or algorithmic “because you watched” rows.
- No weakening of programmed playback or watch-once restrictions.
- No UI-side inference that content is available when the server says it is not.

---

# Definition of Done

The redesign is complete when:

1. Home visually follows a mainstream streaming hierarchy: navigation, hero,
   rails, detail, player.
2. Phone/tablet and TV share behavior and tokens but use form-appropriate
   layouts.
3. Every TV action is D-pad reachable with visible, restorable focus.
4. Every screen has stable loading, content, empty, and error states.
5. Playback controls exactly reflect and enforce policy.
6. Pairing, catalog, channel, guide, details, settings, and updates satisfy the
   BDD scenarios above.
7. Golden tests cover core form factors and states.
8. Physical tests pass on the Samsung phone and T9 box.
9. No regression occurs in grant, heartbeat, completion, watch-once,
   programmed-offset, or instructional-reporting behavior.
