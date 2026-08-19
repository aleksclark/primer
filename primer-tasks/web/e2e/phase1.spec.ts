import { test, expect, type BrowserContext, type Page } from "@playwright/test";

async function signIn(page: Page, parent: "A" | "B") {
  await page.goto("/parent/students");
  const signIn = page.getByRole("button", { name: /continue with parent sign-in/i });
  if (await signIn.isVisible().catch(() => false)) {
    await signIn.click();
    const parentChoice = page.getByRole("link", { name: new RegExp(`Parent ${parent}`) });
    if (await parentChoice.isVisible().catch(() => false)) await parentChoice.click();
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

async function openStudentAndIssueQr(page: Page, name: string) {
  await page.getByRole("button", { name }).click();
  await expect(page.getByRole("heading", { name }).first()).toBeVisible();
  await page.getByRole("button", { name: /issue pairing qr/i }).click();
  await expect(page.getByRole("img", { name: /one-use student pairing qr code/i })).toBeVisible();
  const code = await page.locator(".code").textContent();
  if (!code) throw new Error("The visible parent QR code was empty");
  return { code, studentURL: page.url() };
}

async function pairStudentBrowser(context: BrowserContext, code: string, name: string) {
  const page = await context.newPage();
  await page.goto("/student/pair");
  await page.getByLabel("Pairing code").fill(code);
  await page.getByRole("button", { name: /pair this browser/i }).click();
  await expect(page.getByText("Nothing assigned yet")).toBeVisible();
  await expect(page.getByText(new RegExp(`Today with ${name}`))).toBeVisible();
  return page;
}

test("parent and student pairing lifecycle stays real, tenant-scoped, and revocable", async ({ browser, baseURL }) => {
  if (!baseURL) throw new Error("baseURL is required");
  const suffix = Date.now().toString(36);
  const parentA = await browser.newContext();
  const parentB = await browser.newContext();
  const studentBrowser = await browser.newContext();
  const replayBrowser = await browser.newContext();
  try {
    const parentAPage = await parentA.newPage();
    await signIn(parentAPage, "A");
    const aCookies = await parentA.cookies(baseURL);
    const parentCookie = aCookies.find((cookie) => cookie.name === "tasks_parent");
    expect(parentCookie?.httpOnly).toBe(true);
    expect(parentCookie?.sameSite).toBe("Lax");

    const studentA = `Playwright Phase 1 A ${suffix}`;
    await createStudent(parentAPage, studentA);
    const { code, studentURL } = await openStudentAndIssueQr(parentAPage, studentA);

    const parentBPage = await parentB.newPage();
    await signIn(parentBPage, "B");
    const studentB = `Playwright Phase 1 B ${suffix}`;
    await createStudent(parentBPage, studentB);
    await parentBPage.goto(studentURL);
    await expect(parentBPage.getByText("Unable to load", { exact: true })).toBeVisible();

    const paired = await pairStudentBrowser(studentBrowser, code, studentA);
    await paired.reload();
    await expect(paired.getByText("Nothing assigned yet")).toBeVisible();

    const replayPage = await replayBrowser.newPage();
    await replayPage.goto("/student/pair");
    await replayPage.getByLabel("Pairing code").fill(code);
    await replayPage.getByRole("button", { name: /pair this browser/i }).click();
    await expect(replayPage.getByRole("alert")).toContainText("Pairing expired");

    parentAPage.once("dialog", (dialog) => dialog.accept());
    await parentAPage.getByRole("button", { name: /archive student/i }).click();
    await expect(parentAPage).toHaveURL(/\/parent\/students$/);
    await paired.reload();
    await expect(paired.getByRole("alert")).toContainText("Pairing revoked");
  } finally {
    await Promise.all([parentA.close(), parentB.close(), studentBrowser.close(), replayBrowser.close()]);
  }
});
