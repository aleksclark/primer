import { test as base, expect, type BrowserContext, type Page, type TestInfo } from "@playwright/test";
import { execFile, execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const tasksRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const tools = "list_students,list_tasks,get_task,draft_task,update_task,publish_task,preview_action,confirm_action,list_schedules,create_schedule,update_schedule,list_occurrences";
const stateDir = resolve(process.env.TASKS_HOST_STATE_DIR ?? join(tasksRoot, "tmp/host-stack"));
const hostName = process.env.TASKS_HOST_NAME ?? `primer-tasks-host-${basename(dirname(tasksRoot))}`;
const endpoints = (): Record<string, string> => Object.fromEntries(readFileSync(join(stateDir, "endpoints.env"), "utf8").trim().split("\n").map(line => line.split("=")));

// Never operate on a donor/fleet/database URL. The only process and SELECT
// controls below address the named disposable host-stack from this worktree.
function assertOwnedFixture(baseURL?: string) {
  if (!stateDir.startsWith(join(tasksRoot, "tmp") + "/") || !/^primer-[a-z0-9-]+$/.test(hostName)) throw new Error("P3 requires a named worktree-local disposable host fixture");
  const urls = endpoints();
  for (const url of Object.values(urls)) if (!/^http:\/\/127\.0\.0\.1:\d+\/$/.test(url)) throw new Error("P3 fixture endpoints must be loopback only");
  if (baseURL && new URL(baseURL).origin !== new URL(urls.PRIMER_TASKS_BASE_URL).origin) throw new Error("P3 baseURL does not match the owned fixture endpoints");
  const health = execFileSync("docker", ["inspect", "-f", "{{.State.Health.Status}}", `${hostName}-pg`], { encoding: "utf8" }).trim();
  if (health !== "healthy") throw new Error("Own PostgreSQL fixture is not ready");
}

export function configure(overrides: Record<string, string> = {}, action: "restart-api" | "crash-api" = "restart-api") {
  assertOwnedFixture();
  const before = readFileSync(join(stateDir, "api.pid"), "utf8").trim();
  try {
    execFileSync(join(tasksRoot, "scripts/host-stack"), [action], {
      cwd: tasksRoot,
      env: { ...process.env, TASKS_HOST_NAME: hostName, TASKS_HOST_STATE_DIR: stateDir, TASKS_MODEL_PROVIDER: "scripted", TASKS_AGENT_MODE: "scripted", TASKS_AGENT_ACTIVE_TOOLS: tools, TASKS_AGENT_SCRIPTED_DELAY_MS: "1500", ...overrides },
      stdio: "pipe",
    });
  } catch { throw new Error(`Owned host-stack ${action} failed; no raw process buffers exported`); }
  const after = readFileSync(join(stateDir, "api.pid"), "utf8").trim();
  expect(after).not.toBe(before);
  return { action, beforePID: before, afterPID: after };
}

export function readOnlySQL(query: string) {
  if (!query.trimStart().startsWith("SELECT ") || query.includes(";")) throw new Error("Fixture observer permits one SELECT only");
  return execFileSync("docker", ["exec", `${hostName}-pg`, "psql", "-U", "tasks", "-d", "primer_tasks", "-Atc", query], { encoding: "utf8", stdio: "pipe" }).trim();
}

// Armed BEFORE the UI sends its unique command. Unlike the inherited test, this
// kills immediately upon observing RUNNING, not after an arbitrary UI roundtrip.
export function crashWhenRunning(marker: string): Promise<{ runId: string; observed: string; action: string; beforePID: string; afterPID: string }> {
  assertOwnedFixture();
  if (!/^[a-zA-Z0-9 -]+$/.test(marker)) throw new Error("Unsafe fixture marker");
  const query = `SELECT r.id::text FROM agent_runs r JOIN agent_messages m ON m.id=r.user_message_id WHERE r.status='running' AND m.content LIKE '%${marker}%'`;
  return new Promise((resolveRun, reject) => {
    let stopped = false;
    const deadline = setTimeout(() => { stopped = true; reject(new Error("No matching RUNNING job observed within the 20s admission bound")); }, 20_000);
    const inspect = () => {
      execFile("docker", ["exec", `${hostName}-pg`, "psql", "-U", "tasks", "-d", "primer_tasks", "-Atc", query], { encoding: "utf8" }, (error, stdout) => {
        if (stopped) return;
        if (error) { stopped = true; clearTimeout(deadline); reject(new Error("Own read-only running-job observation failed")); return; }
        const rows = stdout.trim().split("\n").filter(Boolean);
        if (rows.length) {
          stopped = true; clearTimeout(deadline);
          if (rows.length !== 1) { reject(new Error("Running marker must identify exactly one job")); return; }
          try { resolveRun({ runId: rows[0], observed: "running", ...configure({ TASKS_AGENT_SCRIPTED_DELAY_MS: "2500" }, "crash-api") }); } catch (e) { reject(e); }
        } else setTimeout(inspect, 50); // Poll the real state condition, never sleep to infer success.
      });
    };
    inspect();
  });
}

export interface SafeEvent {
  kind: string; protocol: number; cursor: number; sequence: number;
  runId?: string; conversationId?: string; clientMessageId?: string;
  source?: string; text?: string; code?: string; status?: string;
  phase?: string; label?: string; summary?: string; confirmationHandlePresent?: boolean;
}
interface DomainRow { id: string; templateId: string; title: string; status: string; templateStatus?: string; version: number; studentId?: string; studentName?: string; enabled?: boolean }
interface Collection { items: DomainRow[]; totalCount: number }
interface BrowserProbe {
  Native: typeof WebSocket; sockets: WebSocket[]; events: SafeEvent[]; handles: Record<string, string>;
  subscriptions: { cursor: number; conversationId?: string }[];
  unsafe: boolean; invalidJSON: number; keys: string[];
  receiptReads: Record<string, { tasks: number; schedules: number } | "pending" | "failed">;
}
declare global { interface Window { __p3: BrowserProbe } }

export async function installObserver(context: BrowserContext) {
  await context.addInitScript(() => {
    const Native = window.WebSocket;
    const state: BrowserProbe = { Native, sockets: [], events: [], handles: {}, subscriptions: [], unsafe: false, invalidJSON: 0, keys: [], receiptReads: {} };
    window.__p3 = state;
    // Transparent native observer. No headers/protocol constructor arguments
    // are retained, no frames are changed, and probes below use Native itself.
    window.WebSocket = new Proxy(Native, { construct(Target, args) {
      const socket = Reflect.construct(Target, args) as WebSocket;
      if (new URL(String(args[0]), location.href).pathname === "/api/ws") {
        state.sockets.push(socket);
        const send = socket.send.bind(socket);
        socket.send = (data) => {
          try { const input = JSON.parse(String(data)); if (input.kind === "subscribe") state.subscriptions.push({ cursor: input.cursor ?? 0, conversationId: input.conversationId }); } catch { /* unrelated binary traffic is not recorded */ }
          return send(data);
        };
        socket.addEventListener("message", event => {
          try {
            const raw = String(event.data), x = JSON.parse(raw);
            state.unsafe ||= /"hidden"|OnReasoningDelta|chain.of.thought|primer-tasks.v1.bearer|primer-tasks.v1.csrf/.test(raw);
            state.keys = [...new Set([...state.keys, ...Object.keys(x)])];
            const safe = Object.fromEntries(["kind", "protocol", "cursor", "sequence", "runId", "conversationId", "clientMessageId", "source", "text", "code", "status", "phase", "label", "summary"].filter(k => x[k] !== undefined).map(k => [k, x[k]])) as SafeEvent;
            if (x.confirmationId) { state.handles[x.runId] = x.confirmationId; safe.confirmationHandlePresent = true; }
            state.events.push(safe);
            if (x.kind === "text_start" && x.source === "domain") {
              state.receiptReads[x.runId] = "pending";
              // First domain frame must not precede externally visible effects.
              Promise.all(["/api/tasks?status=all&view=templates", "/api/schedules?status=all"].map(async path => { const r = await fetch(path); if (!r.ok) throw new Error("Public receipt read failed"); return (await r.json()).totalCount as number; }))
                .then(([tasks, schedules]) => { state.receiptReads[x.runId] = { tasks, schedules }; })
                .catch(() => { state.receiptReads[x.runId] = "failed"; });
            }
          } catch { state.invalidJSON++; }
        });
      }
      return socket;
    } });
  });
}

function sanitized(text: string) {
  return text.replace(/primer-tasks\.v1\.(?:csrf|bearer)\.[^\s'"\]]+/g, "[credential protocol redacted]").replace(/([?&](?:code|state|token)=)[^&\s]+/g, "$1[redacted]");
}
export class Diagnostics {
  private deliberate: "owned-api-restart" | "missing-csrf" | "opaque-origin" | undefined;
  private expected = 0;
  readonly unexpected: string[] = [];
  readonly receipts: { name: string; expectedErrors: number }[] = [];
  watch(page: Page) {
    page.on("pageerror", error => this.unexpected.push(`pageerror: ${sanitized(error.message)}`));
    page.on("console", message => {
      if (message.type() !== "error" && message.type() !== "warning") return;
      const text = sanitized(message.text());
      const ownSocket = /WebSocket connection to 'ws:\/\/127\.0\.0\.1:\d+\/(?:api\/)?ws' failed:/.test(text);
      const restart = this.deliberate === "owned-api-restart" && ownSocket && /Connection closed before receiving a handshake response|net::ERR_CONNECTION_REFUSED|net::ERR_EMPTY_RESPONSE/.test(text);
      const csrf = this.deliberate === "missing-csrf" && ownSocket && text.includes("Unexpected response code: 403");
      const origin = this.deliberate === "opaque-origin" && ownSocket && text.includes("Unexpected response code: 403");
      if (restart || csrf || origin) this.expected++; else this.unexpected.push(text);
    });
    page.on("response", response => { if (response.status() >= 400) this.unexpected.push(`HTTP ${response.status()} ${new URL(response.url()).pathname}`); });
    page.on("requestfailed", request => {
      // An explicit document navigation may abort a superseded read. It must
      // not hide service failures or WS malformed-close errors.
      const reason = request.failure()?.errorText ?? "unknown";
      this.unexpected.push(`requestfailed ${new URL(request.url()).pathname}: ${sanitized(reason)}`);
    });
  }
  async expectFault<T>(name: "owned-api-restart" | "missing-csrf" | "opaque-origin", operation: () => Promise<T>): Promise<T> {
    if (this.deliberate) throw new Error("Expected fault scopes must not overlap");
    this.deliberate = name; this.expected = 0;
    try { return await operation(); } finally { this.receipts.push({ name, expectedErrors: this.expected }); this.deliberate = undefined; }
  }
  assertClean() { expect(this.unexpected, "No unexpected console/page/network errors (malformed close is NEVER allowlisted)").toEqual([]); }
}

export async function publicRead(page: Page, path: string): Promise<Collection> {
  return page.evaluate(async path => { const r = await fetch(path); if (!r.ok) throw new Error(`Public read ${new URL(path, location.href).pathname} returned ${r.status}`); return r.json(); }, path);
}
async function publicMutation(page: Page, method: string, path: string) {
  return page.evaluate(async ({ method, path }) => { const r = await fetch(path, { method, headers: { "Content-Type": "application/json" } }); if (!r.ok) throw new Error(`Public fixture normalization returned ${r.status}`); await r.arrayBuffer(); return r.status; }, { method, path });
}
export async function counts(page: Page) {
  const tasks = await publicRead(page, "/api/tasks?status=all&view=templates");
  const schedules = await publicRead(page, "/api/schedules?status=all");
  return { tasks: tasks.totalCount, schedules: schedules.totalCount };
}

// Scripted disable/retire chooses the sole active domain record. Normalize only
// this OWN disposable fixture through ordinary parent APIs; never delete SQL or
// depend on phase1/phase2 having run. Retired history/issued work remain intact.
async function normalizeDisposableParent(page: Page) {
  assertOwnedFixture(page.url());
  for (const [path, method, target] of [
    ["/api/schedules?status=active&limit=100", "DELETE", "/api/schedules/"],
    ["/api/tasks?status=active&view=templates&limit=100", "POST", "/api/tasks/"],
  ]) {
    let rows = await publicRead(page, path);
    while (rows.items.length) {
      for (const row of rows.items) await publicMutation(page, method, `${target}${method === "POST" ? row.templateId + "/retire" : row.id}`);
      rows = await publicRead(page, path);
    }
    expect(rows.totalCount).toBe(0);
  }
}
export async function signIn(page: Page, parent: "A" | "B") {
  await page.goto("/parent/students");
  const login = page.getByRole("button", { name: "Continue with parent sign-in", exact: true });
  const heading = page.getByRole("heading", { name: "Students", exact: true });
  await expect(login.or(heading)).toBeVisible();
  if (await login.isVisible()) { await login.click(); await page.getByRole("link", { name: `Parent ${parent}`, exact: true }).click(); }
  await expect(heading).toBeVisible();
}
export async function createStudent(page: Page, name: string) {
  await page.goto("/parent/students");
  await page.getByRole("button", { name: "Add student", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add student", exact: true });
  await expect(dialog.getByLabel("Display name", { exact: true })).toBeFocused();
  await dialog.getByLabel("Display name", { exact: true }).fill(name);
  await dialog.getByRole("button", { name: "Save student", exact: true }).click();
  await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
}
export async function renameStudent(page: Page, oldName: string, newName: string) {
  await page.goto("/parent/students");
  await page.getByRole("button", { name: oldName, exact: true }).click();
  await page.getByRole("button", { name: "Edit name", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Edit student", exact: true });
  await dialog.getByLabel("Display name", { exact: true }).fill(newName);
  await dialog.getByRole("button", { name: "Save student", exact: true }).click();
  await expect(page.getByRole("heading", { name: newName, exact: true, level: 1 })).toBeVisible();
}
export const runState = (page: Page) => page.getByRole("complementary", { name: "Agent run boundaries" }).locator("dl > div").filter({ has: page.locator("dt", { hasText: /^Run state$/ }) }).locator("dd");
export const confirmed = (page: Page) => page.locator(".agent-entry").filter({ has: page.locator(".system-label", { hasText: /^Confirmed result$/ }) }).locator("p");
export const confirmButton = (page: Page) => page.getByRole("button", { name: "Confirm preview", exact: true });
export const connected = (page: Page) => expect(page.locator(".page-actions [role=status]")).toHaveText("Connected");
export async function openAgent(page: Page) { await page.goto("/parent/agent"); await expect(page.getByRole("heading", { name: "Parent agent", exact: true })).toBeVisible(); await connected(page); }
export async function events(page: Page, runId?: string): Promise<SafeEvent[]> { return page.evaluate(id => window.__p3.events.filter(x => !id || x.runId === id), runId); }
export async function terminal(page: Page, runId: string, status: string, timeout = 10_000) {
  await expect.poll(async () => (await events(page, runId)).filter(x => x.kind === "terminal").map(x => x.status), { timeout }).toEqual([status]);
  return (await events(page, runId)).find(x => x.kind === "terminal")!;
}
export async function command(page: Page, text: string) {
  const cursor = Math.max(0, ...(await events(page)).map(x => x.cursor));
  await expect(page.getByLabel("Parent command", { exact: true })).toBeEnabled();
  await page.getByLabel("Parent command", { exact: true }).fill(text);
  await page.getByRole("button", { name: "Send command", exact: true }).click();
  await expect.poll(async () => (await events(page)).filter(x => x.kind === "user_message" && x.cursor > cursor && x.text === text).length).toBe(1);
  return (await events(page)).find(x => x.kind === "user_message" && x.cursor > cursor && x.text === text)!.runId!;
}
export async function pending(page: Page, runId: string) {
  await expect.poll(async () => (await events(page, runId)).filter(x => x.phase === "awaiting_confirmation" && x.confirmationHandlePresent).length).toBe(1);
  await expect(confirmButton(page)).toBeVisible();
  await expect(runState(page)).toHaveText("awaiting confirmation");
  return (await events(page, runId)).find(x => x.phase === "awaiting_confirmation")!;
}
export async function receipt(page: Page, runId: string, expectedText: string | RegExp, clauses?: number) {
  await terminal(page, runId, "completed");
  const domain = (await events(page, runId)).filter(x => x.source === "domain");
  expect(domain.filter(x => x.kind === "text_start")).toHaveLength(1);
  const deltas = domain.filter(x => x.kind === "text_delta");
  if (clauses !== undefined) expect(deltas).toHaveLength(clauses); else expect(deltas.length).toBeGreaterThan(0);
  const ends = domain.filter(x => x.kind === "text_end");
  expect(ends).toHaveLength(1);
  expect(deltas.map(x => x.text).join("")).toBe(ends[0].text);
  if (typeof expectedText === "string") expect(ends[0].text).toContain(expectedText); else expect(ends[0].text).toMatch(expectedText);
  await expect(confirmed(page).filter({ hasText: ends[0].text! })).toHaveCount(1);
  return ends[0].text!;
}
export async function noReceipt(page: Page, runId: string) { expect((await events(page, runId)).filter(x => x.source === "domain")).toEqual([]); }

export async function handleProbe(page: Page, requests: { runId: string; mode: "same" | "altered" }[]) {
  return page.evaluate(async requests => {
    const p = window.__p3;
    const csrf = document.cookie.split(";").map(x => x.trim()).find(x => x.startsWith("tasks_csrf="))?.slice(11);
    if (!csrf) throw new Error("Fixture CSRF input missing");
    return new Promise<string[]>((resolveProbe, reject) => {
      const s = new p.Native(`ws://${location.host}/api/ws`, ["primer-tasks.v1", `primer-tasks.v1.csrf.${csrf}`]);
      const received: string[] = [];
      const timer = setTimeout(() => { s.close(); reject(new Error("Ordered negative-command sentinel missing")); }, 5_000);
      s.onopen = () => {
        for (const request of requests) {
          const handle = p.handles[request.runId];
          if (!handle) { clearTimeout(timer); s.close(); reject(new Error("Observed preview handle missing")); return; }
          s.send(JSON.stringify({ protocol: 1, kind: "confirm", runId: request.runId, confirmationId: request.mode === "altered" ? (handle[0] === "a" ? "b" : "a") + handle.slice(1) : handle }));
        }
        s.send(JSON.stringify({ protocol: 1, kind: "unknown_probe" }));
      };
      s.onmessage = e => { const x = JSON.parse(String(e.data)); received.push(x.kind); if (x.code === "unknown_command") { clearTimeout(timer); s.close(); resolveProbe(received); } };
    });
  }, requests);
}

export async function restartWithReaders(diagnostics: Diagnostics, pages: Page[], overrides: Record<string, string> = {}) {
  return diagnostics.expectFault("owned-api-restart", async () => {
    const generations = await Promise.all(pages.map(p => p.evaluate(() => window.__p3.sockets.length)));
    const result = configure(overrides);
    for (const [index, page] of pages.entries()) {
      await expect.poll(() => page.evaluate(() => window.__p3.sockets.length)).toBeGreaterThan(generations[index]);
      await connected(page);
    }
    return result;
  });
}

export async function replay(page: Page) {
  return page.evaluate(async () => {
    const p = window.__p3, conversationId = p.events.find(x => x.conversationId)?.conversationId;
    const target = Math.max(...p.events.map(x => x.cursor));
    const csrf = document.cookie.split(";").map(x => x.trim()).find(x => x.startsWith("tasks_csrf="))?.slice(11);
    return new Promise<{ events: SafeEvent[]; target: number; unsafe: boolean }>((resolveReplay, reject) => {
      const s = new p.Native(`ws://${location.host}/api/ws`, ["primer-tasks.v1", `primer-tasks.v1.csrf.${csrf}`]);
      const out: SafeEvent[] = []; let unsafe = false;
      const timer = setTimeout(() => { s.close(); reject(new Error("Public replay did not reach durable cursor")); }, 8_000);
      s.onopen = () => s.send(JSON.stringify({ protocol: 1, kind: "subscribe", conversationId, cursor: 0 }));
      s.onmessage = e => {
        const raw = String(e.data), x = JSON.parse(raw);
        unsafe ||= /"hidden"|OnReasoningDelta|primer-tasks.v1.bearer|primer-tasks.v1.csrf/.test(raw);
        if (!x.cursor) return;
        out.push({ protocol: x.protocol, kind: x.kind, cursor: x.cursor, sequence: x.sequence, runId: x.runId, source: x.source, status: x.status, code: x.code, text: x.kind.startsWith("text_") || x.kind === "terminal" ? x.text : undefined });
        s.send(JSON.stringify({ protocol: 1, kind: "ack", cursor: x.cursor }));
        if (x.cursor === target) { clearTimeout(timer); s.close(); resolveReplay({ events: out, target, unsafe }); }
      };
    });
  });
}

export interface P3Fixture { a: Page; edit: Page; b: Page; diagnostics: Diagnostics; suffix: string; apiURL: string; info: TestInfo }
export const test = base.extend<{ p3: P3Fixture }>({
  p3: async ({ browser, baseURL }, use, info) => {
    assertOwnedFixture(baseURL);
    if (!process.env.CHROME_EXECUTABLE || /headless[-_]shell/.test(process.env.CHROME_EXECUTABLE)) throw new Error("P3 requires full Chrome via CHROME_EXECUTABLE, never headless-shell");
    configure();
    const contexts = await Promise.all([browser.newContext({ baseURL, viewport: { width: 1440, height: 1000 } }), browser.newContext({ baseURL })]);
    const diagnostics = new Diagnostics();
    try {
      for (const context of contexts) await installObserver(context);
      const a = await contexts[0].newPage(), b = await contexts[1].newPage();
      await signIn(a, "A"); await signIn(b, "B");
      diagnostics.watch(a); diagnostics.watch(b);
      await normalizeDisposableParent(a);
      const edit = await contexts[0].newPage(); diagnostics.watch(edit);
      await openAgent(a);
      const suffix = `${Date.now().toString(36)}-${info.testId.replace(/[^a-z0-9]/gi, "").slice(-6)}`;
      await use({ a, b, edit, diagnostics, suffix, apiURL: endpoints().PRIMER_TASKS_API_URL, info });
      diagnostics.assertClean();
      const safe = await a.evaluate(() => ({ unsafe: window.__p3.unsafe, invalidJSON: window.__p3.invalidJSON, keys: window.__p3.keys }));
      expect(safe.unsafe).toBe(false); expect(safe.invalidJSON).toBe(0);
      expect(safe.keys).not.toContain("reasoning"); expect(safe.keys).not.toContain("arguments");
      await info.attach("p3-safe-observability", { body: JSON.stringify({ ...safe, deliberateFaults: diagnostics.receipts, unexpected: diagnostics.unexpected }), contentType: "application/json" });
    } finally {
      await Promise.all(contexts.map(context => context.close()));
      configure({ TASKS_MODEL_PROVIDER: "disabled", TASKS_AGENT_MODE: "disabled" });
    }
  },
});
