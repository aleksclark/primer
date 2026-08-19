import { test, expect, type Browser, type BrowserContext, type Page } from "@playwright/test";

async function signIn(page: Page, parent: "A" | "B") {
  await page.goto("/parent/students");
  const signIn = page.getByRole("button", { name: /continue with parent sign-in/i });
  if (await signIn.isVisible().catch(() => false)) {
    await signIn.click();
    await page.getByRole("link", { name: new RegExp(`Parent ${parent}`) }).click();
  }
  await expect(page).toHaveURL(/\/parent\/students$/);
  await expect(page.getByRole("heading", { name: "Students" })).toBeVisible();
}

async function createStudent(page: Page, name: string) {
  await page.getByRole("button", { name: /add( first)? student/i }).first().click();
  await page.getByLabel("Display name").fill(name);
  await page.getByRole("button", { name: /^Save student$/ }).click();
  await expect(page.getByRole("button", { name })).toBeVisible();
}

async function createAndPublishTask(page: Page, title: string) {
  await page.goto("/parent/tasks");
  await page.getByLabel("Task title").fill(title);
  await page.getByRole("button", { name: /^Create draft$/ }).click();
  const row = page.getByRole("row").filter({ hasText: title });
  await expect(row).toContainText("draft");
  await row.getByRole("button", { name: /^Publish$/ }).click();
  await expect(row).toContainText("published");
}

async function scheduleToday(page: Page, title: string, student: string, rrule = "") {
  const today = new Intl.DateTimeFormat("en-CA", { timeZone: "America/Chicago" }).format(new Date());
  const taskOption = page.getByLabel("Task to schedule").locator("option").filter({ hasText: title }).first();
  const taskValue = await taskOption.getAttribute("value");
  if (!taskValue) throw new Error("The public task selector did not expose the published task value");
  await page.getByLabel("Task to schedule").selectOption(taskValue);
  await page.getByLabel("Student to schedule").selectOption({ label: student });
  await page.getByLabel("Schedule start").fill(`${today}T14:00`);
  await page.getByLabel("IANA timezone").fill("America/Chicago");
  await page.getByLabel("RRULE").fill(rrule);
  page.once("dialog", (dialog) => dialog.accept());
  await Promise.all([
    page.waitForResponse((response) => response.url().includes("/api/schedules") && response.status() === 201),
    page.getByRole("button", { name: /^Schedule task$/ }).click(),
  ]);
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
  await expect(page.getByRole("heading", { name: "Today" })).toBeVisible();
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
    await expect(parentPage.getByRole("heading", { name: "Occurrences" })).toBeVisible();
    await expect(parentPage.getByText(taskTitle)).toBeVisible();
    await parentPage.getByLabel("Occurrence status filter").selectOption("pending");
    await parentPage.getByLabel("Occurrence sort direction").selectOption("desc");
    await expect(parentPage).toHaveURL(/status=pending.*dir=desc|dir=desc.*status=pending/);

    const pairing = await issuePairingCode(parentPage, studentName);
    const studentPage = await pairStudent(studentContext, pairing.code);
    await expect(studentPage.getByText(taskTitle)).toBeVisible();
    const taskLink = studentPage.getByRole("link", { name: new RegExp(taskTitle) }).first();
    await taskLink.click();
    await expect(studentPage.getByRole("heading", { name: taskTitle })).toBeVisible();
    await studentPage.getByRole("button", { name: /^Start task$/ }).click();
    await expect(studentPage.getByText(/awaiting_verification/i)).toBeVisible();

    await parentPage.goto("/parent/occurrences?status=awaiting_verification");
    let awaiting = parentPage.getByRole("row").filter({ hasText: taskTitle });
    await expect(awaiting).toContainText("awaiting_verification");
    await awaiting.getByRole("button", { name: /^Reject$/ }).click();
    await parentPage.goto("/parent/occurrences?status=pending");
    let pending = parentPage.getByRole("row").filter({ hasText: taskTitle });
    await expect(pending).toContainText("pending");
    await pending.getByRole("button", { name: /^Retry$/ }).click();
    await parentPage.goto("/parent/occurrences?status=awaiting_verification");
    awaiting = parentPage.getByRole("row").filter({ hasText: taskTitle });
    await expect(awaiting).toContainText("awaiting_verification");
    await awaiting.getByRole("button", { name: /^Approve$/ }).click();
    await parentPage.goto("/parent/occurrences");
    awaiting = parentPage.getByRole("row").filter({ hasText: taskTitle });
    await expect(awaiting).toContainText("completed");

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
