import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(
  new URL("../../web/package.json", import.meta.url),
);
const { chromium, expect } = require("@playwright/test");
const fixture = process.env.ORKEST_EDITOR_FIXTURE;
const browser = await chromium.launch({
  executablePath: process.env.TASK_EDITOR_BROWSER,
});
try {
  const page = await browser.newPage();
  page.on("pageerror", (error) => process.stderr.write(`${error.message}\n`));
  async function open(mode) {
    const response = await fetch(`${fixture}/open`, { method: "POST" });
    assert.equal(response.status, 200);
    const status = await response.json();
    assert.equal(status.mode, mode);
    await page.goto(status.launchUrl);
    await expect(page.locator(".monaco-workbench")).toBeVisible({
      timeout: 60000,
    });
    for (let i = 0; i < 3; i++) {
      const no = page.getByRole("button", { name: /No, I don.t/ });
      if (await no.isVisible()) await no.click();
      const welcome = page.getByRole("button", { name: "Mark Done" });
      if (await welcome.isVisible()) await welcome.click();
      await page.waitForTimeout(500);
    }
    return status;
  }
  async function command(text) {
    await page.keyboard.press("F1");
    const input = page.locator(".quick-input-widget input");
    await expect(input).toBeVisible();
    await input.fill(`>${text}`);
    await page.keyboard.press("Enter");
  }
  const initial = await open("rw");
  await command("Terminal: Create New Terminal");
  await expect(page.locator(".terminal-wrapper textarea").first()).toBeAttached(
    { timeout: 20000 },
  );
  const terminal = page.locator(".terminal-wrapper textarea").first();
  await terminal.focus();
  await page.keyboard.type(
    "echo browser-saved > /workspace/repositories/github/repo/browser.txt",
  );
  await page.keyboard.press("Enter");
  await expect
    .poll(async () => (await fetch(`${fixture}/saved`)).text())
    .toBe("browser-saved\n");
  const transition = await fetch(`${fixture}/transition`, { method: "POST" });
  assert.equal(transition.status, 200);
  const readonly = await open("ro");
  assert.notEqual(initial.generation, readonly.generation);
  await command("Terminal: Create New Terminal");
  await expect(
    page
      .getByText(/Task editor is read-only: terminal and tasks are disabled/)
      .first(),
  ).toBeVisible({ timeout: 15000 });
  console.log(
    "Real backend/browser: launch exchange, assets, WebSockets, terminal, handoff, readonly replacement passed.",
  );
} finally {
  await browser.close();
}
