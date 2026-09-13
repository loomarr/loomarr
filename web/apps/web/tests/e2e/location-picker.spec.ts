import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

test("location search waits for typing and keeps its mobile controls clear", async ({ page }) => {
  const backend = await installMockBackend(page, { authed: true });
  backend.state.edits["server.public_url"] = "http://loomarr.test";
  const locationQueries: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname === "/v1/locations") locationQueries.push(url.searchParams.get("q") ?? "");
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/wizard");
  await expect(page.getByRole("heading", { name: /where are your channels watched/i })).toBeVisible();

  const input = page.getByRole("combobox", { name: "Location" });
  await input.pressSequentially("North Greenbush", { delay: 40 });
  const option = page.getByRole("option", { name: "North Greenbush, United States" });
  await expect(option).toBeVisible();
  await expect(option).toContainText("New York, United States");
  expect(locationQueries).toEqual(["North Greenbush"]);

  const [optionBox, automaticBox] = await Promise.all([
    option.boundingBox(),
    page.getByRole("button", { name: "Use my location" }).boundingBox(),
  ]);
  expect(optionBox).not.toBeNull();
  expect(automaticBox).not.toBeNull();
  expect((optionBox?.y ?? 0) + (optionBox?.height ?? 0)).toBeLessThanOrEqual(automaticBox?.y ?? 0);

  await option.click();
  await expect(input).toHaveValue("North Greenbush, United States");
  await expect(page.getByRole("button", { name: "Continue" })).toBeEnabled();
});
