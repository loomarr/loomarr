import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const sourceReadinessShot = async (page: Page, name: string) => {
  await page.evaluate(() => document.fonts.ready);
  await expect(page).toHaveScreenshot(`${name}.png`, { fullPage: true });
};

test("Archive source search stays in flow and registers only after confirmation", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  await page.route("https://archive.org/embed/**", (route) =>
    route.fulfill({ status: 200, contentType: "text/html", body: "<title>Archive preview</title>" }),
  );
  const registrations: unknown[] = [];
  await page.route("**/v1/filler/sources", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    registrations.push(route.request().postDataJSON());
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        id: "archive:classic_tv_commercials",
        uri: "classic_tv_commercials",
        label: "Classic TV Commercials",
        enabled: true,
      }),
    });
  });

  for (const viewport of [
    { name: "desktop", width: 1280, height: 600 },
    { name: "mobile", width: 390, height: 844 },
  ]) {
    const registrationsBeforeSearch = registrations.length;
    await page.setViewportSize(viewport);
    await page.goto("/filler/sources");
    const secondaryNavigation = page.getByRole("navigation", { name: "Filler sections" });
    const navigationHeights = await secondaryNavigation.evaluate((navigation) => ({
      container: navigation.getBoundingClientRect().height,
      tallestLink: Math.max(
        ...Array.from(navigation.querySelectorAll("a"), (link) => link.getBoundingClientRect().height),
      ),
    }));
    expect(
      navigationHeights.container,
      `${viewport.name} secondary navigation must not clip its links`,
    ).toBeGreaterThanOrEqual(navigationHeights.tallestLink);
    const input = page.getByRole("combobox", { name: "Find an Archive.org collection" });
    await input.fill("classic tv commercials");
    const option = page.getByRole("option", { name: /Classic TV Commercials/i });
    await expect(option).toBeVisible();

    const inputBox = await input.boundingBox();
    const resultsBox = await page.getByRole("listbox").boundingBox();
    expect(inputBox).not.toBeNull();
    expect(resultsBox).not.toBeNull();
    expect(resultsBox?.y ?? 0, `${viewport.name} results stay below the field`).toBeGreaterThanOrEqual(
      (inputBox?.y ?? 0) + (inputBox?.height ?? 0),
    );
    expect(registrations).toHaveLength(registrationsBeforeSearch);

    await input.press("ArrowDown");
    await input.press("Enter");
    await expect(page.getByRole("button", { name: "Add collection" })).toBeVisible();
    await expect(page.getByText("8,457 items")).toBeVisible();
    await expect(page.getByText("Popular videos in this collection")).toBeVisible();
    await expect(page.getByRole("link", { name: "1970s station break" })).toBeVisible();
    await expect(page.getByText(/a small sample from this source/i)).toBeVisible();
    await page.getByRole("button", { name: "Preview" }).first().click();
    const preview = page.locator('iframe[title="Preview 1970s station break"]');
    await expect(preview).toHaveAttribute("src", "https://archive.org/embed/station_break_1978?autoplay=1");
    await expect(page.getByRole("link", { name: /open original/i })).toHaveAttribute(
      "href",
      "https://archive.org/details/station_break_1978",
    );
    await page.getByRole("button", { name: "Close" }).click();

    const bodyWidth = await page.locator("body").evaluate((body) => ({
      client: body.clientWidth,
      scroll: body.scrollWidth,
    }));
    expect(bodyWidth.scroll, `${viewport.name} has no horizontal overflow`).toBe(bodyWidth.client);

    const results = await new AxeBuilder({ page }).analyze();
    const blocking = results.violations.filter(
      (violation) => violation.impact === "serious" || violation.impact === "critical",
    );
    expect(blocking, `${viewport.name} axe: ${blocking.map((violation) => violation.id).join(", ")}`).toEqual(
      [],
    );

    if (viewport.name === "desktop") {
      await page.getByRole("button", { name: "Add collection" }).click();
      await expect.poll(() => registrations.length).toBe(1);
      expect(registrations[0]).toEqual({
        kind: "archive",
        uri: "classic_tv_commercials",
        label: "Classic TV Commercials",
      });
    }
  }
});

test("a specific clip keeps its parent source and reconnects to the durable download", async ({ page }) => {
  const backend = await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto("/filler/sources");

  const sourceName = "Classic television commercials from a deliberately long collection name";
  await page.getByRole("button", { name: `Manage ${sourceName}` }).click();
  let workspace = page.getByRole("dialog", { name: sourceName });
  await workspace.getByRole("button", { name: /finding a specific clip/i }).click();
  await workspace.getByRole("textbox", { name: "Search this source" }).fill("station break");
  await workspace.getByRole("button", { name: "Search", exact: true }).click();
  await workspace.getByRole("button", { name: "Queue download: Found clip 1" }).click();

  await expect
    .poll(() => backend.state.fillerSourceItems)
    .toEqual([
      {
        sourceId: "archive:long",
        remoteId: "clip-1",
        url: "https://archive.org/details/clip-1",
      },
    ]);
  await expect(workspace.getByText("Downloading…")).toBeVisible();

  backend.state.fillerAcquisitionStatus = "success";
  await expect(workspace.getByText("Added · being checked")).toBeVisible();

  await page.reload();
  await page.getByRole("button", { name: `Manage ${sourceName}` }).click();
  workspace = page.getByRole("dialog", { name: sourceName });
  await workspace.getByRole("button", { name: /finding a specific clip/i }).click();
  await workspace.getByRole("textbox", { name: "Search this source" }).fill("station break");
  await workspace.getByRole("button", { name: "Search", exact: true }).click();

  await expect(workspace.getByText("Added · being checked")).toBeVisible();
  expect(backend.state.fillerSourceItems).toHaveLength(1);
});

test("YouTube source search hides yt-dlp behind the same search-or-paste flow", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  await page.route("https://www.youtube-nocookie.com/embed/**", (route) =>
    route.fulfill({ status: 200, contentType: "text/html", body: "<title>YouTube preview</title>" }),
  );
  const registrations: unknown[] = [];
  await page.route("**/v1/filler/sources", async (route) => {
    if (route.request().method() === "POST") {
      registrations.push(route.request().postDataJSON());
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "youtube:UC-retro-reels",
          uri: "https://www.youtube.com/channel/UC-retro-reels/videos",
          label: "Retro Reels",
          enabled: true,
        }),
      });
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        sources: [
          {
            id: "provider:youtube",
            kind: "youtube",
            target: "YouTube",
            detail: "channels and playlists you choose",
            count: 0,
            configured: true,
            fetchable: false,
            enabled: true,
            effectiveEnabled: true,
            providerEnabled: true,
            switchable: true,
            removable: false,
            searchable: false,
            group: true,
            readiness: "ready",
            ready: true,
            locationSource: "missing",
            actions: ["configure", "disable"],
          },
        ],
        total: 0,
      }),
    });
  });

  for (const viewport of [
    { name: "desktop", width: 1280, height: 720 },
    { name: "mobile", width: 390, height: 844 },
  ]) {
    const registrationsBeforeSearch = registrations.length;
    await page.setViewportSize(viewport);
    await page.goto("/filler/sources");
    const input = page.getByRole("combobox", { name: "Find a YouTube channel or playlist" });
    await input.fill("retro commercials");
    const option = page.getByRole("option", { name: /Retro Reels/i });
    await expect(option).toBeVisible();
    await expect(page.getByRole("option", { name: /Broadcast Vault/i })).toBeVisible();
    await expect(page.getByText("Channel", { exact: true }).first()).toBeVisible();
    expect(registrations).toHaveLength(registrationsBeforeSearch);

    await input.press("ArrowDown");
    await input.press("Enter");
    await expect(page.getByRole("link", { name: /open on youtube/i })).toHaveAttribute(
      "href",
      "https://www.youtube.com/channel/UC-retro-reels/videos",
    );
    await expect(page.getByRole("button", { name: "Add source", exact: true })).toBeVisible();
    await expect(page.getByText("A few videos from this source")).toBeVisible();
    await expect(page.getByRole("link", { name: "An hour of vintage station breaks" })).toBeVisible();
    await page.getByRole("button", { name: "Preview" }).first().click();
    await expect(page.locator('iframe[title="Preview An hour of vintage station breaks"]')).toHaveAttribute(
      "src",
      "https://www.youtube-nocookie.com/embed/retro-breaks?autoplay=1&playsinline=1",
    );
    await page.getByRole("button", { name: "Close" }).click();

    const widths = await page.locator("body").evaluate((body) => ({
      client: body.clientWidth,
      scroll: body.scrollWidth,
    }));
    expect(widths.scroll, `${viewport.name} has no horizontal overflow`).toBe(widths.client);

    const results = await new AxeBuilder({ page }).analyze();
    const blocking = results.violations.filter(
      (violation) => violation.impact === "serious" || violation.impact === "critical",
    );
    expect(blocking, `${viewport.name} axe: ${blocking.map((violation) => violation.id).join(", ")}`).toEqual(
      [],
    );

    if (viewport.name === "desktop") {
      await page.getByRole("button", { name: "Add source", exact: true }).click();
      await expect.poll(() => registrations.length).toBe(registrationsBeforeSearch + 1);
      expect(registrations.at(-1)).toEqual({
        kind: "youtube",
        uri: "https://www.youtube.com/channel/UC-retro-reels/videos",
        label: "Retro Reels",
      });
    }
  }
});

test("registered sources stay compact and open a scalable source workspace", async ({ page }) => {
  const backend = await installMockBackend(page, { authed: true, role: "admin", fillerEnabled: true });
  await page.route("https://archive.org/embed/**", (route) =>
    route.fulfill({ status: 200, contentType: "text/html", body: "<title>Archive preview</title>" }),
  );
  let archiveProviderEnabled = true;
  const sourceName = "Classic television commercials from a deliberately long collection name";
  const archiveSources = Array.from({ length: 20 }, (_, index) => {
    const attention = index === 12;
    return {
      id: `archive:${index + 1}`,
      uri: `collection_${index + 1}`,
      kind: "archive",
      target: index === 0 ? sourceName : attention ? "Paused collection" : `Archive collection ${index + 1}`,
      detail: "an archive.org collection",
      count: 0,
      configured: true,
      fetchable: true,
      enabled: !attention,
      effectiveEnabled: !attention,
      providerEnabled: true,
      switchable: true,
      removable: true,
      searchable: true,
      parentId: "provider:archive",
      readiness: attention ? "off" : "ready",
      ready: !attention,
      locationSource: "installation",
      effectiveCountry: "US",
      effectiveMarket: "New York City",
      actions: attention ? ["enable"] : ["fetch", "search", "disable", "remove", "edit_location"],
    };
  });
  await page.route("**/v1/filler/sources", async (route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        sources: [
          {
            id: "provider:archive",
            kind: "archive",
            target: "Archive.org",
            detail: "collections you added",
            count: 0,
            configured: true,
            fetchable: false,
            enabled: archiveProviderEnabled,
            effectiveEnabled: archiveProviderEnabled,
            providerEnabled: archiveProviderEnabled,
            switchable: true,
            removable: false,
            searchable: false,
            group: true,
            readiness: archiveProviderEnabled ? "ready" : "off",
            ready: archiveProviderEnabled,
            locationSource: "missing",
            actions: ["configure", "disable"],
          },
          ...archiveSources.map((source) => ({
            ...source,
            effectiveEnabled: archiveProviderEnabled && source.enabled,
            providerEnabled: archiveProviderEnabled,
            readiness: archiveProviderEnabled ? source.readiness : "provider_off",
            ready: archiveProviderEnabled && source.ready,
          })),
        ],
        total: 0,
      }),
    });
  });
  await page.route("**/v1/filler/providers/archive", async (route) => {
    if (route.request().method() !== "PATCH") return route.fallback();
    const body = route.request().postDataJSON() as { enabled: boolean };
    archiveProviderEnabled = body.enabled;
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ kind: "archive", enabled: archiveProviderEnabled }),
    });
  });

  for (const viewport of [
    { name: "desktop", width: 1280, height: 720 },
    { name: "mobile", width: 390, height: 844 },
  ]) {
    await page.setViewportSize(viewport);
    await page.goto("/filler/sources");

    const localSources = page.getByRole("region", { name: "Your files" });
    const provider = page.getByRole("region", { name: "Archive.org" });
    await expect(localSources.getByRole("textbox", { name: "Library name" })).toBeVisible();
    const localBox = await localSources.boundingBox();
    const providerBox = await provider.boundingBox();
    expect(localBox).not.toBeNull();
    expect(providerBox).not.toBeNull();
    expect(localBox?.y ?? 0, `${viewport.name} local sources come before remote providers`).toBeLessThan(
      providerBox?.y ?? 0,
    );
    await expect(provider.getByText("20 sources", { exact: true })).toBeVisible();
    await expect(provider.getByText(/needs attention/i)).toHaveCount(0);
    await expect(provider.getByText("Paused collection")).toBeVisible();
    await expect(provider.getByRole("button", { name: /^Manage / })).toHaveCount(6);
    await provider.getByRole("button", { name: "Show 14 more under Archive.org" }).click();
    await expect(provider.getByRole("button", { name: /^Manage / })).toHaveCount(20);
    await provider.getByRole("button", { name: "Show fewer under Archive.org" }).click();
    await provider.getByRole("searchbox", { name: "Filter Archive.org sources" }).fill("deliberately long");
    await expect(provider.getByRole("button", { name: /^Manage / })).toHaveCount(1);

    const trigger = page.getByRole("button", { name: `Manage ${sourceName}` });
    await expect(trigger).toBeVisible();
    await expect(page.getByRole("button", { name: "Look for new clips" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Browse clips" })).toHaveCount(0);

    await trigger.click();
    const workspace = page.getByRole("dialog", { name: sourceName });
    await expect(workspace).toBeVisible();
    await expect
      .poll(async () => {
        const animatedBox = await workspace.boundingBox();
        return Math.round((animatedBox?.x ?? 0) + (animatedBox?.width ?? 0));
      })
      .toBe(viewport.width);
    const box = await workspace.boundingBox();
    expect(box).not.toBeNull();
    expect(Math.round((box?.x ?? 0) + (box?.width ?? 0))).toBe(viewport.width);
    if (viewport.name === "desktop") {
      expect(box?.width ?? 0).toBeLessThanOrEqual(576);
    } else {
      expect(Math.round(box?.width ?? 0)).toBe(viewport.width);
    }

    await expect(workspace.getByRole("heading", { name: "From this source" })).toBeVisible();
    await expect(workspace.getByRole("link", { name: "1970s station break" })).toBeVisible();
    await workspace.getByRole("button", { name: "Preview" }).first().click();
    await expect(page.locator('iframe[title="Preview 1970s station break"]')).toHaveAttribute(
      "src",
      "https://archive.org/embed/station_break_1978?autoplay=1",
    );
    await page.getByRole("button", { name: "Close" }).click();

    await workspace.getByRole("button", { name: "Look for new clips" }).click();
    await expect(workspace.getByRole("status")).toContainText("2 new clips were queued");
    expect(backend.state.fillerFetches.at(-1)).toBe("archive:1");

    await expect(workspace.getByRole("textbox", { name: "Search this source" })).toHaveCount(0);
    await workspace.getByRole("button", { name: /finding a specific clip/i }).click();
    await workspace.getByRole("textbox", { name: "Search this source" }).fill("breakfast cereal");
    await workspace.getByRole("button", { name: "Search", exact: true }).click();
    await expect(workspace.getByText("Showing 8 of 20 matches")).toBeVisible();
    await workspace.getByRole("button", { name: "Preview: Found clip 1" }).click();
    await expect(page.locator('iframe[title="Preview Found clip 1"]')).toHaveAttribute(
      "src",
      "https://archive.org/embed/clip-1?autoplay=1",
    );
    await page.getByRole("button", { name: "Close" }).click();
    await expect(workspace.getByRole("button", { name: /Queue download:/ })).toHaveCount(8);
    await workspace.getByRole("button", { name: "Show 8 more" }).click();
    await expect(workspace.getByText("Showing 16 of 20 matches")).toBeVisible();
    await expect(workspace.getByRole("button", { name: /Queue download:/ })).toHaveCount(16);

    await workspace.getByRole("button", { name: /source settings/i }).click();
    await expect(workspace.getByRole("combobox", { name: "Location" })).toHaveCount(1);
    await expect(workspace.getByRole("textbox", { name: /country override/i })).toHaveCount(0);
    await expect(workspace.getByRole("textbox", { name: /local area override/i })).toHaveCount(0);

    const results = await new AxeBuilder({ page }).include('[role="dialog"]').analyze();
    const blocking = results.violations.filter(
      (violation) => violation.impact === "serious" || violation.impact === "critical",
    );
    expect(
      blocking,
      `${viewport.name} sheet axe: ${blocking.map((violation) => violation.id).join(", ")}`,
    ).toEqual([]);

    await workspace.getByRole("button", { name: "Close" }).click();
    await expect(workspace).toBeHidden();
    await expect(trigger).toBeFocused();

    await provider.getByRole("searchbox", { name: "Filter Archive.org sources" }).fill("");
    const providerSwitch = provider.getByRole("switch", { name: "Use Archive.org" });
    await providerSwitch.click();
    await expect.poll(() => archiveProviderEnabled).toBe(false);
    await expect(providerSwitch).not.toBeChecked();
    const childList = provider.locator('ul[aria-label="Sources under Archive.org"]');
    await expect(childList).toBeHidden();
    await expect(provider.locator(`input[aria-label="Use ${sourceName}"]`)).toBeDisabled();

    await providerSwitch.click();
    await expect(childList).toBeVisible();
    await expect(provider.getByRole("switch", { name: `Use ${sourceName}` })).toBeChecked();
    await expect(provider.getByRole("switch", { name: "Use Paused collection" })).not.toBeChecked();
  }
});

for (const viewport of [
  { name: "desktop", width: 1280, height: 720 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`source readiness stays truthful through location recovery on ${viewport.name}`, async ({ page }) => {
    const backend = await installMockBackend(page, {
      authed: true,
      role: "admin",
      fillerEnabled: true,
    });
    let failLocationSave = true;

    // The ordinary mock starts in a configured state. This test instead makes the Sources
    // projection follow the persisted Installation location so the browser exercises the real
    // blocked -> failed repair -> ready journey without teaching the UI to infer readiness.
    await page.route("**/v1/filler/watch", async (route) => {
      const locationReady = backend.state.edits["filler.home_country"] === "US";
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          health: locationReady ? "healthy" : "attention",
          sourcesOn: 2,
          sourcesReady: locationReady ? 2 : 1,
          sourcesTotal: 2,
          clips: 3,
          held: 0,
        }),
      });
    });
    await page.route("**/v1/filler/sources", async (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      const locationReady = backend.state.edits["filler.home_country"] === "US";
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          sources: [
            {
              id: "folder",
              uri: "/data/filler",
              kind: "folder",
              target: "/data/filler",
              detail: "watched directly — new files appear on the next pass",
              count: 3,
              incoming: 0,
              configured: true,
              fetchable: true,
              enabled: true,
              effectiveEnabled: true,
              providerEnabled: true,
              switchable: true,
              removable: false,
              searchable: false,
              readiness: "ready",
              ready: true,
              locationSource: "installation",
              actions: ["fetch", "disable"],
            },
            {
              id: "provider:archive",
              kind: "archive",
              target: "Archive.org",
              detail: "collections you added",
              count: 0,
              incoming: 0,
              configured: true,
              fetchable: false,
              enabled: true,
              effectiveEnabled: true,
              providerEnabled: true,
              switchable: true,
              removable: false,
              searchable: false,
              group: true,
              readiness: "ready",
              ready: true,
              locationSource: "missing",
              actions: ["configure", "disable"],
            },
            {
              id: "archive:classic_tv_commercials",
              uri: "classic_tv_commercials",
              kind: "archive",
              target: "Classic TV Commercials",
              detail: "an Archive.org collection",
              count: 0,
              incoming: 0,
              configured: true,
              fetchable: locationReady,
              enabled: true,
              effectiveEnabled: true,
              providerEnabled: true,
              switchable: true,
              removable: true,
              searchable: true,
              parentId: "provider:archive",
              readiness: locationReady ? "ready" : "needs_location",
              ready: locationReady,
              locationSource: locationReady ? "installation" : "missing",
              ...(locationReady ? { effectiveCountry: "US", effectiveMarket: "New York City" } : {}),
              actions: locationReady
                ? ["fetch", "search", "disable", "remove", "edit_location"]
                : ["set_location", "disable", "remove"],
            },
          ],
          total: 3,
        }),
      });
    });
    await page.route("**/v1/settings", async (route) => {
      if (route.request().method() !== "PATCH" || !failLocationSave) return route.fallback();
      return route.fulfill({
        status: 503,
        contentType: "application/problem+json",
        body: JSON.stringify({
          title: "Location wasn't saved",
          detail: "Your previous location is still in use. Try saving again.",
        }),
      });
    });

    await page.setViewportSize(viewport);
    await page.goto("/filler/sources");
    await expect(page.getByText("1 of 2 ready")).toBeVisible();
    await expect(page.getByRole("link", { name: "Set location" })).toBeVisible();
    await expect(page.getByText("Classic TV Commercials")).toBeVisible();
    await sourceReadinessShot(page, `source-readiness-missing-${viewport.name}`);

    await page.getByRole("button", { name: "Manage Classic TV Commercials" }).click();
    const blockedSource = page.getByRole("dialog", { name: "Classic TV Commercials" });
    await expect(blockedSource.getByRole("heading", { name: "Location needed" })).toBeVisible();
    await expect(blockedSource.getByRole("button", { name: "Look for new clips" })).toHaveCount(0);
    await blockedSource.getByRole("button", { name: "Close" }).click();

    await page.getByRole("link", { name: "Set location" }).click();
    await expect(page).toHaveURL(/\/settings\/access$/);
    const location = page.getByRole("combobox", { name: "Location" });
    await location.fill("New York");
    await page.getByRole("option", { name: "New York City, United States" }).click();
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect(page.getByRole("alert")).toContainText("Location wasn't saved");
    await expect(page.getByRole("region", { name: "Unsaved changes" })).toContainText("2 unsaved changes");
    await expect(location).toHaveValue("New York City, United States");
    await sourceReadinessShot(page, `source-readiness-save-failed-${viewport.name}`);

    failLocationSave = false;
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect(page.getByRole("region", { name: "Unsaved changes" })).toHaveCount(0);
    expect(backend.state.edits["filler.home_country"]).toBe("US");
    expect(backend.state.edits["filler.home_market"]).toBe("New York City");

    await page.goto("/filler/sources");
    await expect(page.getByText("2 of 2 ready")).toBeVisible();
    await expect(page.getByRole("link", { name: "Set location" })).toHaveCount(0);
    await sourceReadinessShot(page, `source-readiness-ready-${viewport.name}`);
    await page.getByRole("button", { name: "Manage Classic TV Commercials" }).click();
    const readySource = page.getByRole("dialog", { name: "Classic TV Commercials" });
    await expect(readySource.getByRole("heading", { name: "Ready" })).toBeVisible();
    await expect(readySource.getByRole("button", { name: "Look for new clips" })).toBeVisible();
    await readySource.getByRole("button", { name: "Close" }).click();
  });
}
