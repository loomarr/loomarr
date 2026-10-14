import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const eventPage = {
  items: [
    {
      id: "event-transition",
      occurredAt: 1_780_000_000_000,
      receivedAt: 1_780_000_000_080,
      level: "error",
      source: "web",
      subsystem: "player",
      event: "player.transition_failed",
      message: "Replacement source missed its handoff deadline.",
      processRunId: "process-19",
      requestId: "request-7",
      attributes: { transport: "hls_js", drift_ms: 2140 },
    },
  ],
  dropped: 2,
};

test("Logs filters, expands evidence, and downloads the visible page", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin" });
  await page.route("**/v1/diagnostics/events**", async (route) => {
    if (route.request().headers().accept?.includes("application/x-ndjson")) {
      return route.fulfill({
        status: 200,
        contentType: "application/x-ndjson",
        body: `${JSON.stringify(eventPage.items[0])}\n`,
      });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(eventPage) });
  });
  await page.route("**/v1/diagnostics/verbose-capture", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ active: false }) }),
  );

  await page.goto("/settings/system/diagnostics?view=logs");
  await expect(page.getByRole("heading", { name: "Diagnostics", exact: true })).toBeVisible();
  await expect(page.getByText("Replacement source missed its handoff deadline.")).toBeVisible();
  await expect(page.getByText("2 events dropped since startup")).toBeVisible();

  await page.getByRole("button", { name: /Replacement source missed/ }).click();
  await page.locator("summary:visible").filter({ hasText: "Technical details" }).click();
  await expect(page.locator("pre:visible").filter({ hasText: "drift_ms" })).toBeVisible();
  await expect(page.locator('button[aria-label="Copy Request id"]:visible')).toBeVisible();

  const filteredRequest = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === "/v1/diagnostics/events" && url.searchParams.get("subsystem") === "player";
  });
  await page.getByRole("button", { name: "More filters" }).click();
  await page.getByRole("textbox", { name: "Subsystem" }).fill("player");
  await filteredRequest;
  await expect(page).toHaveURL(/subsystem=player/);

  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download logs" }).click();
  expect((await download).suggestedFilename()).toMatch(/loomarr-diagnostics-.*\.ndjson/);
});

test("Media processes follows a live process and downloads retained output", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin" });
  const run = {
    id: "process-19",
    purpose: "channel-segment",
    status: "running",
    startedAt: 1_780_000_000_000,
    updatedAt: 1_780_000_001_000,
    channelId: "channel-7",
    target: "commercial-02.ts",
    outputBytes: 120,
    discardedLines: 2,
  };
  await page.route("**/v1/diagnostics/processes?**", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [run] }) }),
  );
  await page.route("**/v1/diagnostics/processes/process-19", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        run,
        progress: [{ frame: 200, occurredAt: run.updatedAt, outTimeMs: 8_000, speed: 1.02 }],
        progressTruncated: false,
      }),
    }),
  );
  await page.route("**/v1/diagnostics/processes/process-19/output", (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/plain",
      headers: { "X-Diagnostic-Truncated": "true", "X-Diagnostic-Discarded-Lines": "2" },
      body: "[2026-08-24T01:00:00Z] frame=200 healthy\n[2026-08-24T01:00:01Z] warning retry",
    }),
  );

  await page.goto("/settings/system/diagnostics?view=process");
  await expect(page.getByRole("heading", { name: "Media processes" })).toBeVisible();
  await expect(page.getByText("channel-segment").first()).toBeVisible();
  await expect(page.getByText(/Frame 200/)).toBeVisible();
  await page.getByRole("textbox", { name: "Search Process output" }).fill("warning");
  await expect(page.getByText(/warning retry/)).toBeVisible();
  await expect(page.getByText(/frame=200 healthy/)).toBeHidden();
  await page.getByRole("button", { name: "Timestamps" }).click();
  await expect(page.getByText("warning retry", { exact: true })).toBeVisible();
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download output" }).click();
  expect((await download).suggestedFilename()).toBe("loomarr-process-process-19.log");
});

test("Troubleshooting report reviews exact contents before downloading the redacted ZIP", async ({
  page,
}) => {
  await installMockBackend(page, { authed: true, role: "admin" });
  await page.route("**/v1/diagnostics/support-bundle/preview", async (route) => {
    const selection = route.request().postDataJSON();
    expect(selection).toMatchObject({ events: true, processes: true, processOutput: true });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        estimatedBytes: 2048,
        manifest: {
          formatVersion: "loomarr.support-bundle.v1",
          generatedAt: Date.UTC(2026, 7, 24, 12),
          selection,
          effectiveFrom: selection.from,
          effectiveTo: selection.to,
          loomarr: { version: "v0.9.0" },
          clientVersions: ["web:v0.9.0"],
          entries: [
            { name: "system.json", uncompressedBytes: 512 },
            { name: "events.ndjson", uncompressedBytes: 1536 },
          ],
          counts: {
            events: 14,
            processes: 2,
            processOutputs: 1,
            eventRecorderDrops: 0,
            discardedProcessLines: 3,
            redactions: 5,
          },
          truncationReasons: [],
          uncompressedBytes: 2048,
          finalArchiveBytes: 0,
        },
      }),
    });
  });
  await page.route("**/v1/diagnostics/support-bundle", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/zip",
      headers: { "Content-Disposition": 'attachment; filename="loomarr-support-20260824T120000Z.zip"' },
      body: "PK mock bundle",
    }),
  );

  await page.goto("/settings/system/diagnostics?view=logs");
  await page.getByRole("button", { name: "Download troubleshooting report" }).click();
  await expect(page.getByRole("heading", { name: "Download troubleshooting report" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Troubleshooting report summary" })).toContainText(
    "About 2.0 KiB",
  );
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download report" }).click();
  expect((await download).suggestedFilename()).toBe("loomarr-support-20260824T120000Z.zip");
});
