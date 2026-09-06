#!/usr/bin/env python3
"""Small CDP probe for the real Vite HMR proof.

This deliberately uses Chrome DevTools Protocol rather than Playwright. It
navigates once, observes a source-backed DOM marker, and rejects any
Page.frameNavigated event after the baseline was established.
"""
from __future__ import annotations

import json
import os
import sys
import time
import urllib.request
from pathlib import Path

try:
    import websocket  # type: ignore[import-not-found]
except ImportError:
    print("hmr-proof: websocket-client Python module is required", file=sys.stderr)
    raise SystemExit(2)


def get_json(url: str) -> object:
    with urllib.request.urlopen(url, timeout=3) as response:
        return json.load(response)


def main() -> int:
    if len(sys.argv) != 6:
        print("usage: prove-hmr.py CDP_PORT URL BASELINE CHANGED READY_FILE", file=sys.stderr)
        return 2
    port, url, baseline, changed, ready_file = sys.argv[1:]
    deadline = time.monotonic() + 30

    targets = get_json(f"http://127.0.0.1:{port}/json/list")
    pages = [target for target in targets if target.get("type") == "page" and target.get("webSocketDebuggerUrl")]
    if not pages:
        print("hmr-proof: Chrome exposed no page target", file=sys.stderr)
        return 1

    os.environ.setdefault("NO_PROXY", "127.0.0.1,localhost")
    os.environ.setdefault("no_proxy", "127.0.0.1,localhost")
    ws = websocket.create_connection(
        pages[0]["webSocketDebuggerUrl"],
        timeout=1,
        http_proxy_host=None,
        http_proxy_port=None,
    )
    sequence = 0
    navigations = 0

    def receive() -> dict:
        nonlocal navigations
        while True:
            message = json.loads(ws.recv())
            if message.get("method") == "Page.frameNavigated":
                navigations += 1
            return message

    def command(method: str, params: dict | None = None) -> dict:
        nonlocal sequence
        sequence += 1
        command_id = sequence
        ws.send(json.dumps({"id": command_id, "method": method, "params": params or {}}))
        command_deadline = min(deadline, time.monotonic() + 5)
        while time.monotonic() < command_deadline:
            try:
                message = receive()
            except Exception:
                continue
            if message.get("id") == command_id:
                return message
        raise TimeoutError(f"timed out waiting for CDP command {method}")

    def marker() -> str:
        response = command(
            "Runtime.evaluate",
            {
                "expression": "document.querySelector('[data-hmr-proof-marker]')?.getAttribute('data-hmr-proof-marker') || ''",
                "returnByValue": True,
            },
        )
        return str(response.get("result", {}).get("result", {}).get("value", ""))

    try:
        command("Page.enable")
        command("Runtime.enable")
        command("Page.navigate", {"url": url})
        while time.monotonic() < deadline:
            if marker() == baseline:
                # The initial navigation is not part of the HMR assertion.
                navigations = 0
                Path(ready_file).write_text("ready\n", encoding="utf-8")
                break
            time.sleep(0.2)
        else:
            print("hmr-proof: baseline DOM marker did not appear", file=sys.stderr)
            return 1

        while time.monotonic() < deadline:
            current = marker()
            if current == changed:
                if navigations:
                    print("hmr-proof: marker changed only after a full navigation", file=sys.stderr)
                    return 1
                print("hmr-proof: PASS marker changed without navigation")
                return 0
            time.sleep(0.2)
        print(f"hmr-proof: timed out waiting for marker {changed!r}", file=sys.stderr)
        return 1
    finally:
        ws.close()


if __name__ == "__main__":
    raise SystemExit(main())
