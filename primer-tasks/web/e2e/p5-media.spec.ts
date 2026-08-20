import { test, expect, type BrowserContext, type Page } from "@playwright/test";
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
  await expect(page.getByRole("heading", { name: "Students", exact: true })).toBeVisible();
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

async function pair(parent: Page, context: BrowserContext, student: string) {
  await parent.goto("/parent/students");
  await parent.getByLabel("Search students").fill(student);
  await parent.getByText(student, { exact: true }).click();
  await parent.getByRole("button", { name: /issue pairing qr/i }).click();
  const code = await parent.locator(".code").textContent();
  if (!code?.trim()) throw new Error("pairing code absent from the public parent UI");
  const learner = await context.newPage();
  await learner.goto("/student/pair");
  await learner.getByLabel("Pairing code").fill(code.trim());
  await learner.getByRole("button", { name: /pair this browser/i }).click();
  await expect(learner.getByRole("heading", { name: /today with/i })).toBeVisible();
  return learner;
}

async function start(page: Page, title: string) {
  await page.getByRole("link", { name: new RegExp(title) }).click();
  await page.getByRole("button", { name: /^start task$/i }).click();
}

test("parent-authored image rubric bounds browser upload, persists scripted completion, and rejects malformed retry safely", async ({ browser }, testInfo) => {
  const suffix = Date.now().toString(36), student = `P5 image ${suffix}`, title = `P5 image rubric ${suffix}`;
  const parent = await browser.newContext(), learnerContext = await browser.newContext();
  try {
    const p = await parent.newPage();
    await signIn(p); await createStudent(p, student); await createArtifactTask(p, title, "image"); await schedule(p, title, student);
    const s = await pair(p, learnerContext, student); await start(s, title);
    const oversize = path.join(testInfo.outputDir, "oversize.png");
    await fs.mkdir(testInfo.outputDir, { recursive: true });
    await fs.writeFile(oversize, Buffer.alloc(25 * 1024 * 1024 + 1));
    await s.setInputFiles('input[type="file"]', oversize);
    await expect(s.getByRole("alert")).toContainText(/larger than the 25 MB task limit/i);
    await s.setInputFiles('input[type="file"]', fixture("malformed.png"));
    await s.getByRole("button", { name: /finalize upload/i }).click();
    await expect(s.getByRole("alert")).toContainText(/invalid artifact/i);
    await s.getByRole("button", { name: /submit a new file/i }).click();
    const bounded = s.waitForResponse((r) => r.request().method() === "PUT" && /\/api\/student\/artifacts\/[^/]+\/upload$/.test(new URL(r.url()).pathname));
    await s.setInputFiles('input[type="file"]', fixture("poem-fixture.png"));
    await s.getByRole("button", { name: /finalize upload/i }).click();
    expect(new URL((await bounded).url()).origin).toBe(new URL(s.url()).origin);
    await expect(s.getByText(/^(Queued|Evaluating|Complete)$/, { exact: true }).first()).toBeVisible();
    await expect.poll(async () => { await s.reload(); return s.locator("body").innerText(); }, { timeout: 30_000 }).toMatch(/Complete/);
    await expect(s.getByText(/every required criterion was accepted/i)).toBeVisible();
    await expect(s.locator('input[type="file"]')).toBeDisabled();
    await expect(s.locator("body")).not.toContainText(/x-amz|presign|object key|chain of thought|internal reasoning/i);
  } finally { await Promise.all([parent.close(), learnerContext.close()]); }
});

test("audio and video route to durable parent review with local preview", async ({ browser }) => {
  const suffix = Date.now().toString(36), student = `P5 media ${suffix}`;
  const parent = await browser.newContext(), learnerContext = await browser.newContext();
  try {
    const p = await parent.newPage();
    await signIn(p); await createStudent(p, student);
    for (const [media, file] of [["audio", "evidence.mp3"], ["video", "evidence.mp4"]] as const) {
      const title = `P5 ${media} ${suffix}`;
      await createArtifactTask(p, title, media); await schedule(p, title, student);
      const s = await pair(p, learnerContext, student); await start(s, title);
      await s.setInputFiles('input[type="file"]', fixture(file));
      await expect(s.getByText(media === "audio" ? "audio/mpeg" : "video/mp4", { exact: true })).toBeVisible();
      await expect(s.getByLabel(new RegExp(`selected ${media} preview`, "i"))).toBeVisible();
      await s.getByRole("button", { name: /finalize upload/i }).click();
      await expect(s.getByText(/^(Queued|Evaluating|Parent review)$/, { exact: true }).first()).toBeVisible();
      await expect.poll(async () => { await s.reload(); return s.locator("body").innerText(); }, { timeout: 30_000 }).toMatch(/Parent review/);
      await s.close();
    }
  } finally { await Promise.all([parent.close(), learnerContext.close()]); }
});
