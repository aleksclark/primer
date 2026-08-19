# Started Android occurrence identification

The Android app was paired through the exact final QR and the third visible **Today** card was opened while it showed `pending`, then started. Its subsequent detail showed `Status: awaiting_verification` (`07-pending-detail.accessibility.txt`, `08-started-awaiting-verification.accessibility.txt`).

The Android client fetches `/device/today`, whose public server query is ordered by `nominal_at` ascending (`primer-tasks/internal/api/phase2.go`, `listDeviceToday`). Its live initial order was:

1. `completed`
2. `awaiting_verification`
3. `pending` — selected and started
4. `excused`

The public parent state associates the first two chronological active recurrence rows with:

- Aug 20, 2026 2:00 PM CDT: `22f44c2c-70db-4e28-aee5-aee3ef5282f3`, already awaiting verification.
- Aug 21, 2026 2:00 PM CDT: `30f929a3-e6d8-4038-a149-926cae3cb4f5`.

Therefore the Android-started third Today card is the **Aug 21, 2026 2:00 PM CDT occurrence, ID `30f929a3-e6d8-4038-a149-926cae3cb4f5`**. This was identified only from the Android UI ordering and public implementation/state evidence; no direct API request or token inspection was performed.
