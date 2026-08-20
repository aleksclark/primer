# Phase 5 media — Terra final exploratory and promotion PASS

## Real-stack scope

- Rebuilt real Stacklane Compose at the public web origin with PostgreSQL, MinIO/S3 artifact storage, ffprobe 8.1.2, and the deterministic scripted Fantasy fixture enabled only for its checked-in image digest.
- The scripted fixture establishes image-flow wiring only. It is not a live multimodal-quality claim; valid audio and video correctly remain explicit Parent review.
- The StudentArtifactPage HMR update was observed before the rerun: A/V browser metadata is no longer used to reject an upload before server finalize.

## Fresh Chrome exploration

1. Parent UI created a fresh student; authored a video-only artifact rubric; created, published, and scheduled v1; issued a fresh pairing code; and paired a new student tab.
2. The student started the scheduled occurrence, selected a truncated MP4, and finalized it. The server authoritative ffprobe/decoder path rejected it as invalid.
3. On that exact occurrence, **Submit a new file** accepted a checked-in valid MP4. It reached **Parent review** with local video preview and no chat.
4. Authorized read-only database inspection confirmed lifecycle cleanup plus replacement: one rejected/canceled failed artifact/reservation and one finalized/finalized replacement.
5. Network from the user flow stayed at the same public origin for reserve, bounded upload, and finalize. The independent fresh post-Playwright Chrome tab redrove the persisted video occurrence and displayed **Parent review**. Its console had no errors or warnings and its API reads were all 200.

## Playwright promotion

Focused real-stack command:

```text
PRIMER_TASKS_BASE_URL=http://web.primer-tasks-p5.primer-tasks.test:5173 CHROME_EXECUTABLE=/home/aleks/.local/bin/google-chrome npm --prefix primer-tasks/web run browser:promoted -- e2e/p5-media.spec.ts
```

Result: **2 passed** (image completion/retry/CSP, and audio/video local-preview Parent review). The promoted spec was checked before execution: no skips, `only`, hollow assertions, `waitForTimeout`, or route mocks.

## Sanitization and limits

Raw browser snapshots, pairing material, media previews, request logs, generated Playwright JSON/results, IDs, and bulky files were removed. The retained evidence is sanitized text only and contains no storage endpoint, credential material, signed query, or reasoning disclosure.

This PASS covers the repaired same-occurrence truncated-video retry, focused media suite, and independent Chrome redrive. No live-model-quality claim is made.
