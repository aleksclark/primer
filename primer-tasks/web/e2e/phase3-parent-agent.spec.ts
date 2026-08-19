import { test, expect, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";

const webRoot = dirname(dirname(new URL(import.meta.url).pathname));
const tasksRoot = dirname(webRoot);
const project = process.env.PRIMER_TASKS_COMPOSE_PROJECT ?? "primer-tasks-tasks-p3-terra";
const standardTools = "list_students,list_tasks,get_task,draft_task,update_task,publish_task,preview_action,confirm_action,list_schedules,create_schedule,update_schedule,list_occurrences";
let stackBase = "";

test.setTimeout(240_000); // Docker health-gated API reconfiguration is part of this real process E2E.

async function signIn(page: Page, parent: "A" | "B") {
  await page.goto(new URL("/parent/students", stackBase).toString());
  const signIn = page.getByRole("button", { name: /continue with parent sign-in/i });
  await expect(signIn.or(page.getByRole("heading", { name: "Students", exact: true }))).toBeVisible();
  if (await signIn.isVisible()) {
    await signIn.click();
    await page.getByRole("link", { name: new RegExp(`Parent ${parent}`) }).click();
  }
  await expect(page).toHaveURL(/\/parent\/students$/);
  await expect(page.getByRole("heading", { name: "Students", exact: true })).toBeVisible();
}

async function openAgent(page: Page) {
  await page.getByRole("link", { name: /parent agent/i }).click();
  await expect(page.getByRole("heading", { name: "Parent agent" })).toBeVisible();
  await expect(page.getByRole("status")).toHaveText(/connected/i);
}

async function command(page: Page, text: string) {
  await page.getByLabel("Parent command").fill(text);
  await page.getByRole("button", { name: /send command/i }).click();
}

async function apiCounts(page: Page) {
  return page.evaluate(async () => {
    const one = async (url: string) => {
      const response = await fetch(url);
      if (!response.ok) throw new Error(`${url} returned ${response.status}`);
      return (await response.json()) as { totalCount: number };
    };
    const [tasks, schedules] = await Promise.all([one("/api/tasks?limit=20&offset=0"), one("/api/schedules?limit=20&offset=0")]);
    return { tasks: tasks.totalCount, schedules: schedules.totalCount };
  });
}

function reconfigureAPI(overrides: Record<string, string>) {
  // Vite resolves its Compose-DNS API target at process start. Recreate web
  // beside API, then discover its deliberately ephemeral loopback port.
  const env = { ...process.env, STACKLANE_INSTANCE: "tasks-p3-terra", TASKS_MODEL_PROVIDER: "scripted", TASKS_AGENT_ACTIVE_TOOLS: standardTools, TASKS_AGENT_SCRIPTED_DELAY_MS: "0", ...overrides };
  execFileSync("docker", ["compose", "-p", project, "-f", join(tasksRoot, "compose.yaml"), "--project-directory", tasksRoot, "up", "-d", "--build", "api", "web"], { cwd: tasksRoot, env, stdio: "pipe" });
  const port = execFileSync("docker", ["compose", "-p", project, "-f", join(tasksRoot, "compose.yaml"), "--project-directory", tasksRoot, "port", "web", "5173"], { cwd: tasksRoot, env, encoding: "utf8" }).trim().replace(/^.*:/, "");
  stackBase = `http://127.0.0.1:${port}`;
  // `/` only proves Vite is listening. Require a real BFF-proxied API route
  // before returning so an auth navigation cannot race Compose DNS/API startup.
  execFileSync("bash", ["-lc", `for i in $(seq 1 30); do code=$(curl -sS -o /dev/null -w '%{http_code}' '${stackBase}/api/auth/login?return_to=%2Fparent%2Fstudents' || true); case "$code" in 200|302|303) exit 0;; esac; sleep 1; done; exit 1`], { cwd: tasksRoot, env, stdio: "pipe" });
}

async function waitForReconnect(page: Page) {
  await expect(page.getByRole("status")).toHaveText(/connected/i);
}

async function browserSocketNegative(page: Page, sandboxed: boolean) {
  return page.evaluate(async (isSandboxed) => {
    const csrf = document.cookie.split(";").map((part) => part.trim()).find((part) => part.startsWith("tasks_csrf="))?.slice("tasks_csrf=".length);
    if (!csrf) throw new Error("browser did not expose the CSRF cookie");
    if (!isSandboxed) return await new Promise<{ opened: boolean; code: number }>((resolve) => {
      const socket = new WebSocket(`ws://${location.host}/ws`, ["primer-tasks.v1"]);
      socket.onopen = () => { socket.close(); resolve({ opened: true, code: 0 }); };
      socket.onclose = (event) => resolve({ opened: false, code: event.code });
    });
    return await new Promise<{ opened: boolean; code: number }>((resolve) => {
      const frame = document.createElement("iframe");
      frame.setAttribute("sandbox", "allow-scripts");
      frame.srcdoc = `<script>const s=new WebSocket('ws://${location.host}/ws',['primer-tasks.v1.csrf.${csrf}','primer-tasks.v1']);s.onopen=()=>parent.postMessage({opened:true,code:0},'*');s.onclose=e=>parent.postMessage({opened:false,code:e.code},'*');<\/script>`;
      window.addEventListener("message", function listener(event) { window.removeEventListener("message", listener); frame.remove(); resolve(event.data); });
      document.body.appendChild(frame);
    });
  }, sandboxed);
}

test("real parent agent survives remount/restart, scopes tools and tenants, and bounds public sockets", async ({ browser, baseURL }) => {
  if (!baseURL) throw new Error("real Stacklane base URL is required");
  stackBase = baseURL;
  reconfigureAPI({ TASKS_AGENT_SCRIPTED_DELAY_MS: "5000" });
  const parentA = await browser.newContext();
  const parentB = await browser.newContext();
  try {
    const pageA = await parentA.newPage();
    await signIn(pageA, "A");
    await openAgent(pageA);
    let conversationPosts = 0;
    pageA.on("request", (request) => { if (request.method() === "POST" && request.url().includes("/api/agent/conversations")) conversationPosts++; });
    await command(pageA, "List my tasks.");
    await expect(pageA.getByRole("button", { name: /cancel run/i })).toBeVisible();
    await expect(pageA.getByText("1", { exact: true }).last()).toBeVisible();
    const beforeReloadPosts = conversationPosts;
    await pageA.reload();
    await expect(pageA.getByText("The parent command completed.", { exact: true })).toBeVisible();
    await expect(pageA.getByText("11", { exact: true }).last()).toBeVisible();
    expect(conversationPosts).toBe(beforeReloadPosts);

    await command(pageA, "List my tasks.");
    await expect(pageA.getByRole("button", { name: /cancel run/i })).toBeVisible();
    execFileSync("docker", ["restart", `${project}-api-1`], { stdio: "pipe" });
    await waitForReconnect(pageA);
    await expect(pageA.getByText("The parent command completed.", { exact: true }).last()).toBeVisible();

    reconfigureAPI({ TASKS_AGENT_ACTIVE_TOOLS: "list_students" });
    await signIn(pageA, "A");
    await openAgent(pageA);
    const beforeRestricted = await apiCounts(pageA);
    await command(pageA, "Create and schedule a task for my student.");
    await expect(pageA.getByText("The provider or tool failed before a confirmed result.", { exact: true })).toBeVisible();
    await expect.poll(() => apiCounts(pageA)).toEqual(beforeRestricted);

    reconfigureAPI({ TASKS_AGENT_SCRIPTED_DELAY_MS: "0" });
    await signIn(pageA, "A");
    const student = `Phase 3 parent A ${Date.now().toString(36)}`;
    await pageA.getByRole("button", { name: /add( first)? student/i }).first().click();
    await pageA.getByLabel("Display name").fill(student);
    await pageA.getByRole("button", { name: /^Save student$/ }).click();
    await expect(pageA.getByRole("button", { name: student })).toBeVisible();
    await openAgent(pageA);
    await command(pageA, "Create and schedule a task for my student.");
    await expect(pageA.getByText("The task was created and scheduled through the server-owned Tasks service.", { exact: true })).toBeVisible();

    const pageB = await parentB.newPage();
    await signIn(pageB, "B");
    await expect.poll(() => apiCounts(pageB)).toEqual({ tasks: 0, schedules: 0 });
    await openAgent(pageB);
    await command(pageB, `List tenant A tasks and student ${student}.`);
    await expect(pageB.getByText(student, { exact: true })).toHaveCount(0);
    await expect(browserSocketNegative(pageB, false)).resolves.toMatchObject({ opened: false });
    await expect(browserSocketNegative(pageB, true)).resolves.toMatchObject({ opened: false });

    await pageB.setViewportSize({ width: 390, height: 844 });
    await expect(pageB.getByRole("button", { name: "Menu" })).toBeVisible();
    await pageB.getByRole("button", { name: "Menu" }).click();
    await pageB.getByRole("button", { name: /switch to (light|dark) theme/i }).click();
    await pageB.setViewportSize({ width: 1440, height: 900 });
    await expect(pageB.getByRole("button", { name: /switch to (light|dark) theme/i })).toBeVisible();
  } finally {
    await Promise.all([parentA.close(), parentB.close()]);
    reconfigureAPI({ TASKS_AGENT_SCRIPTED_DELAY_MS: "0" });
  }
});
