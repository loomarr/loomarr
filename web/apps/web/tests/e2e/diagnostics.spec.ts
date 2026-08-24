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

test("Application diagnostics filters, expands evidence, and downloads the visible page", async ({
  page,
}) => {
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

  await page.goto("/settings/system/diagnostics?view=application");
  await expect(page.getByRole("heading", { name: "Diagnostics", exact: true })).toBeVisible();
  await expect(page.getByText("player.transition_failed")).toBeVisible();
  await expect(page.getByText("2 events dropped since startup")).toBeVisible();

  await page.getByRole("button", { name: /details/i }).click();
  await expect(page.getByText("Structured attributes")).toBeVisible();
  await expect(page.getByText(/drift_ms/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Copy Request id" })).toBeVisible();

  const filteredRequest = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === "/v1/diagnostics/events" && url.searchParams.get("subsystem") === "player";
  });
  await page.getByRole("textbox", { name: "Subsystem" }).fill("player");
  await filteredRequest;
  await expect(page).toHaveURL(/subsystem=player/);

  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download this page (NDJSON)" }).click();
  expect((await download).suggestedFilename()).toMatch(/loomarr-diagnostics-.*\.ndjson/);
});

test("Playout diagnostics follows a live Process and downloads retained output", async ({ page }) => {
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

  await page.goto("/settings/system/diagnostics?view=playout");
  await expect(page.getByRole("heading", { name: "Playout" })).toBeVisible();
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
