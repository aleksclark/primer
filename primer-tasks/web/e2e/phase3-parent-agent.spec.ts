import { test, expect, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";

const tasksRoot = dirname(dirname(dirname(new URL(import.meta.url).pathname)));
const tools = "list_students,list_tasks,get_task,draft_task,update_task,publish_task,preview_action,confirm_action,list_schedules,create_schedule,update_schedule,list_occurrences";
test.setTimeout(240_000);

function configure(overrides: Record<string, string> = {}, action = "restart-api") {
  execFileSync(join(tasksRoot, "scripts/host-stack"), [action], {
    cwd: tasksRoot,
    env: { ...process.env, TASKS_MODEL_PROVIDER: "scripted", TASKS_AGENT_MODE: "scripted", TASKS_AGENT_ACTIVE_TOOLS: tools, TASKS_AGENT_SCRIPTED_DELAY_MS: "0", ...overrides },
    stdio: "pipe",
  });
}
async function signIn(page: Page, parent: "A" | "B") {
  await page.goto("/parent/students");
  const login = page.getByRole("button", { name: /continue with parent sign-in/i });
  const heading = page.getByRole("heading", { name: "Students", exact: true });
  await expect(login.or(heading)).toBeVisible();
  if (await login.isVisible()) { await login.click(); await page.getByRole("link", { name: new RegExp(`Parent ${parent}`) }).click(); }
  await expect(heading).toBeVisible();
}
async function openAgent(page: Page) {
  await page.goto("/parent/agent");
  await expect(page.getByRole("heading", { name: "Parent agent", exact: true })).toBeVisible();
  await expect(page.locator(".page-actions [role=status]")).toHaveText("Connected");
}
async function command(page: Page, text: string) {
  await expect(page.getByLabel("Parent command")).toBeEnabled();
  await page.getByLabel("Parent command").fill(text);
  await page.getByRole("button", { name: "Send command", exact: true }).click();
}
async function counts(page: Page) {
  return page.evaluate(async () => {
    async function count(path: string) { const r = await fetch(path); if (!r.ok) throw new Error(`public API returned ${r.status}`); return (await r.json()).totalCount as number; }
    return { tasks: await count("/api/tasks"), schedules: await count("/api/schedules") };
  });
}
async function socketNegative(page: Page, sandboxed: boolean) {
  return page.evaluate(async (sandboxed) => {
    const csrf = document.cookie.split(";").map(v => v.trim()).find(v => v.startsWith("tasks_csrf="))?.slice(11);
    if (!csrf) throw new Error("missing CSRF cookie");
    const url = `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws`;
    if (!sandboxed) return new Promise<boolean>((resolve) => { const s = new WebSocket(url, ["primer-tasks.v1"]); s.onopen = () => { s.close(); resolve(true); }; s.onclose = () => resolve(false); });
    return new Promise<boolean>((resolve) => {
      const frame = document.createElement("iframe"); frame.setAttribute("sandbox", "allow-scripts");
      frame.srcdoc = `<script>const s=new WebSocket(${JSON.stringify(url)},['primer-tasks.v1.csrf.${csrf}','primer-tasks.v1']);s.onopen=()=>parent.postMessage({opened:true},'*');s.onclose=()=>parent.postMessage({opened:false},'*');</script>`;
      const listener = (e: MessageEvent) => { if (e.source !== frame.contentWindow) return; window.removeEventListener("message", listener); frame.remove(); resolve(e.data.opened); };
      window.addEventListener("message", listener); document.body.appendChild(frame);
    });
  }, sandboxed);
}

test("parent command previews, rejects, confirms, reconnects and survives a killed worker with scoped tools", async ({ browser, baseURL }) => {
  if (!baseURL) throw new Error("real host-stack URL required");
  configure({ TASKS_AGENT_SCRIPTED_DELAY_MS: "2500" });
  const a = await browser.newContext({ baseURL }); const b = await browser.newContext({ baseURL });
  try {
    const page = await a.newPage(); await signIn(page, "A"); await openAgent(page);
    let posts = 0; page.on("request", r => { if (r.method() === "POST" && r.url().endsWith("/agent/conversations")) posts++; });
    await command(page, "List my tasks.");
    await expect(page.getByText("Thinking", { exact: true }).first()).toBeVisible();
    const beforeReload = posts;
    await page.reload();
    await expect(page.getByText("The parent command completed.", { exact: true })).toBeVisible();
    await expect(page.getByText("List my tasks.", { exact: true })).toBeVisible();
    expect(posts).toBe(beforeReload);
    const cursor = Number(await page.locator(".agent-inspector dl div").filter({ hasText: "Cursor" }).locator("dd").textContent());
    expect(cursor).toBeGreaterThan(1);

    const thinkingBeforeRestart = await page.getByText("Thinking", { exact: true }).count();
    await command(page, "List my tasks after restart.");
    await expect(page.getByRole("button", { name: "Cancel run", exact: true })).toBeVisible();
    await expect(page.getByText("Thinking", { exact: true })).toHaveCount(thinkingBeforeRestart + 1);
    configure({ TASKS_AGENT_SCRIPTED_DELAY_MS: "2500" }, "crash-api");
    // A real killed process retains its 30s PostgreSQL lease. This timeout is
    // derived from that lease, not a sleep masking a UI synchronization race.
    await expect(page.getByText("Parent authorization must be renewed. Send a new request.", { exact: true })).toBeVisible({ timeout: 45_000 });
    await expect(page.getByText("The parent command completed.", { exact: true })).toHaveCount(1);
    await expect(page.locator(".page-actions [role=status]")).toHaveText("Connected");
    expect(Number(await page.locator(".agent-inspector dl div").filter({ hasText: "Cursor" }).locator("dd").textContent())).toBeGreaterThan(cursor);

    configure({ TASKS_AGENT_ACTIVE_TOOLS: "list_students" }); await page.reload();
    const restricted = await counts(page);
    await command(page, "Create and schedule a task.");
    await expect(page.getByText("The provider or tool failed before a confirmed result.", { exact: true })).toBeVisible();
    expect(await counts(page)).toEqual(restricted);

    configure(); await page.goto("/parent/students");
    const student = `P3 scoped student ${Date.now().toString(36)}`;
    await page.getByRole("button", { name: /add( first)? student/i }).first().click();
    await page.getByLabel("Display name").fill(student); await page.getByRole("button", { name: "Save student", exact: true }).click();
    await expect(page.getByRole("button", { name: student, exact: true })).toBeVisible();
    await openAgent(page); const before = await counts(page);
    const create = `Create and schedule a task for student "${student}".`;
    await command(page, create);
    await expect(page.getByRole("button", { name: "Confirm preview", exact: true })).toBeVisible();
    await expect(page.getByText(new RegExp(`Create and publish.*${student}`))).toBeVisible();
    expect(await counts(page)).toEqual(before);
    await page.getByRole("button", { name: "Cancel request", exact: true }).click();
    await expect(page.getByRole("button", { name: "Confirm preview", exact: true })).toHaveCount(0);
    expect(await counts(page)).toEqual(before);
    await command(page, create); await expect(page.getByRole("button", { name: "Confirm preview", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Confirm preview", exact: true }).click();
    await expect(page.getByText("The confirmed parent command completed.", { exact: true })).toBeVisible();
    await expect.poll(() => counts(page)).toEqual({ tasks: before.tasks + 1, schedules: before.schedules + 1 });
    await page.reload(); await expect(page.getByText("The confirmed parent command completed.", { exact: true })).toBeVisible();
    const createdTask = await page.evaluate(async () => {
      const response = await fetch("/api/tasks?q=Scripted%20parent%20task");
      if (!response.ok) throw new Error(`tasks read returned ${response.status}`);
      const body = await response.json();
      if (body.items.length !== 1 || body.items[0].status !== "published") throw new Error("confirmed published task missing");
      return body.items[0] as { title: string };
    });
    await page.goto("/parent/schedules");
    // Preserve the released usability surface: titles and student names lead.
    await expect(page.getByRole("row").filter({ hasText: createdTask.title }).filter({ hasText: student })).toBeVisible();

    const other = await b.newPage(); await signIn(other, "B"); const otherCounts = await counts(other); await openAgent(other);
    await command(other, `List tenant A student ${student}.`);
    await expect(other.getByText("The parent command completed.", { exact: true })).toBeVisible();
    await expect(other.getByText(student, { exact: true })).toHaveCount(0); expect(await counts(other)).toEqual(otherCounts);
    expect(await socketNegative(other, false)).toBe(false); expect(await socketNegative(other, true)).toBe(false);
    await other.setViewportSize({ width: 390, height: 844 }); await other.getByRole("button", { name: "Menu", exact: true }).click();
    await other.getByRole("button", { name: /switch to (light|dark) theme/i }).click();
    await other.setViewportSize({ width: 1440, height: 900 }); await expect(other.getByRole("button", { name: /switch to (light|dark) theme/i })).toBeVisible();
    configure({ TASKS_MODEL_PROVIDER: "disabled", TASKS_AGENT_MODE: "disabled" }); await other.reload();
    await command(other, "List with model disabled."); await expect(other.getByText("Agent mode is disabled. Use the ordinary Tasks and Schedules pages.", { exact: true })).toBeVisible();
    expect(await counts(other)).toEqual(otherCounts);
  } finally { await Promise.all([a.close(), b.close()]); configure({ TASKS_MODEL_PROVIDER: "disabled", TASKS_AGENT_MODE: "disabled" }); }
});
