# Content Library — Master Acquisition List

Consolidated video-library plan for the virtual TV channel (see `../tv-channel.md` and `../plans/video-as-instruction.md`).

**Screening criteria (finalized):**
- ✅ Moderate language OK
- ✅ Violence OK (war films, westerns, battle scenes)
- ❌ Progressive/leftist ideological framing is the disqualifier

Detail files in this directory:
- `science.md` — 26 titles: life / earth-space / physical science (TN grade 6)
- `history-engineering.md` — 20 titles: ancient civilizations + engineering
- `practical-skills.md` — 30 titles: woodworking, cooking, homesteading, competence
- `entertainment-music-arts.md` — 47 titles: classic films, series, music & arts

## Channel category coverage

| TV channel category | Source file | Approx. titles |
|---|---|---|
| Educational documentary | science + history-engineering | ~46 |
| Practical skills | practical-skills | ~30 |
| Quality entertainment | entertainment (films + series) | ~37 |
| Music & arts | entertainment §3 | ~10 |
| Current events | *unsourced — needs a curation strategy, see open items* | 0 |

## Acquisition priority — start with these 12

1. **The Living Planet** (1984) — ecosystems backbone; grade 6 life science
2. **Engineering an Empire** (2005–07) — history+engineering twofer; matches the TN civilization sequence
3. **The Woodwright's Shop** early seasons — hand-tool discipline before/during the coop build
4. **Good Eats** (1999–2012) — food-science anchor; tutor launches fractions/chemistry from it
5. **Victorian Farm** + **Tales from the Green Valley** — history + homesteading + craftsmanship in one
6. **Cosmos: A Personal Voyage** (1980) — astronomy + history of science (preferred over the 2014 remake — see ideology flags)
7. **The Secret Life of Machines** (1988–93) — physical science in perfect 25-min slots
8. **Leonard Bernstein's Young People's Concerts** (Kultur DVDs) — the music & arts anchor
9. **PBS Empires collection** — maps onto the TN civilization sequence incl. ancient Israel (*Kingdom of David*)
10. **Wild Weather** + curated **NOVA** weather eps — pre-teach for the weather station (Q3)
11. **Hornblower** (1998–2003) — evening series anchor
12. **Tier A classic films** (Robin Hood '38, Great Escape, Shane, The General, Court Jester, Ben-Hur…) — evening rotation, watch-once ledger

## Project → content map

| Project | Pre/during viewing |
|---|---|
| Garden (Q1) | Private Life of Plants, Gardeners' World, Victory Garden, Life in the Undergrowth, Clarkson's Farm |
| Chicken coop (Q2) | Justin Rhodes archive, Life of Birds, Woodwright's Shop, Essential Craftsman, All Creatures Great and Small |
| Weather station (Q3) | Wild Weather, NOVA weather eps, Mr. Wizard's World, Earth: The Power of the Planet, MythBusters |
| Family cookbook (Q4) | Good Eats, Jacques Pépin, America's Test Kitchen, The French Chef, Townsends, Baking with Julia |

## Ideology screening results

**Flagged / restricted:**
- **Cosmos: A Spacetime Odyssey (2014)** — anti-religion framing (Bruno cartoon, ep 1) + climate-advocacy episodes; use selected middle eps or prefer Sagan
- **Post-2010 Attenborough** (Our Planet, A Life on Our Planet, Planet Earth III) — climate-advocacy framing; excluded. Pre-2010 catalog is pure natural history
- **Blue Planet II ep 7** — advocacy episode; skip (eps 1–6 fine)
- **Legacy (1991) ep 6** — "West as aberration" framing; eps 1–5 excellent
- **The West (1996)** — co-view for guilt-narrative moments; otherwise even-handed
- **NOVA** — avoid explicit climate-policy episodes; science eps fine

**Reinstated under revised criteria (previously flagged for language/violence):**
- **Clarkson's Farm** — language fine now; ideologically simpatico (skewers green bureaucracy); air the run
- **Dirty Jobs** — dignity-of-work message on-mission; air broadly
- **MythBusters** — air broadly, skip adult-myth eps only
- **Ben-Hur, Zulu, Patton, Kelly's Heroes, The Dirty Dozen** — violence/language waived; added to film lists
- **Ken Burns Jazz** — vice treated honestly; air the run

**Still held back (neither language nor violence — unaddressed by revised criteria):**
- **Sharpe** — love scenes/nudity; per-episode screening or hold to ~14

## Open items
1. **Current events** — still unsourced; candidate approach: parent-clipped weekly segments rather than any live feed.
2. **Acquisition mechanics** — out of scope for the TV server (guideline 11); separate effort. yt-dlp targets: Paul Sellers, Essential Craftsman, Justin Rhodes, Townsends, Primitive Technology, Time Team official, Caltech Mechanical Universe, Hunkin channels, GBH Open Vault Victory Garden. DVD rip list: everything in history-engineering §acquisition notes plus Attenborough/Bernstein/classic-film sets.
3. **Direct-play validation** — everything ripped/archived checked H.264/H.265 for the RK3318 box before import.
4. **Standards tagging** — on import, set `subject_tags` + `standard_codes` so instructional hours land correctly in the LMS (Attenborough → life science; Stewart/Cox/Sagan → earth-space; Hunkin/Mechanical Universe → physical; Wood/Empires/Macaulay → social studies).
