import { expect, test } from "@playwright/test";
import { findClippedContent } from "./clipped-content";
import { channelId, installTunerBackend } from "./tuner-backend";

// Watch at phone width on the web (#4731): the watching screen draws its over-the-picture
// arrangement at every width until the window and phone arrangements land (#4732, #4738). The
// picture spans the screen edge to edge, the back control and the quiet row are on it and unclipped,
// and touch alone tunes and pauses. Runs on the tuner fixture because Watch needs a real stream, and in every tuner
// browser (WebKit stands in for a phone's Safari). The shell routes' phone gate
// (phone-layout.spec.ts) has no channel to watch.
test.use({ hasTouch: true, viewport: { width: 390, height: 844 } });

test("Watch at phone width draws the picture edge to edge with its controls on it, unclipped", async ({
  page,
}) => {
  await installTunerBackend(page);
  await page.goto(`/watch/${channelId(1)}`);
  const video = page.locator("video");
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.readyState)).toBeGreaterThanOrEqual(2);

  // The <video> box fills the frame; the picture is drawn centred in it (contain, centred), so
  // measure the drawn picture, not the box.
  const picture = await video.evaluate((el: HTMLVideoElement) => {
    const box = el.getBoundingClientRect();
    const height = Math.min(box.height, (box.width * el.videoHeight) / el.videoWidth);
    return { height, width: box.width, x: box.x, y: box.y + (box.height - height) / 2 };
  });
  expect(Math.round(picture.x)).toBe(0);
  expect(Math.round(picture.x + picture.width)).toBe(390);
  expect(Math.round(picture.height)).toBe(Math.round((picture.width * 9) / 16));

  // The named back control and the quiet row stand over the picture.
  await expect(page.getByTestId("watch-back")).toBeVisible();
  await expect(page.getByRole("toolbar", { name: "Player controls" })).toBeVisible();

  const clipped = await findClippedContent(page);
  expect(
    clipped.map((c) => `${c.element} "${c.text}" cut ${c.px}px on the ${c.side} by ${c.clippedBy}`),
    "Watch should not clip content at 390 px",
  ).toEqual([]);

  // A phone has no keys: a tap opens Channels, a tap on a row tunes it, a tap on the picture pauses.
  await page.getByRole("button", { name: "Channels" }).tap();
  await page.getByRole("option", { name: /Channel 2\b/ }).tap();
  await expect(page).toHaveURL(new RegExp(`/watch/${channelId(2)}$`));
  await expect.poll(() => video.evaluate((el: HTMLVideoElement) => el.readyState)).toBeGreaterThanOrEqual(2);
  await page.getByTestId("watch-picture").tap();
  await expect(page.getByRole("button", { name: "Play (Space)" })).toBeVisible();
});
