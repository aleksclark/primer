# Fresh Android pairing QR evidence

- **Context:** existing isolated authenticated parent-A Chrome context (`phase2-tasks-schedules-call7-parent`), public route `/parent/students/a40db714-ba59-4f45-93fd-191aa41a6563`.
- **Live selectors:** roster **OPEN** button `uid=76_39`; student-detail **ISSUE PAIRING QR** button `uid=77_15`.
- **Observed result:** clicking the real parent UI action displayed a show-once, short-lived **One-use student pairing QR code**. The exact rendered UI is saved at `android-fresh-parent-qr.png`; its a11y evidence is `android-fresh-parent-qr.snapshot.txt`.
- **Network evidence:** public browser request `POST /api/students/a40db714-ba59-4f45-93fd-191aa41a6563/pairing` returned `200` (reqid 84). Console had no messages.
- **Handling:** pairing material was issued only through the parent UI. Its value is deliberately not transcribed into this note, URL, or source; no manual code entry, seed, mock, or private API call was used.

## Current QR for immediate Android acceptance

- **Issued at:** 2026-08-19T03:26:03-05:00 (browser exploration capture time).
- **Rendered expiry:** `AUG 19, 2026` (the public UI exposes a date, not a time-of-day).
- **Fresh exact screenshot:** `android-fresh-parent-qr-current.png`.
- **Fresh QR snapshot:** `android-fresh-parent-qr-current.snapshot.txt`.
- **Action:** existing authenticated parent-A context clicked **ISSUE A NEW CODE** (`uid=77_15`); public `POST /api/students/{student-id}/pairing` returned 200 (reqid 85).

The QR secret is not reproduced in this note. It is present only in the requested exact rendered screenshot/snapshot for the immediate Android run.

## Final QR for coordinated Android rerun

- **Issued at:** 2026-08-19T03:32:23-05:00.
- **Rendered expiry:** `AUG 19, 2026` (no time-of-day is exposed by this UI).
- **Exact screenshot:** `android-qr-final.png`.
- **Snapshot:** `android-qr-final.snapshot.txt`.
- Existing authenticated parent-A Chrome context remains open on the rendered QR page. The action was **ISSUE A NEW CODE** (`uid=77_15`); public pairing request reqid 86 returned 200.

No QR secret was transcribed or manually entered.
