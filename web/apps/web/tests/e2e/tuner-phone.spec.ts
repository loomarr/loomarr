import { expect, test } from "@playwright/test";
import { findClippedContent } from "./clipped-content";
import { channelId, installTunerBackend } from "./tuner-backend";

// Phone Watch gate (#2775; variant J, #3449): at 390 × 844 the top row sits above the picture, the
// picture is a strip edge to edge at 16:9 in the middle of the screen, and the band and keys sit in
// the black space below it, so nothing is drawn on the picture. Runs on the tuner fixture because
// Watch needs a real stream, and in every tuner browser (WebKit stands in for a phone's Safari). The
// shell routes' phone gate (phone-layout.spec.ts) has no channel to watch.
test.use({ viewport: { width: 390, height: 844 } });

test("Watch at phone width draws the picture between the top row and the keys", async ({ page }) => {
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

  // The top row above the picture; the keys below it; nothing on it.
  const leave = page.getByRole("button", { name: "Leave", exact: true });
  await expect(leave).toBeVisible();
  const leaveBox = await leave.boundingBox();
  expect((leaveBox?.y ?? 0) + (leaveBox?.height ?? 0)).toBeLessThanOrEqual(picture.y + 1);
  for (const name of ["Channel up", "Channel down", /^(Play|Pause)$/]) {
    const control = page.getByRole("button", typeof name === "string" ? { exact: true, name } : { name });
    await expect(control).toBeVisible();
    expect((await control.boundingBox())?.y ?? 0).toBeGreaterThanOrEqual(picture.y + picture.height);
  }

  const clipped = await findClippedContent(page);
  expect(
    clipped.map((c) => `${c.element} "${c.text}" cut ${c.px}px on the ${c.side} by ${c.clippedBy}`),
    "Watch should not clip content at 390 px",
  ).toEqual([]);

  await page.getByRole("button", { name: "Channel up", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/watch/${channelId(2)}$`));
});
