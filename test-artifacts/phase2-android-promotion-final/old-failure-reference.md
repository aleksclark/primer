# Prior failure reference (concise)

- `0b7b1163`: failed QR pairing after using a stale/wrong origin.
- This run first recorded `make tasks-endpoints` and proceeded only after the live loopback web origin was `http://127.0.0.1:37952`, using exactly `http://10.0.2.2:37952` for the current APK build.
