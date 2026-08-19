# Curated dialogue fixture: chapter 4

This deterministic fixture proves the wiring and policy boundary; it is not a
claim about educational model quality.

- **Source:** `fixture://chapter-4`
- **Required distinct accepted questions:** 3
- **Concepts:** central conflict, textual evidence, cause and consequence
- **Retry policy:** one follow-up per insufficient answer; eight maximum turns

The scripted fixture accepts answers containing the relevant concept vocabulary,
rejects an insufficient answer, and records only a short safe rationale. The
server remains authoritative for question distinctness, accepted counting,
idempotency, and the terminal decision.
