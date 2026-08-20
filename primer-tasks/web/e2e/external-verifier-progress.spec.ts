import { test, expect, type BrowserContext, type Page } from "@playwright/test";

const fixtureStudent = "Stacklane Student";

test.setTimeout(120_000);

async function signIn(page: Page) {
  await page.goto("/parent/students");
  const signIn = page.getByRole("button", { name: /continue with parent sign-in/i });
  if (await signIn.isVisible().catch(() => false)) {
    await signIn.click();
    await page.getByRole("link", { name: "Parent A" }).click();
  }
  await expect(page).toHaveURL(/\/parent\/students$/);
  await expect(page.getByRole("heading", { name: "Students", exact: true })).toBeVisible();
}

async function createPublishAndScheduleExternalTask(page: Page, title: string) {
  await page.goto("/parent/tasks");
  const externalConfiguration = page.getByRole("region", { name: "External verifier configuration" });
  const externalForm = page.getByRole("form", { name: "External verifier task configuration" });
  await expect(externalForm).toContainText("Stacklane fixture verifier");
  await expect(externalForm).toContainText("response");
  await expect(externalForm).toContainText("external_callback.v1");
  await expect(externalConfiguration).toContainText(/endpoints, credentials, and signatures stay server-side/i);
  await expect(externalForm).toContainText(/no endpoint or secret is exposed here/i);

  await externalForm.getByLabel("Verifier task name").fill(title);
  await externalForm.getByRole("button", { name: "Create external task" }).click();
  await page.getByLabel("Search tasks").fill(title);
  const row = page.getByRole("row").filter({ hasText: title });
  await expect(row).toContainText("draft");
  await row.getByRole("button", { name: "Publish" }).click();
  await expect(row).toContainText("published");

  await page.getByLabel("Task to schedule").selectOption({ label: `${title} · v1` });
  await page.getByLabel("Student to schedule").selectOption({ label: fixtureStudent });
  // The real Stacklane fixture clock is 2026-08-20. A past local slot
  // materializes this one-off occurrence immediately, as established in exploration.
  await page.getByLabel("Schedule start").fill("2026-08-19T14:00");
  await page.getByLabel("IANA timezone").fill("America/Chicago");
  await page.getByLabel("RRULE").fill("FREQ=DAILY;COUNT=1");
  const scheduled = page.waitForEvent("dialog");
  await page.getByRole("button", { name: "Schedule task" }).click();
  await (await scheduled).accept();
}

async function pairFreshStudentBrowser(page: Page, context: BrowserContext) {
  await page.goto("/parent/students");
  await page.getByRole("button", { name: fixtureStudent }).click();
  await page.getByRole("button", { name: /issue pairing qr/i }).click();
  await expect(page.getByRole("img", { name: /one-use student pairing qr code/i })).toBeVisible();
  const code = await page.locator(".code").textContent();
  if (!code?.trim()) throw new Error("The public pairing UI did not render a one-use code");

  const studentPage = await context.newPage();
  await studentPage.goto("/student/pair");
  await studentPage.getByLabel("Pairing code").fill(code.trim());
  await studentPage.getByRole("button", { name: /pair this browser/i }).click();
  await expect(studentPage).toHaveURL(/\/student$/);
  await expect(studentPage.getByRole("heading", { name: /today with/i })).toBeVisible();
  return studentPage;
}

test("real external callback persists safe progress and completion for student and parent", async ({ browser }) => {
  const suffix = Date.now().toString(36);
  const title = `Playwright external verifier ${suffix}`;
  const parent = await browser.newContext();
  const student = await browser.newContext();
  try {
    const parentPage = await parent.newPage();
    await signIn(parentPage);
    await createPublishAndScheduleExternalTask(parentPage, title);

    const studentPage = await pairFreshStudentBrowser(parentPage, student);
    const occurrence = studentPage.getByRole("link", { name: new RegExp(title) }).first();
    await expect(occurrence).toContainText("pending");
    await occurrence.click();
    await expect(studentPage.getByRole("heading", { name: title })).toBeVisible();
    await studentPage.getByRole("button", { name: "Start task" }).click();

    const progress = studentPage.getByRole("region", { name: "External verifier progress" });
    await expect(progress).toContainText("Stacklane fixture verifier");
    await expect(progress.getByRole("heading", { name: "awaiting verification" })).toBeVisible();
    const submitted = studentPage.waitForResponse((response) =>
      response.url().includes("/external/submit") && response.status() === 202,
    );
    await studentPage.getByLabel("Your response").fill("A concise student response for external verification.");
    await studentPage.getByRole("button", { name: "Submit for external verification" }).click();
    await submitted;
    await expect(progress.getByRole("heading", { name: "queued" })).toBeVisible();
    await expect(progress.getByRole("heading", { name: "completed" })).toBeVisible({ timeout: 30_000 });
    await expect(progress.locator("ol > li")).toHaveCount(4);
    await studentPage.reload();
    await expect(studentPage.getByRole("heading", { name: "completed" })).toBeVisible();
    await expect(studentPage.getByRole("region", { name: "External verifier progress" }).locator("ol > li")).toHaveCount(4);

    // Browser-facing status is intentionally a safe projection, not a callback envelope.
    await expect(studentPage.locator("body")).not.toContainText(/callbackId|requestDigest|x-external-signature|chain of thought|internal reasoning/i);

    await parentPage.goto("/parent/occurrences");
    const parentOccurrence = parentPage.getByRole("row").filter({ hasText: title });
    await expect(parentOccurrence).toContainText("completed");
    const inspected = parentPage.waitForResponse((response) =>
      response.url().includes("/external/inspect") && response.status() === 200,
    );
    await parentOccurrence.getByRole("button", { name: "Inspect" }).click();
    await inspected;
    const delivery = parentPage.getByRole("region", { name: "External verifier delivery" });
    await expect(delivery).toContainText("Stacklane fixture verifier");
    await expect(delivery).toContainText("completed");
    const durableUpdates = delivery.locator("ol > li");
    await expect(durableUpdates).toHaveCount(4);
    await expect(durableUpdates.first()).toHaveText("Verification requested.");
    await expect(parentPage.locator("body")).not.toContainText(/callbackId|requestDigest|x-external-signature|chain of thought|internal reasoning/i);
  } finally {
    await Promise.all([parent.close().catch(() => undefined), student.close().catch(() => undefined)]);
  }
});
