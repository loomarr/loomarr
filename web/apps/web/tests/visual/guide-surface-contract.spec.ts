import { expect, test } from "@playwright/test";

test("guide focus updates rich detail and empty filters stay unavailable", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-guide-surface--pointer&viewMode=story");

  const teapot = page.getByRole("button", {
    name: /Cartoon Classics, Kettle Kids · The Borrowed Teapot/,
  });
  const kite = page.getByRole("button", {
    name: /Cartoon Classics, Kettle Kids · The Runaway Kite/,
  });
  await expect(page.getByRole("group", { name: "Channel schedule" })).toBeVisible();
  await expect(teapot).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("S10E03", { exact: false })).toBeVisible();
  await expect(page.getByText("1998 · TV-PG · Animation · Comedy")).toBeVisible();

  await kite.focus();
  await expect(kite).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("The Runaway Kite", { exact: true }).last()).toBeVisible();
  await expect(page.getByText("S10E07", { exact: false })).toBeVisible();

  await expect(page.getByRole("button", { name: "Favorites channels" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Recent channels" })).toBeDisabled();
});
