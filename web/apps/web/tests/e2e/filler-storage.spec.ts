import { expect, type Page, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

type StorageScenario = "healthy" | "approaching" | "paused" | "unknown";

const GiB = 1024 ** 3;
const MiB = 1024 ** 2;

const storageFor = (scenario: StorageScenario) => {
  const common = {
    automatic: true,
    totalBytes: 64 * GiB,
    managedBytes: 5.5 * GiB,
    reservedBytes: 0,
    filesystemReservedBytes: 0,
    softBudgetBytes: 6.4 * GiB,
    hardReserveBytes: 6.4 * GiB,
  };
  switch (scenario) {
    case "approaching":
      return { ...common, state: "approaching", freeBytes: 8 * GiB, availableBytes: 512 * MiB };
    case "paused":
      return {
        ...common,
        state: "paused",
        freeBytes: 2 * GiB,
        availableBytes: 0,
        pausedBy: "host_reserve",
      };
    case "unknown":
      return {
        ...common,
        state: "unknown",
        freeBytes: 0,
        availableBytes: 0,
        pausedBy: "capacity_unavailable",
      };
    default:
      return { ...common, state: "healthy", freeBytes: 32 * GiB, availableBytes: 0.9 * GiB };
  }
};

const readinessFor = (scenario: StorageScenario) => ({
  ready: scenario === "healthy" || scenario === "approaching",
  nextAction:
    scenario === "paused"
      ? "free_disposable_space"
      : scenario === "unknown"
        ? "choose_filler_folder"
        : "none",
  repairs: { count: 0 },
  fetch: { enabled: true, catalogClips: 12 },
  storage: storageFor(scenario),
  pipeline: {
    runnable: 0,
    scheduled: 0,
    inProgress: 0,
    needsDecision: 0,
    recoverable: 0,
    ready: 12,
    complete: 0,
    rejected: 0,
    dismissed: 0,
  },
  pool: { clips: 12, breakBody: 10, eligible: 10, untagged: 0, channels: [] },
  acquisitions: [],
});

const expectNoHorizontalOverflow = async (page: Page) => {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
};

test("storage recovery stays truthful and usable at desktop and phone widths", async ({ page }) => {
  const backend = await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  let scenario: StorageScenario = "healthy";
  let cleanupFails = false;
  let cleanupRequests = 0;

  await page.route("**/v1/filler/readiness**", (route) => route.fulfill({ json: readinessFor(scenario) }));
  await page.route("**/v1/filler/storage/cleanup**", (route) => {
    if (route.request().method() === "GET") {
      return route.fulfill({ json: { items: 2, bytes: 2 * MiB } });
    }
    cleanupRequests += 1;
    return cleanupFails
      ? route.fulfill({ status: 503, json: { title: "Cleanup unavailable" } })
      : route.fulfill({
          json: {
            removedItems: 2,
            removedBytes: 2 * MiB,
            failedItems: 0,
            remaining: { items: 0, bytes: 0 },
          },
        });
  });

  for (const viewport of [
    { width: 1440, height: 1000, folder: "/media/filler-desktop" },
    { width: 390, height: 844, folder: "/media/filler-phone" },
  ]) {
    await page.setViewportSize(viewport);

    scenario = "healthy";
    await page.goto("/filler/sources");
    const healthy = page.getByRole("region", { name: "Storage" });
    await expect(healthy).toContainText("5.5 GB used for filler");
    await expect(healthy).toContainText("Automatic downloads will pause");
    await expect(healthy.getByRole("link", { name: "Storage options" })).toHaveAttribute(
      "href",
      "/filler/settings/storage",
    );
    await expectNoHorizontalOverflow(page);

    scenario = "approaching";
    await page.reload();
    const approaching = page.getByRole("region", { name: "Storage" });
    await expect(approaching).toContainText("512.0 MB is left for new filler");
    await expect(approaching).toContainText("Loomarr will pause before it risks space");
    await expectNoHorizontalOverflow(page);

    scenario = "paused";
    cleanupFails = false;
    await page.reload();
    const paused = page.getByRole("region", { name: "Storage" });
    await expect(paused).toContainText("New filler is paused");
    await expect(paused).toContainText("2.0 MB in 2 unfinished downloads is safe to remove");
    await expect(paused).toContainText("Clips in your library stay untouched");
    const cleanupBeforeSuccess = cleanupRequests;
    await paused.getByRole("button", { name: "Free 2.0 MB" }).click();
    await expect(paused.getByRole("status")).toContainText("Removed 2 temporary folders · freed 2.0 MB");
    expect(cleanupRequests).toBe(cleanupBeforeSuccess + 1);
    await expectNoHorizontalOverflow(page);

    cleanupFails = true;
    await page.reload();
    const cleanupBeforeFailure = cleanupRequests;
    await page.getByRole("region", { name: "Storage" }).getByRole("button", { name: "Free 2.0 MB" }).click();
    await expect(page.getByRole("alert")).toContainText(
      "Temporary files could not be checked or removed. Nothing in your library was changed.",
    );
    expect(cleanupRequests).toBe(cleanupBeforeFailure + 1);

    scenario = "unknown";
    cleanupFails = false;
    await page.reload();
    const unknown = page.getByRole("region", { name: "Storage" });
    await expect(unknown).toContainText("Loomarr cannot safely check the available space in this folder");
    await unknown.getByRole("link", { name: "Choose folder" }).click();
    await expect(page).toHaveURL(/\/filler\/settings\/folders$/);
    const folder = page.getByRole("textbox", { name: "Clip library" });
    await expect(folder).toHaveValue(
      viewport.folder === "/media/filler-desktop" ? "/data/filler" : "/media/filler-desktop",
    );
    await folder.fill(viewport.folder);
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect.poll(() => backend.state.edits["filler.dir"]).toBe(viewport.folder);
    await expectNoHorizontalOverflow(page);
  }
});
