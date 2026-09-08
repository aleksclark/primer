import { expect, test } from "@playwright/test";

const cases = [
  ["tasks-pilot-workflow--pairing", "Connect this browser"],
  ["tasks-pilot-workflow--mixed-checklist", "Today with Ada"],
  ["tasks-pilot-workflow--not-started", "Practice fractions"],
  ["tasks-pilot-workflow--in-progress", "Practice fractions"],
  ["tasks-pilot-workflow--waiting-for-parent", "Practice fractions"],
  ["tasks-pilot-workflow--rejected-retry", "Practice fractions"],
  ["tasks-pilot-workflow--completed", "Practice fractions"],
] as const;

for (const theme of ["dark", "light"] as const) {
  for (const [id, heading] of cases) {
    test(`${id} ${theme}`, async ({ page }) => {
      await page.setViewportSize({ width: 1280, height: 900 });
      await page.goto(`/iframe.html?id=${id}&viewMode=story&globals=theme:${theme}`);
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
      await expect(page).toHaveScreenshot(`${id}-${theme}.png`, { animations: "disabled", caret: "hide", fullPage: true });
    });
  }
}
