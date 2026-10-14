import { expect, test } from "@playwright/test";
import { findClippedContent } from "./clipped-content";
import { channelId, installTunerBackend } from "./tuner-backend";

// Phone Watch gate (#1785, native mock 5e, decision N5): at 390 × 844 the picture runs edge to
// edge of the content column with nothing drawn on it, and the controls sit under it. Runs on the
// tuner fixture because Watch needs a real stream, and in every tuner browser (WebKit stands in for
// a phone's Safari). The shell routes' phone gate (phone-layout.spec.ts) has no channel to watch.
test.use({ viewport: { width: 390, height: 844 } });

test("Watch at phone width puts the picture edge to edge and the controls under it", async ({ page }) => {
  await installTunerBackend(page);
  await page.goto(`/channels/${channelId(1)}/watch`);
  const video = page.locator("video");
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.readyState)).toBeGreaterThanOrEqual(2);

  // Edge to edge: the picture spans the content column to the viewport's right edge, 16:9.
  const main = await page.locator("main").boundingBox();
  const picture = await video.boundingBox();
  expect(main).not.toBeNull();
  expect(picture).not.toBeNull();
  if (!main || !picture) return;
  expect(Math.round(picture.x)).toBe(Math.round(main.x));
  expect(Math.round(picture.x + picture.width)).toBe(390);
  expect(Math.round(picture.height)).toBe(Math.round((picture.width * 9) / 16));

  // Nothing on the picture; the four controls under it.
  await expect(page.getByRole("group", { name: "Playback controls" })).toHaveCount(0);
  for (const name of ["Previous", "Channel −", "Pause", "Channel +"]) {
    const control = page.getByRole("button", { name, exact: true });
    await expect(control).toBeVisible();
    expect((await control.boundingBox())?.y ?? 0).toBeGreaterThanOrEqual(picture.y + picture.height);
  }

  const clipped = await findClippedContent(page);
  expect(
    clipped.map((c) => `${c.element} "${c.text}" cut ${c.px}px on the ${c.side} by ${c.clippedBy}`),
    "Watch should not clip content at 390 px",
  ).toEqual([]);

  await page.getByRole("button", { name: "Channel +", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/channels/${channelId(2)}/watch$`));
});
