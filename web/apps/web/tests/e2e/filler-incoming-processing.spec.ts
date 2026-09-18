import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const at = "2026-09-17T03:20:00Z";

const measured = {
  clipHash: "measured",
  name: "Measured video check",
  from: "Preview collection",
  durationMs: 12_000,
  statusLabel: "Checking video",
  updatedAt: at,
  preparation: { state: "estimated", percent: 47, readyIn: { lowerSeconds: 120, upperSeconds: 240 } },
  processing: {
    attempts: 1,
    stages: [
      {
        label: "Inspecting the file",
        outcome: "finished",
        outcomeLabel: "Finished",
        note: "This step finished successfully.",
        at,
      },
      {
        label: "Checking for separate clips",
        outcome: "not_needed",
        outcomeLabel: "Not needed",
        note: "This step was not needed for this clip.",
        at,
      },
      {
        label: "Listening for speech",
        outcome: "not_needed",
        outcomeLabel: "Not needed",
        note: "This step was not needed for this clip.",
        at,
      },
      {
        label: "Preparing playback",
        outcome: "in_progress",
        outcomeLabel: "In progress",
        note: "Loomarr is working on this step now.",
        at,
        progress: 47,
      },
    ],
  },
};

const unmeasured = {
  ...measured,
  clipHash: "unmeasured",
  name: "Unmeasured details check",
  statusLabel: "Adding details",
  preparation: { state: "estimating", percent: 33 },
  processing: {
    stages: [
      {
        label: "Adding clip details",
        outcome: "in_progress",
        outcomeLabel: "In progress",
        note: "Loomarr is working on this step now.",
        at,
      },
    ],
  },
};

const ready = {
  ...measured,
  clipHash: "ready",
  name: "Recently ready example",
  statusLabel: "Ready",
  preparation: { state: "ready", percent: 100 },
  processing: {
    stages: [
      {
        label: "Finishing",
        outcome: "finished",
        outcomeLabel: "Finished",
        note: "This step finished successfully.",
        at,
      },
    ],
  },
};

const installIncoming = async (page: Parameters<typeof installMockBackend>[0]) => {
  await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  // Register after the broad mock route so the SSE connection receives its real media type
  // instead of that route's JSON fallback and reporting a browser MIME error.
  await page.route("**/v1/events**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      headers: { "cache-control": "no-cache" },
      body: ": connected\n\n",
    }),
  );
  await page.route("**/v1/filler/incoming*", (route) =>
    route.fulfill({
      json: {
        preparing: { rows: [measured, unmeasured], total: 2 },
        needsHelp: { rows: [], total: 0 },
        recentlyReady: { rows: [ready], total: 1 },
        readyWindowSeconds: 86_400,
      },
    }),
  );
  await page.route("**/v1/filler?*", (route) => {
    const hash = new URL(route.request().url()).searchParams.get("hashes") ?? "";
    const match = [measured, unmeasured, ready].find((row) => row.clipHash === hash) ?? measured;
    return route.fulfill({
      json: {
        clips: [
          {
            hash: match.clipHash,
            name: match.name,
            kind: "commercial",
            durationMs: match.durationMs,
            source: match.from,
            held: match.statusLabel !== "Ready",
            playCount: 0,
            playsCounted: true,
          },
        ],
        total: 1,
      },
    });
  });
  await page.route("**/v1/filler/media/**", (route) =>
    route.fulfill({ status: 200, contentType: "video/mp4", body: Buffer.alloc(0) }),
  );
};

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`Incoming explains processing without losing context on ${viewport.name}`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.emulateMedia({ reducedMotion: "reduce" });
    await installIncoming(page);
    const errors: string[] = [];
    page.on("console", (message) => message.type() === "error" && errors.push(message.text()));
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto("/filler/incoming");

    const trigger = page.getByRole("button", { name: "View details for Measured video check" });
    await trigger.focus();
    await trigger.click();
    const panel = page.getByRole("dialog", { name: "Measured video check" });
    await expect(panel.getByText("About 2–4 minutes")).toBeVisible();
    await expect(panel.getByRole("progressbar", { name: "Clip preparation" })).toHaveAttribute(
      "aria-valuenow",
      "47",
    );
    const details = panel.locator("details");
    await expect(details).not.toHaveAttribute("open", "");
    await panel.getByText("Processing details", { exact: true }).click();
    await expect(panel.getByRole("listitem")).toHaveCount(2);
    await expect(panel.getByRole("progressbar", { name: "Current step: 47%" })).toHaveAttribute(
      "aria-valuenow",
      "47",
    );
    await panel.getByRole("button", { name: "Show 2 skipped steps" }).click();
    await expect(panel.getByRole("listitem")).toHaveCount(4);
    await expect(panel.getByText("Not needed", { exact: true })).toHaveCount(2);
    await panel.getByRole("button", { name: "Preview clip" }).click();
    await expect(panel.locator("video")).toHaveCount(1);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    const accessibility = await new AxeBuilder({ page }).analyze();
    expect(
      accessibility.violations.filter((violation) =>
        ["serious", "critical"].includes(violation.impact ?? ""),
      ),
    ).toEqual([]);
    await page.keyboard.press("Escape");
    await expect(panel).toHaveCount(0);
    await expect(page.locator("video")).toHaveCount(0);
    await expect(trigger).toBeFocused();

    await page.getByRole("button", { name: "View details for Unmeasured details check" }).click();
    const unmeasuredPanel = page.getByRole("dialog", { name: "Unmeasured details check" });
    await expect(unmeasuredPanel.getByText("Estimating time remaining…")).toBeVisible();
    await unmeasuredPanel.getByText("Processing details", { exact: true }).click();
    await expect(
      unmeasuredPanel.getByRole("progressbar", { name: "Current step in progress" }),
    ).not.toHaveAttribute("aria-valuenow");
    await page.keyboard.press("Escape");

    await page.getByRole("button", { name: "View details for Recently ready example" }).click();
    const readyPanel = page.getByRole("dialog", { name: "Recently ready example" });
    await expect(readyPanel.getByRole("progressbar", { name: "Clip preparation" })).toHaveAttribute(
      "aria-valuenow",
      "100",
    );
    await readyPanel.getByText("Processing details", { exact: true }).click();
    await expect(readyPanel.getByText("Finished", { exact: true })).toBeVisible();
    await expect(readyPanel.getByRole("progressbar", { name: /Current step/ })).toHaveCount(0);
    expect(errors).toEqual([]);
  });
}
