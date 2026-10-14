import { expect, test } from "@playwright/test";

// #1202 made the default desktop Guide fit without scrolling sideways. The shared grid (#1659)
// sizes blocks as fractions of the window, so it should fit any width it's given: nothing in its
// frame, nor the page, may scroll horizontally. The story's own baseline covers how it looks.
test("the Guide grid fits its frame on a 1440 by 900 desktop", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/iframe.html?id=loomarr-components-guide-grid--evening&viewMode=story");
  await expect(page.getByRole("button", { name: /The pilot/ })).toBeVisible();

  const overflowing = await page.evaluate(() => {
    const frame = document.querySelector("#storybook-root > div") as HTMLElement | null;
    const all = [document.scrollingElement, frame, ...(frame?.querySelectorAll("*") ?? [])];
    return all
      .filter((el): el is HTMLElement => el instanceof HTMLElement)
      .filter((el) => el.scrollWidth > el.clientWidth + 1 && getComputedStyle(el).overflowX !== "hidden")
      .map((el) => `${el.tagName} ${el.scrollWidth}>${el.clientWidth}`);
  });
  expect(overflowing).toEqual([]);
});
