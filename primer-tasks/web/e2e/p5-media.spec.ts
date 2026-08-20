import { test, expect, type Browser, type BrowserContext, type Page } from "@playwright/test";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

test.setTimeout(180_000);

const fixture = (name: string) => path.resolve(path.dirname(fileURLToPath(import.meta.url)), "fixtures", name);

async function signIn(page: Page) {
  await page.goto("/parent/students");
  const button = page.getByRole("button", { name: /continue with parent sign-in/i });
  if (await button.isVisible().catch(() => false)) {
    await button.click();
    await page.getByRole("link", { name: "Parent A" }).click();
  }
  await expect(page.getByRole("heading", { name: "Students", exact: true })).toBeVisible();
}

async function createStudent(page: Page, name: string) {
  await page.getByRole("button", { name: /add( first)? student/i }).first().click();
  await page.getByLabel("Display name").fill(name);
  const created = page.waitForResponse((response) => response.request().method() === "POST" && new URL(response.url()).pathname === "/api/students");
  await page.getByRole("button", { name: /^save student$/i }).click();
  await created;
  await page.reload();
  await page.getByLabel("Search students").fill(name);
  await expect(page.getByText(name, { exact: true })).toBeVisible();
}

async function createArtifactTask(page: Page, title: string, media: "image" | "audio" | "video") {
  await page.goto("/parent/tasks");
  const form = page.getByRole("region", { name: /artifact rubric verification/i });
  await form.getByLabel("Task title").fill(title);
  await form.getByLabel("Student instructions").fill(`Submit one ${media} evidence item.`);
  for (const kind of ["image", "audio", "video"] as const) {
    const control = form.getByRole("checkbox", { name: kind });
    if ((await control.isChecked()) !== (kind === media)) await control.click();
  }
  if (media !== "image") await form.getByLabel("Maximum seconds").fill("60");
  const created = page.waitForResponse((response) => response.request().method() === "POST" && new URL(response.url()).pathname === "/api/tasks");
  await form.getByRole("button", { name: /create artifact draft/i }).click();
  await created;
  await page.getByLabel("Search tasks").fill(title);
  const row = page.getByRole("row").filter({ hasText: title });
  await expect(row).toContainText(/draft/i);
  await row.getByRole("button", { name: /^publish$/i }).click();
  await expect(row).toContainText(/published/i);
}

async function schedule(page: Page, title: string, student: string) {
  const today = new Intl.DateTimeFormat("en-CA", { timeZone: "America/Chicago" }).format(new Date());
  await page.getByLabel("Task to schedule").selectOption({ label: `${title} · v1` });
  await page.getByLabel("Student to schedule").selectOption({ label: student });
  await page.getByLabel("Schedule start").fill(`${today}T14:00`);
  await page.getByLabel("IANA timezone").fill("America/Chicago");
  const dialog = page.waitForEvent("dialog");
  await page.getByRole("button", { name: /^schedule task$/i }).click();
  await (await dialog).accept();
}

async function pair(parent: Page, browser: Browser, student: string): Promise<{ context: BrowserContext; page: Page }> {
  await parent.goto("/parent/students");
  await parent.getByLabel("Search students").fill(student);
  await parent.getByText(student, { exact: true }).click();
  await parent.getByRole("button", { name: /issue pairing qr/i }).click();
  const code = await parent.locator(".code").textContent();
  if (!code?.trim()) throw new Error("pairing code absent from the public parent UI");
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto("/student/pair");
  await page.getByLabel("Pairing code").fill(code.trim());
  await page.getByRole("button", { name: /pair this browser/i }).click();
  await expect(page.getByRole("heading", { name: /today with/i })).toBeVisible();
  return { context, page };
}

async function start(page: Page, title: string) {
  await page.getByRole("link", { name: new RegExp(title) }).click();
  await page.getByRole("button", { name: /^start task$/i }).click();
}

async function assertNoPreviewLeak(page: Page, requestURLs: string[]) {
  await expect.poll(() => requestURLs.filter((url) => url.startsWith("blob:"))).toEqual([]);
  await expect.poll(() => page.evaluate(() => ({
    blobAttribute: Array.from(document.querySelectorAll("[src],[href]")).some((element) => (element.getAttribute("src") || element.getAttribute("href") || "").startsWith("blob:")),
    sensitiveText: /x-amz|presign|object key|signature|chain of thought|internal reasoning/i.test(document.body.innerText),
  }))).toEqual({ blobAttribute: false, sensitiveText: false });
}

test("image completion is live, malformed input retries safely, and CSP keeps previews local", async ({ browser }, testInfo) => {
  const suffix = Date.now().toString(36), student = `P5 image ${suffix}`, title = `P5 image rubric ${suffix}`;
  const parent = await browser.newContext();
  const errors: string[] = [], requestURLs: string[] = [];
  try {
    const p = await parent.newPage();
    const response = await p.goto("/");
    expect(response?.headers()["content-security-policy"] ?? "").toContain("object-src 'none'");
    expect(response?.headers()["content-security-policy"] ?? "").toContain("frame-ancestors 'none'");
    await signIn(p); await createStudent(p, student); await createArtifactTask(p, title, "image"); await schedule(p, title, student);
    const learner = await pair(p, browser, student), s = learner.page;
    s.on("console", (message) => { if (message.type() === "error") errors.push(message.text()); });
    s.on("request", (request) => requestURLs.push(request.url()));
    await start(s, title);
    const oversize = path.join(testInfo.outputDir, "oversize.png");
    await fs.mkdir(testInfo.outputDir, { recursive: true });
    await fs.writeFile(oversize, Buffer.alloc(25 * 1024 * 1024 + 1));
    await s.setInputFiles('input[type="file"]', oversize);
    await expect(s.getByRole("alert")).toContainText(/larger than the 25 MB task limit/i);
    await s.setInputFiles('input[type="file"]', fixture("malformed.png"));
    await s.getByRole("button", { name: /finalize upload/i }).click();
    await expect(s.getByRole("alert")).toContainText(/invalid artifact/i);
    expect(errors).toEqual(["Failed to load resource: the server responded with a status of 400 (Bad Request)"]);
    errors.length = 0;
    await s.getByRole("button", { name: /submit a new file/i }).click();
    const bounded = s.waitForResponse((r) => r.request().method() === "PUT" && /\/api\/student\/artifacts\/[^/]+\/upload$/.test(new URL(r.url()).pathname));
    await s.setInputFiles('input[type="file"]', fixture("poem-fixture.png"));
    await s.getByRole("button", { name: /finalize upload/i }).click();
    expect(new URL((await bounded).url()).origin).toBe(new URL(s.url()).origin);
    await expect(s.getByText(/^Complete$/, { exact: true }).first()).toBeVisible();
    await expect(s.getByText(/every required criterion was accepted/i)).toBeVisible();
    await expect(s.locator('input[type="file"]')).toBeDisabled();
    await assertNoPreviewLeak(s, requestURLs);
    expect(response?.headers()["content-security-policy"] ?? "").toMatch(/media-src[^;]*data:/);
    expect(errors).toEqual([]);
    await learner.context.close();
  } finally { await parent.close(); }
});

test("audio and video use local previews and deliver live parent review", async ({ browser }) => {
  const suffix = Date.now().toString(36), student = `P5 media ${suffix}`;
  const parent = await browser.newContext();
  try {
    const p = await parent.newPage();
    await signIn(p); await createStudent(p, student);
    for (const [media, file] of [["audio", "evidence.mp3"], ["video", "evidence.mp4"]] as const) {
      const title = `P5 ${media} ${suffix}`, errors: string[] = [], requestURLs: string[] = [];
      await createArtifactTask(p, title, media); await schedule(p, title, student);
      const learner = await pair(p, browser, student), s = learner.page;
      s.on("console", (message) => { if (message.type() === "error") errors.push(message.text()); });
      s.on("request", (request) => requestURLs.push(request.url()));
      await start(s, title);
      await s.setInputFiles('input[type="file"]', fixture(file));
      await expect(s.getByText(media === "audio" ? "audio/mpeg" : "video/mp4", { exact: true })).toBeVisible();
      await s.getByRole("button", { name: /finalize upload/i }).click();
      await expect(s.getByLabel(new RegExp(`selected ${media} preview`, "i"))).toBeVisible();
      await expect(s.getByText(/^Parent review$/, { exact: true }).first()).toBeVisible();
      await expect(s.getByText(/automatic review cannot decide this media safely/i)).toBeVisible();
      await expect(s.getByRole("button", { name: /submit a new file/i })).toBeVisible();
      await assertNoPreviewLeak(s, requestURLs);
      expect(errors).toEqual([]);
      await learner.context.close();
    }
  } finally { await parent.close(); }
});
