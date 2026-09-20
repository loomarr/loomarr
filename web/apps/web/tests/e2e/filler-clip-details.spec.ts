import { expect, type Page, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const expectNoHorizontalOverflow = async (page: Page) => {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
};

test("clip-detail search stays simple through setup, status, advanced controls, and removal", async ({
  page,
}) => {
  const backend = await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });

  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/filler/settings/details");
  await expect(page.getByRole("heading", { name: "Clip details" })).toBeVisible();
  await expect(page.getByText("Clip details are ready")).toBeVisible();
  await expect(page.getByText(/Web search is used only when they cannot identify a clip/)).toBeVisible();
  await expect(page.getByText("Monthly web searches")).not.toBeVisible();
  await expect(page.getByText("Wikidata")).not.toBeVisible();
  await expectNoHorizontalOverflow(page);

  await page.getByRole("button", { name: "Add web search" }).click();
  const setup = page.getByRole("dialog");
  await expect(setup.getByRole("heading", { name: "Add web search" })).toBeVisible();
  await expect(setup.getByRole("radio", { name: /Brave Search/ })).toBeChecked();
  await setup.getByLabel("Brave Search API key").fill("browser-secret");
  await setup.getByRole("button", { name: "Test and add" }).click();

  await expect(page.getByText("Web search ready")).toBeVisible();
  await expect
    .poll(() => backend.state.fillerResearchTests)
    .toEqual([{ provider: "brave", apiKey: "browser-secret" }]);
  expect(backend.state.edits["filler.research.brave_api_key"]).toBe("browser-secret");
  await page.getByText("Advanced", { exact: true }).click();
  await expect(page.getByRole("switch", { name: "Wikidata" })).toBeChecked();
  await expect(page.getByRole("switch", { name: "Wikipedia" })).toBeChecked();
  await expect(page.getByRole("switch", { name: "Archive.org" })).toBeChecked();
  await expect(page.getByRole("switch", { name: "Library of Congress" })).toBeChecked();
  await page.getByRole("switch", { name: "Archive.org" }).click();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect.poll(() => backend.state.edits["filler.research.archive_enabled"]).toBe("false");
  await expect(page.getByRole("spinbutton", { name: "Monthly web searches" })).toHaveValue("100");
  await page.getByRole("button", { name: "Test connection" }).click();
  await expect.poll(() => backend.state.fillerResearchTests.at(-1)).toEqual({ provider: "brave" });
  await page.getByRole("button", { name: "Remove web search" }).click();
  await expect(page.getByText("Clip details are ready")).toBeVisible();
  expect(backend.state.edits["filler.research.web_provider"]).toBe("none");
  expect(backend.state.edits["filler.research.brave_api_key"]).toBe("");

  backend.state.fillerResearchStatus = {
    provider: "searxng",
    configured: true,
    state: "degraded",
    requestCount: 9,
    requestLimit: 25,
    lastFailureAt: "2026-09-20T13:00:00Z",
  };
  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  await expect(page.getByText("Web search needs a check")).toBeVisible();
  await expect(page.getByText(/SearXNG · 9 of 25 searches this month/)).toBeVisible();
  await page.getByText("Advanced", { exact: true }).click();
  await page.getByRole("button", { name: "Replace provider" }).click();
  const replacement = page.getByRole("dialog");
  await replacement.getByText("Advanced / self-hosted").click();
  await replacement.getByRole("radio", { name: /SearXNG/ }).check();
  await replacement.getByLabel("SearXNG address").fill("https://search.home.example");
  await replacement.getByRole("button", { name: "Test and add" }).click();
  await expect(page.getByText("Web search ready")).toBeVisible();
  await expect
    .poll(() => backend.state.fillerResearchTests.at(-1))
    .toEqual({
      provider: "searxng",
      endpoint: "https://search.home.example",
    });
  expect(backend.state.edits["filler.research.searxng_url"]).toBe("https://search.home.example");
  await expectNoHorizontalOverflow(page);
});
