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
