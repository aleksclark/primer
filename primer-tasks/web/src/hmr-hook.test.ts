/// <reference types="node" />
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

test("hot-reload proofs share a source-backed attribute with no announced marker text", () => {
  const app = readFileSync(new URL("./App.tsx", import.meta.url), "utf8");
  const baseline = app.match(/const HMR_PROOF_MARKER = "([^"]+)";/)?.[1];
  assert.equal(baseline, "tasks-source-baseline");
  assert.match(app, /<span data-hmr-proof-marker=\{HMR_PROOF_MARKER\} aria-hidden="true" \/>/);
  assert.doesNotMatch(app, /System C|HMR baseline|Explore \+ Configure|server-owned|tenant-scoped/);
  for (const name of ["prove-host.sh", "prove-dev.sh"]) {
    const proof = readFileSync(new URL(`../../scripts/${name}`, import.meta.url), "utf8");
    assert.ok(proof.includes(baseline!));
    assert.doesNotMatch(proof, /System C/);
  }
  const cdp = readFileSync(new URL("../../scripts/prove-hmr.py", import.meta.url), "utf8");
  assert.match(cdp, /getAttribute\('data-hmr-proof-marker'\)/);
  assert.doesNotMatch(cdp, /textContent/);
  assert.match(cdp, /if navigations:/); // Still rejects a full document navigation.
});
