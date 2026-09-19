import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

for (const width of [1440, 390]) {
  test(`Library clip inspection keeps context and editing optional at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 844 });
    await page.emulateMedia({ reducedMotion: "reduce" });
    await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
    const clips = Array.from({ length: 24 }, (_, index) => ({
      hash: `clip-${index}`,
      name: `Candy commercial ${index}`,
      kind: index === 0 ? "unclassified" : "commercial",
      durationMs: 30000,
      aiTagged: false,
      tagged: false,
      source: "classic-ads",
      playCount: 0,
      playsCounted: true,
      ...(index === 1 ? { era: 1977, audience: "kids", brand: "Tootsie Pop", assertedTags: ["candy"] } : {}),
    }));
    const writes: unknown[] = [];
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.route("**/v1/filler?*", (route) => {
      const hash = new URL(route.request().url()).searchParams.get("hashes");
      const rows = hash
        ? clips
            .filter((clip) => clip.hash === hash)
            .map((clip) => ({
              ...clip,
              sourceUrl: "https://archive.org/details/exact-item",
              enrichment:
                clip.hash === "clip-0"
                  ? {
                      state: "details_limited",
                      facts: [{ axis: "kind", evidence: "item_metadata" }],
                    }
                  : {
                      state: "complete",
                      facts: [
                        { axis: "kind", evidence: "item_metadata" },
                        { axis: "brand", evidence: "content_observation" },
                      ],
                    },
            }))
        : clips;
      return route.fulfill({ json: { clips: rows, total: rows.length } });
    });
    await page.route("**/v1/filler/tags", async (route) => {
      writes.push(route.request().postDataJSON());
      await route.fulfill({ json: clips[0] });
    });
    // Mounting/unmounting and degraded preview are deterministic here. Actual playback is
    // checked separately against the real isolated runtime, not claimed by a fake media body.
    await page.route("**/v1/filler/media/**", (route) =>
      route.fulfill({ status: 404, body: "No preview in this fixture" }),
    );
    await page.goto("/filler/library?q=Candy&view=list");
    const title = page.getByRole("button", { name: "View details for Candy commercial 0", exact: true });
    await page.getByRole("checkbox", { name: "Select Candy commercial 0", exact: true }).check();
    await title.focus();
    const scrollBefore = await title.evaluate((element) => {
      let ancestor = element.parentElement;
      while (ancestor && ancestor.scrollHeight <= ancestor.clientHeight) ancestor = ancestor.parentElement;
      return ancestor?.scrollTop ?? 0;
    });
    await page.keyboard.press("Enter");
    const panel = page.getByRole("dialog", { name: "Candy commercial 0", exact: true });
    await expect(panel).toBeVisible();
    await expect(panel.locator("video")).toHaveCount(1);
    await expect(panel.getByText("Ready", { exact: true })).toBeVisible();
    await expect(panel.getByRole("link", { name: "View original" })).toHaveAttribute(
      "href",
      "https://archive.org/details/exact-item",
    );
    await expect(panel.getByText("Details limited", { exact: true })).toBeVisible();
    await panel.getByText("More about this clip", { exact: true }).click();
    await expect(panel.getByText("Item details", { exact: true })).toBeVisible();
    await expect(panel.getByText(/Untagged|review required|Geography unknown/)).toHaveCount(0);
    await expect(panel.getByRole("button", { name: "Use in a channel" })).toHaveCount(0);
    expect(writes).toEqual([]);
    await panel.getByRole("button", { name: "Edit details", exact: true }).click();
    const editor = panel.getByRole("region", { name: "Edit details: Candy commercial 0" });
    await expect(editor).toBeFocused();
    await expect(panel.locator("video")).toHaveCount(0);
    await expect(editor.getByText("Location and broadcast details")).toBeVisible();
    await editor.getByRole("textbox", { name: "Advertiser", exact: true }).fill("Do not save this");
    await editor.getByRole("button", { name: "Cancel", exact: true }).click();
    expect(writes).toEqual([]);
    await expect(panel.getByText("Do not save this", { exact: true })).toHaveCount(0);
    await page.keyboard.press("Escape");
    await expect(panel).toHaveCount(0);
    await expect(page.locator("video")).toHaveCount(0);
    await expect(title).toBeFocused();
    await expect(page).toHaveURL(/q=Candy.*view=list/);
    await expect(page.getByText("1 clip selected", { exact: true })).toBeVisible();
    expect(
      await title.evaluate((element) => {
        let ancestor = element.parentElement;
        while (ancestor && ancestor.scrollHeight <= ancestor.clientHeight) ancestor = ancestor.parentElement;
        return ancestor?.scrollTop ?? 0;
      }),
    ).toBe(scrollBefore);
    await page.getByRole("radio", { name: "Grid", exact: true }).click();
    await page.getByRole("button", { name: "View details for Candy commercial 1", exact: true }).click();
    const knownPanel = page.getByRole("dialog", { name: "Candy commercial 1", exact: true });
    await expect(knownPanel.getByText("1977", { exact: true })).toBeVisible();
    await expect(knownPanel.getByText("Tootsie Pop", { exact: true })).toBeVisible();
    await expect(knownPanel.getByText(/Adding details|Details limited/)).toHaveCount(0);
    await knownPanel.getByRole("button", { name: "Close", exact: true }).click();
    await expect(knownPanel).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    expect(errors).toEqual([]);
  });
}
