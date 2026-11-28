import { expect, type Page, test } from "@playwright/test";
import { channelId, installTunerBackend } from "./tuner-backend";

// The web's Watch route on The Cross's watching screen (#4731), in a real browser: the keys, the
// list, pause and live, and the three focus and pointer rules the shared screen holds. Runs on the
// tuner fixture because Watch needs a real stream.

const quietRow = (page: Page) => page.getByTestId("watch-quiet-row");
const amberRings = (page: Page) => page.locator('[data-testid="ring-actionFocus"]');
const list = (page: Page) => page.getByRole("listbox", { name: "Channels" });
const onChannel = (page: Page, number: number) =>
  expect(page).toHaveURL(new RegExp(`/watch/${channelId(number)}$`));
// The chrome rests five seconds after the last activity; a little more for the fade.
const RESTS = { timeout: 8_000 };

const openWatch = async (page: Page, number = 5) => {
  await installTunerBackend(page);
  await page.goto(`/watch/${channelId(number)}`);
  await expect(page.getByTestId("lower-third-numeral")).toHaveText(String(number));
  // A real picture, so pause has something to hold.
  await expect
    .poll(() => page.locator("video").evaluate((el: HTMLVideoElement) => el.readyState))
    .toBeGreaterThanOrEqual(2);
};

test("Down flips at once and shows only the lower third", async ({ page }) => {
  await openWatch(page);
  await page.keyboard.press("ArrowDown");
  await onChannel(page, 6);
  await expect(page.getByTestId("lower-third-numeral")).toHaveText("6");
  await expect(list(page)).toHaveCount(0);
});

test("Channels opens the list; the arrows look, Enter tunes, Esc closes", async ({ page }) => {
  await openWatch(page);
  await page.getByRole("button", { name: "Channels" }).click();
  await expect(list(page)).toBeVisible();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  await onChannel(page, 5);
  await page.keyboard.press("Enter");
  await onChannel(page, 7);
  await page.keyboard.press("Escape");
  await expect(list(page)).toHaveCount(0);
});

test("Space pauses and resumes, and End goes live", async ({ page }) => {
  await openWatch(page);
  await page.keyboard.press(" ");
  await expect(page.getByRole("button", { name: "Play (Space)" })).toBeVisible();
  await page.keyboard.press(" ");
  await expect(page.getByRole("button", { name: "Pause (Space)" })).toBeVisible();
  await page.keyboard.press(" ");
  await page.keyboard.press("End");
  await expect(page.getByRole("button", { name: "Pause (Space)" })).toBeVisible();
});

test("/ finds a channel by name, and digits tune by number", async ({ page }) => {
  await openWatch(page);
  await page.keyboard.press("/");
  await page.getByRole("searchbox").fill("Channel 12");
  await page.keyboard.press("Enter");
  await onChannel(page, 12);
  await page.keyboard.press("Escape");
  await page.keyboard.press("3");
  await page.keyboard.press("Enter");
  await onChannel(page, 3);
});

test("a click on Mute and then a key still lets the screen go quiet", async ({ page }) => {
  await openWatch(page);
  await page.getByRole("button", { name: /^(Mute|Unmute) \(M\)$/ }).click();
  await page.mouse.move(5, 5);
  await page.keyboard.press("m");
  await expect(amberRings(page)).toHaveCount(0);
  await expect(quietRow(page)).toHaveCSS("opacity", "0", RESTS);
});

test.describe("focus and the pointer", () => {
  test("Tab to a control then a click on another draws no ring, and the chrome rests after the pointer leaves", async ({
    page,
  }) => {
    await openWatch(page);
    await page.keyboard.press("Tab");
    await page.getByRole("button", { name: /^Subtitles/ }).click();
    await expect(amberRings(page)).toHaveCount(0);
    // The pointer closes the menu it opened, and leaves.
    await page.getByRole("button", { name: /^Subtitles/ }).click();
    await page.mouse.move(640, 300);
    await expect(quietRow(page)).toHaveCSS("opacity", "0", RESTS);
  });

  test("a modifier alone and then a click draws no ring", async ({ page }) => {
    await openWatch(page);
    for (const modifier of ["Shift", "Meta"]) {
      await page.keyboard.press(modifier);
      await page.getByRole("button", { name: /^Audio/ }).click();
      await expect(amberRings(page)).toHaveCount(0);
      await page.getByRole("button", { name: /^Audio/ }).click();
    }
  });

  test("a menu a key closes hands focus back to its control with the ring; a click's close hands none", async ({
    page,
  }) => {
    await openWatch(page);
    await page.keyboard.press("a");
    await expect(page.getByRole("dialog", { name: "Audio" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("button", { name: /^Audio/ })).toBeFocused();
    await expect(amberRings(page)).toHaveCount(1);

    await page.keyboard.press("a");
    await page.getByTestId("watch-picture").click();
    await expect(page.getByRole("dialog", { name: "Audio" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Audio/ })).not.toBeFocused();
    await expect(amberRings(page)).toHaveCount(0);
  });

  test("the lower third back under a still pointer keeps the chrome awake until the pointer leaves", async ({
    page,
  }) => {
    await openWatch(page);
    const third = await page.getByTestId("watch-lower-third").boundingBox();
    if (!third) throw new Error("no lower third");
    // The pointer rests where the lower third comes back, and does not move while the list opens
    // (find, from its key) and closes (Esc out of find, then out of the list).
    await page.mouse.move(third.x + 20, third.y + third.height / 2);
    await page.keyboard.press("/");
    await expect(list(page)).toBeVisible();
    await page.keyboard.press("Escape");
    await page.keyboard.press("Escape");
    await expect(list(page)).toHaveCount(0);
    await expect(page.getByTestId("watch-lower-third")).toBeVisible();
    await page.waitForTimeout(7_000);
    await expect(quietRow(page)).toHaveCSS("opacity", "1");
    await page.mouse.move(5, 5);
    await expect(quietRow(page)).toHaveCSS("opacity", "0", RESTS);
  });
});
