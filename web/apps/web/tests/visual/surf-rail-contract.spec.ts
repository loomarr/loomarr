import { expect, test } from "@playwright/test";

test("Surf focus follows authoritative now-next detail and tunes without unmounting playback", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=loomarr-components-surf-rail--pointer&viewMode=story");

  const trek = page.getByRole("button", {
    name: "All channels, channel 2, Star Trek Classics",
  });
  await trek.focus();
  await expect(page.getByText("The Best of Both Worlds", { exact: true })).toBeVisible();
  await expect(page.getByText("S03E26", { exact: false })).toBeVisible();
  await expect(page.getByText("Next 8:45 PM · Family")).toBeVisible();

  await trek.press("Enter");
  await expect(page.getByText("PLAYBACK REMAINS MOUNTED · ch-scifi")).toBeVisible();
  await expect(page.getByText("No favourites yet")).toBeVisible();
});
