import { expect, test } from "@playwright/test";

test("guide focus updates rich detail and empty filters stay unavailable", async ({ page }) => {
  await page.goto("/iframe.html?id=loomarr-components-guide-surface--pointer&viewMode=story");

  const bart = page.getByRole("button", {
    name: /Springfield Classics, The Simpsons · Bart the Mother/,
  });
  const lisa = page.getByRole("button", {
    name: /Springfield Classics, The Simpsons · Lisa Gets an A/,
  });
  await expect(page.getByRole("group", { name: "Channel schedule" })).toBeVisible();
  await expect(bart).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("S10E03", { exact: false })).toBeVisible();
  await expect(page.getByText("1998 · TV-PG · Animation · Comedy")).toBeVisible();

  await lisa.focus();
  await expect(lisa).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("Lisa Gets an A", { exact: true }).last()).toBeVisible();
  await expect(page.getByText("S10E07", { exact: false })).toBeVisible();

  await expect(page.getByRole("button", { name: "Favourites channels" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Recent channels" })).toBeDisabled();
});
