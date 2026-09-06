import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, cp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync, execFileSync } from "node:child_process";

const here = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
test("boundary gate rejects parallel sockets/fetch/copied DTOs and tracked generated output", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "primer-tasks-boundary-"));
  try {
    const fixture = path.join(root, "primer-tasks/clients/typescript");
    await mkdir(path.join(fixture, "scripts"), { recursive: true });
    for (const directory of ["src", "generated"]) await cp(path.join(here, directory), path.join(fixture, directory), { recursive: true });
    await cp(path.join(here, "scripts/check-agent-boundary.mjs"), path.join(fixture, "scripts/check-agent-boundary.mjs"));
    const env = { ...process.env, GIT_OPTIONAL_LOCKS: "0", GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" };
    execFileSync("git", ["init", "-q", root], { env });
    const check = () => spawnSync(process.execPath, [path.join(fixture, "scripts/check-agent-boundary.mjs")], { cwd: fixture, encoding: "utf8", env });
    assert.equal(check().status, 0, "real facade/outputs must satisfy the copied gate");
    const rogue = path.join(fixture, "src/rogue.ts");
    for (const source of ['new WebSocket("wss://example.invalid");', 'fetch("/tasks/api/student/profile");', 'export interface StudentDialogueEvent { kind: string }']) {
      await writeFile(rogue, source);
      assert.notEqual(check().status, 0, "boundary mutation must fail");
      await rm(rogue);
    }
    execFileSync("git", ["-C", root, "add", "--force", "primer-tasks/clients/typescript/generated/student-dialogue.ts"], { env });
    const result = check();
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /must not be tracked/);
  } finally { await rm(root, { recursive: true, force: true }); }
});
