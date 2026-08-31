import { expect, type Page, test } from "@playwright/test";

interface StorybookPreviewWindow {
  __STORYBOOK_PREVIEW__?: { storyRenders?: { id: string; phase?: string }[] };
}

const story = (id: string) => `/iframe.html?id=${id}&viewMode=story`;

const openStory = async (page: Page, id: string) => {
  await page.goto(story(id));
  await page.locator("#storybook-root > *").first().waitFor({ state: "visible" });
  await page.waitForFunction(
    (storyId) => {
      const renders = (window as unknown as StorybookPreviewWindow).__STORYBOOK_PREVIEW__?.storyRenders;
      const phase = renders?.find((render) => render.id === storyId)?.phase;
      return phase === "finished" || phase === "errored" || phase === "aborted";
    },
    id,
    { timeout: 15_000 },
  );
  await page.evaluate(async () => document.fonts.ready);
};

test.describe("People workspace certification", () => {
  test("the mixed roster stays within the viewport and preserves practical touch targets", async ({
    page,
  }, testInfo) => {
    await openStory(page, "people-userspage--mixed-roster");

    const widths = await page.evaluate(() => ({
      viewport: window.innerWidth,
      document: document.documentElement.scrollWidth,
      root: document.querySelector("#storybook-root")?.scrollWidth ?? 0,
    }));
    expect(widths.document, "the page must not create horizontal document overflow").toBeLessThanOrEqual(
      widths.viewport,
    );
    expect(widths.root, "the People workspace must stay inside its mobile gutter").toBeLessThanOrEqual(
      widths.viewport,
    );

    if (testInfo.project.name === "mobile") {
      const undersized = await page
        .locator("#storybook-root button, #storybook-root input")
        .evaluateAll((elements) =>
          elements
            .filter((element) => {
              const style = getComputedStyle(element);
              const rect = element.getBoundingClientRect();
              return (
                style.visibility !== "hidden" &&
                style.display !== "none" &&
                rect.right > 0 &&
                rect.bottom > 0 &&
                rect.left < window.innerWidth &&
                rect.top < window.innerHeight
              );
            })
            .map((element) => {
              const rect = element.getBoundingClientRect();
              return {
                name: element.getAttribute("aria-label") ?? element.textContent?.trim(),
                ...rect.toJSON(),
              };
            })
            .filter((rect) => rect.width < 36 || rect.height < 36),
        );
      expect(undersized, "visible People controls must be at least 36×36 CSS pixels").toEqual([]);
    }
  });

  for (const overlay of [
    {
      id: "people-userspage--selected-offline-ready",
      trigger: "Manage Katherine Johnson",
      snapshot: "people-workspace-selected.png",
    },
    {
      id: "people-userspage--import-open",
      trigger: "Import from Emby/Jellyfin",
      snapshot: "people-workspace-import.png",
    },
    {
      id: "people-userspage--local-account-open",
      trigger: "Add local account",
      snapshot: "people-workspace-local.png",
    },
  ]) {
    test(`${overlay.trigger} traps focus and returns it to its page control`, async ({ page }) => {
      await openStory(page, overlay.id);
      const dialog = page.getByRole("dialog");
      await expect(dialog).toBeVisible();
      await expect(page).toHaveScreenshot(overlay.snapshot);

      for (let step = 0; step < 10; step++) {
        await page.keyboard.press("Tab");
        await expect
          .poll(() => dialog.evaluate((element) => element.contains(document.activeElement)), {
            message: `focus escaped ${overlay.trigger} after ${step + 1} Tab presses`,
          })
          .toBe(true);
      }

      await page.keyboard.press("Escape");
      await expect(dialog).not.toBeVisible();
      await expect(page.getByRole("button", { name: overlay.trigger })).toBeFocused();
    });
  }

  test("the mixed workspace survives Windows forced colors", async ({ page }) => {
    await page.emulateMedia({ forcedColors: "active" });
    await openStory(page, "people-userspage--mixed-roster");
    await expect(page).toHaveScreenshot("people-workspace-forced-colors.png");
  });
});
