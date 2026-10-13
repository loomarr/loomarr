import { expect, test } from "@playwright/test";

// #1899: in the narrow column a lineup row stacks, and its controls must read in the order Tab
// visits them. #1896 had placed remove top right, beside the title, while it stayed last in Tab
// order. This walks a series row with the keyboard and checks that each control it lands on sits
// after the previous one in reading order: lower, or on the same line and further right.
test("a narrow lineup row's Tab order is its visual order", async ({ page }) => {
  await page.goto("/iframe.html?id=channels-channellineupeditor--narrow-column&viewMode=story");

  const grip = page.getByRole("button", { name: "Reorder Bowling Alley Blues" });
  await expect(page.getByRole("button", { name: "Keep — Bowling Alley Blues" })).toBeEnabled();
  await grip.focus();

  const stops: { label: string | null; x: number; y: number; height: number }[] = [];
  for (let i = 0; i < 8; i++) {
    stops.push(
      await page.evaluate(() => {
        // Relative to the row: focusing a control scrolls the lineup's own scroller, which moves
        // every viewport coordinate between one stop and the next.
        const el = document.activeElement as HTMLElement;
        const box = el.getBoundingClientRect();
        const row = (el.closest("li") as HTMLElement).getBoundingClientRect();
        return {
          label: el.getAttribute("aria-label"),
          x: box.left - row.left,
          y: box.top - row.top,
          height: box.height,
        };
      }),
    );
    await page.keyboard.press("Tab");
  }

  expect(stops.map((s) => s.label)).toEqual([
    "Reorder Bowling Alley Blues",
    "Bowling Alley Blues first season",
    "Bowling Alley Blues last season",
    "Keep — Bowling Alley Blues",
    "Less like this — Bowling Alley Blues",
    "Never — Bowling Alley Blues",
    "Surprise me — Bowling Alley Blues",
    "Remove Bowling Alley Blues",
  ]);
  for (let i = 1; i < stops.length; i++) {
    const prev = stops[i - 1]!;
    const next = stops[i]!;
    // Same line when the vertical centres are within half a control of each other.
    const sameLine = Math.abs(next.y + next.height / 2 - (prev.y + prev.height / 2)) < prev.height / 2;
    expect(sameLine ? next.x > prev.x : next.y > prev.y, `${next.label} reads after ${prev.label}`).toBe(
      true,
    );
  }

  // The narrow remove is a full 44 px touch target.
  const remove = await page.getByRole("button", { name: "Remove Bowling Alley Blues" }).boundingBox();
  expect(remove!.width).toBeGreaterThanOrEqual(44);
  expect(remove!.height).toBeGreaterThanOrEqual(44);
});
