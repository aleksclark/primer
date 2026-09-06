import { test, expect, type BrowserContext, type Page } from "@playwright/test";

async function signIn(page: Page, parent: "A" | "B") {
  await page.goto("/parent/students");
  const students = page.getByRole("heading", { name: "Students", exact: true });
  const signIn = page.getByRole("button", { name: /continue with parent sign-in/i });
  await expect(students.or(signIn)).toBeVisible();
  if (await signIn.isVisible()) {
    await signIn.click();
    await page.getByRole("link", { name: new RegExp(`Parent ${parent}`) }).click();
  }
  await expect(page).toHaveURL(/\/parent\/students$/);
  await expect(students).toBeVisible();
}

async function createStudent(page: Page, name: string) {
  await page.getByRole("button", { name: /add( first)? student/i }).first().click();
  await page.getByLabel("Display name").fill(name);
  await page.getByRole("button", { name: /^Save student$/ }).click();
  await expect(page.getByRole("button", { name })).toBeVisible();
}

async function createAndPublishTask(page: Page, title: string) {
  await page.goto("/parent/tasks");
  await page.getByRole("button", { name: /^Create task$/ }).click();
  await page.getByLabel("Task title").fill(title);
  await page.getByRole("button", { name: /^Create draft$/ }).click();
  const row = page.getByRole("row").filter({ hasText: title });
  await expect(row).toContainText("draft");
  await row.getByRole("button", { name: /^Publish$/ }).click();
  await expect(row).toContainText("published");
}

async function scheduleToday(page: Page, title: string, student: string, preset: "once" | "daily" = "once") {
  const today = new Intl.DateTimeFormat("en-CA", { timeZone: "America/Chicago" }).format(new Date());
  await page.getByRole("button", { name: /^Schedule a task$/ }).click();
  await page.getByLabel("Find a published task").fill(title);
  const taskOption = page.getByLabel("Task to schedule").locator("option").filter({ hasText: title }).first();
  const taskValue = await taskOption.getAttribute("value");
  if (!taskValue) throw new Error("The public task selector did not expose the published task value");
  await page.getByLabel("Task to schedule").selectOption(taskValue);
  await page.getByLabel("Find a student").fill(student);
  await page.getByLabel("Student to schedule").selectOption({ label: student });
  await page.getByLabel("Repeat", { exact: true }).selectOption(preset);
  if (preset === "daily") await page.getByLabel("Repeat limit (optional)", { exact: true }).fill("2");
  await page.getByLabel("Start date", { exact: true }).fill(today);
  await page.getByLabel("Time", { exact: true }).fill("14:00");
  await page.getByText("Advanced", { exact: true }).click();
  await page.getByLabel("Time zone", { exact: true }).fill("America/Chicago");
  await Promise.all([
    page.waitForResponse((response) => response.url().includes("/api/schedules") && response.status() === 201),
    page.getByRole("button", { name: /^Schedule task$/ }).click(),
  ]);
  await expect(page.getByText("Schedule results", { exact: true })).toBeVisible();
  await expect(page.getByRole("status")).toContainText("Saved");
}

async function issuePairingCode(page: Page, student: string) {
  await page.goto("/parent/students");
  await page.getByRole("button", { name: student }).click();
  await page.getByRole("button", { name: /issue pairing qr/i }).click();
  await expect(page.getByRole("img", { name: /one-use student pairing qr code/i })).toBeVisible();
  const code = await page.locator(".code").textContent();
  if (!code?.trim()) throw new Error("The public pairing UI did not render a code");
  return { code: code.trim(), studentURL: page.url() };
}

async function pairStudent(context: BrowserContext, code: string) {
  const page = await context.newPage();
  await page.goto("/student/pair");
  await page.getByLabel("Pairing code").fill(code);
  await page.getByRole("button", { name: /pair this browser/i }).click();
  await expect(page).toHaveURL(/\/student$/);
  await expect(page.getByRole("heading", { name: /^Today with / })).toBeVisible();
  return page;
}

test("public task schedule, student verification, collections, and tenant boundary", async ({ browser }) => {
  const suffix = Date.now().toString(36);
  const parentA = await browser.newContext();
  const parentB = await browser.newContext();
  const studentContext = await browser.newContext();
  try {
    const parentPage = await parentA.newPage();
    await signIn(parentPage, "A");
    const studentName = `Playwright phase 2 student ${suffix}`;
    const taskTitle = `Playwright phase 2 task ${suffix}`;
    await createStudent(parentPage, studentName);
    await createAndPublishTask(parentPage, taskTitle);

    await parentPage.goto("/parent/tasks");
    await scheduleToday(parentPage, taskTitle, studentName);

    await parentPage.goto("/parent/schedules?status=all");
    await expect(parentPage.getByRole("heading", { name: "Schedules", exact: true })).toBeVisible();
    await expect(parentPage).toHaveURL(/\/parent\/schedules\?status=all$/);
    await expect(parentPage.getByText("America/Chicago", { exact: true }).first()).toBeVisible();
    await parentPage.getByLabel("Schedule status filter").selectOption("active");
    await parentPage.getByLabel("Schedule status filter").selectOption("all");
    await expect(parentPage).toHaveURL(/\/parent\/schedules\?status=all$/);

    await parentPage.goto("/parent/occurrences");
    await expect(parentPage.getByRole("heading", { name: "Assigned work" })).toBeVisible();
    await expect(parentPage.getByText(taskTitle)).toBeVisible();
    await parentPage.getByLabel("Assigned work status filter").selectOption("pending");
    await parentPage.getByLabel("Assigned work sort direction").selectOption("desc");
    await expect(parentPage).toHaveURL(/status=pending.*dir=desc|dir=desc.*status=pending/);

    const pairing = await issuePairingCode(parentPage, studentName);
    const studentPage = await pairStudent(studentContext, pairing.code);
    await expect(studentPage.getByText(taskTitle)).toBeVisible();
    const taskLink = studentPage.getByRole("link", { name: new RegExp(taskTitle) }).first();
    await taskLink.click();
    await expect(studentPage.getByRole("heading", { name: taskTitle })).toBeVisible();
    await studentPage.getByRole("button", { name: /^Start task$/ }).click();
    await expect(studentPage.getByText("In progress", { exact: true })).toBeVisible();
    await studentPage.getByRole("button", { name: /^Submit for parent approval$/ }).click();
    await expect(studentPage.getByText("Waiting for parent", { exact: true })).toBeVisible();

    await parentPage.goto("/parent/occurrences?status=awaiting_verification");
    let awaiting = parentPage.getByRole("row").filter({ hasText: taskTitle });
    await expect(awaiting).toContainText("Waiting for parent");
    await Promise.all([
      parentPage.waitForResponse((response) => response.url().includes("/decision") && response.status() === 200),
      awaiting.getByRole("button", { name: /^Reject$/ }).click(),
    ]);
    await parentPage.goto("/parent/occurrences?status=pending");
    const pending = parentPage.getByRole("row").filter({ hasText: taskTitle });
    await expect(pending).toContainText("Not started");
    await Promise.all([
      parentPage.waitForResponse((response) => response.url().includes("/retry") && response.status() === 200),
      pending.getByRole("button", { name: /^Retry$/ }).click(),
    ]);
    await parentPage.goto("/parent/occurrences?status=awaiting_verification");
    awaiting = parentPage.getByRole("row").filter({ hasText: taskTitle });
    await expect(awaiting).toContainText("Waiting for parent");
    await Promise.all([
      parentPage.waitForResponse((response) => response.url().includes("/decision") && response.status() === 200),
      awaiting.getByRole("button", { name: /^Approve$/ }).click(),
    ]);
    await parentPage.goto("/parent/occurrences");
    awaiting = parentPage.getByRole("row").filter({ hasText: taskTitle });
    await expect(awaiting).toContainText("Completed");

    await studentPage.goto("/student");
    await expect(studentPage.getByText(taskTitle)).toBeVisible();
    await expect(studentPage.getByText(/completed/i)).toBeVisible();

    const parentBPage = await parentB.newPage();
    await signIn(parentBPage, "B");
    await parentBPage.goto("/parent/tasks");
    await expect(parentBPage.getByText(taskTitle)).toHaveCount(0);
    await parentBPage.goto(pairing.studentURL);
    await expect(parentBPage.getByText("Unable to load", { exact: true })).toBeVisible();
  } finally {
    await Promise.all([parentA.close(), parentB.close(), studentContext.close()]);
  }
});

test("parents edit drafts and published tasks without rewriting assigned work, then edit and cancel a cadence", async ({ browser }) => {
  const context = await browser.newContext();
  try {
    const page = await context.newPage();
    const suffix = Date.now().toString(36);
    const title = `Careful work ${suffix}`;
    const revised = `Careful work updated ${suffix}`;
    const student = `Schedule learner ${suffix}`;
    await signIn(page, "A");
    await createStudent(page, student);
    await createAndPublishTask(page, title);
    await scheduleToday(page, title, student);
    await page.goto("/parent/tasks");
    await page.getByLabel("Search tasks").fill(suffix);
    await page.getByRole("row").filter({ hasText: title }).getByRole("button", { name: "Edit", exact: true }).click();
    const editor = page.getByRole("dialog", { name: "Edit task", exact: true });
    await expect(editor).toContainText("Work already assigned keeps its original instructions");
    await editor.getByLabel("Task title").fill(revised);
    await editor.getByLabel("Instructions").fill("Read the new instructions carefully.");
    await editor.getByRole("button", { name: "Save new draft" }).click();
    let row = page.getByRole("row").filter({ hasText: revised });
    await expect(row).toHaveCount(1);
    await expect(row).toContainText("draft");
    // Editing a draft also appends a version; it still occupies one task row.
    await row.getByRole("button", { name: "Edit", exact: true }).click();
    await editor.getByLabel("Instructions").fill("Read these final instructions carefully.");
    await editor.getByRole("button", { name: "Save new draft" }).click();
    row = page.getByRole("row").filter({ hasText: revised });
    await expect(row).toHaveCount(1);
    await row.getByRole("button", { name: "Publish", exact: true }).click();
    await expect(row).toContainText("published");

    await page.goto("/parent/occurrences");
    await expect(page.getByRole("row").filter({ hasText: title })).toContainText(student);
    await expect(page.getByText(revised, { exact: true })).toHaveCount(0);
    await page.goto("/parent/schedules");
    const originalSchedule = page.getByRole("row").filter({ hasText: title });
    await expect(originalSchedule).toContainText(student);
    await originalSchedule.getByRole("button", { name: "Edit schedule", exact: true }).click();
    const scheduleEditor = page.getByRole("form", { name: "Edit schedule", exact: true });
    await expect(scheduleEditor).toContainText("Existing assigned times and instructions stay unchanged");
    await scheduleEditor.getByLabel("Repeat", { exact: true }).selectOption("daily");
    await scheduleEditor.getByLabel("Repeat limit (optional)", { exact: true }).fill("2");
    await scheduleEditor.getByRole("button", { name: "Save schedule", exact: true }).click();
    await expect(scheduleEditor.getByRole("status")).toContainText("Already assigned work has not changed");
    await scheduleEditor.getByRole("button", { name: "Done", exact: true }).click();
    await expect(originalSchedule).toContainText("Daily");
    page.once("dialog", (dialog) => dialog.accept());
    await originalSchedule.getByRole("button", { name: "Cancel schedule", exact: true }).click();
    await expect(originalSchedule).toHaveCount(0);

    await page.goto("/parent/tasks");
    await scheduleToday(page, revised, student, "daily");
    await page.goto("/parent/tasks");
    row = page.getByRole("row").filter({ hasText: revised });
    page.once("dialog", (dialog) => dialog.accept());
    await row.getByRole("button", { name: "Archive", exact: true }).click();
    await expect(row).toHaveCount(0);
    await page.getByLabel("Task status filter").selectOption("all");
    await expect(row).toContainText("Archived");
    await expect(row.getByRole("button", { name: "Publish", exact: true })).toHaveCount(0);
  } finally { await context.close(); }
});

test("several daily times are separate, named schedules with honest save results", async ({ browser }, testInfo) => {
  const context = await browser.newContext();
  try {
    const page = await context.newPage();
    const suffix = Date.now().toString(36);
    const title = `Daily care ${suffix}`;
    const student = `Daily learner ${suffix}`;
    await signIn(page, "A");
    await createStudent(page, student);
    await createAndPublishTask(page, title);
    await page.getByRole("button", { name: "Schedule a task", exact: true }).click();
    await page.getByLabel("Find a published task").fill(title);
    await page.getByLabel("Task to schedule", { exact: true }).selectOption({ label: title });
    await page.getByLabel("Find a student").fill(student);
    await page.getByLabel("Student to schedule", { exact: true }).selectOption({ label: student });
    await page.getByLabel("Repeat", { exact: true }).selectOption("multiple");
    await page.getByLabel("Times per day", { exact: true }).fill("2");
    await page.getByLabel("Time 1", { exact: true }).fill("09:00");
    await page.getByLabel("Time 2", { exact: true }).fill("18:00");
    await page.getByLabel("Repeat limit for each time (optional)", { exact: true }).fill("2");
    const form = page.getByRole("form", { name: "Schedule a task", exact: true });
    await expect(form).toContainText("Each time becomes a separate daily schedule");
    await page.screenshot({ path: testInfo.outputPath("presets-dark-desktop.png"), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(page.getByLabel("Time 2", { exact: true })).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("presets-dark-mobile.png"), fullPage: true });
    await page.getByRole("button", { name: "Schedule task", exact: true }).click();
    await expect(form.getByRole("status")).toContainText("09:00 — Saved");
    await expect(form.getByRole("status")).toContainText("18:00 — Saved");
    await expect(form.getByRole("button", { name: "Schedule task", exact: true })).toHaveCount(0);
    await page.goto("/parent/schedules");
    const rows = page.getByRole("row").filter({ hasText: title });
    await expect(rows).toHaveCount(2);
    await expect(rows.first()).toContainText(student);
    await expect(rows.first()).toContainText("09:00");
    await expect(rows.last()).toContainText("18:00");
    await page.screenshot({ path: testInfo.outputPath("schedules-dark-mobile.png"), fullPage: true });
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.getByRole("button", { name: "Switch to light theme", exact: true }).click();
    await page.screenshot({ path: testInfo.outputPath("schedules-light-desktop.png"), fullPage: true });
  } finally { await context.close(); }
});
