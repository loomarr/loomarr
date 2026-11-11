import { appendFileSync } from "node:fs";
import type { Page } from "@playwright/test";
import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

// Route timing, in a real browser (#3399, item 9.1 of the codebase-quality program). It drives the
// shipping SPA through the main nav and reads the browser's own User Timing entries, written by
// src/lib/route-timing:
//
//   loomarr:route:request-to-rendered   the router committed the page and it painted
//   loomarr:route:request-to-content    the page's real data is painted (no loader in its place)
//
// The clock starts at the click, in the page, so Playwright's own latency never counts.
//
// COLD is a page asked for the first time in the session. WARM is the same page asked for again
// moments later, with its data still in the query cache (inside its 15 s staleTime).
//
// THE BACKEND IS FIXED, not fast. Every `/v1` read answers after API_DELAY_MS, whatever the
// runner's speed, so a number here is the app's own work plus a known number of round trips: a
// page that waits on two reads in a row costs twice the delay, and one that paints from its cache
// costs none. Change the delay and every budget below moves with it.
//
// BUDGETS ARE A RATCHET, not the goal. The goal is warm <= 100 ms and cold <= 300 ms to real
// content on every route. Loaders, SSE-driven freshness and the persisted cache are landing in
// parallel PRs, so the budgets below sit just above what main measures today and each of those PRs
// lowers the ones it earns. A budget only ever goes down; raise one only with the reason in the
// commit. Today's numbers (CI, macOS runner, median of RUNS, API_DELAY_MS = 100):
//
//   route      cold   warm
//   (filled in from the first CI run)
//
// The assertion is on the median of RUNS fresh sessions, so one slow frame on a shared runner does
// not fail the gate. Every run is printed (and appended to the job summary) so a failure shows
// the spread, not only the median.

const API_DELAY_MS = 100;
const RUNS = 3;

interface Destination {
  name: string;
  // The primary-nav link that opens it. Channels has no nav entry (the Guide opens a channel from
  // a row), so it is entered as the app's own router does for a link: a history push.
  link?: string;
  // The pathname the page settles on, or the prefix of it: Requests redirects to its landing tab.
  path: string;
}

const DESTINATIONS: Destination[] = [
  { name: "Home", link: "Home", path: "/dashboard" },
  { name: "Guide", link: "Guide", path: "/guide" },
  { name: "Library", link: "Library", path: "/library" },
  { name: "Requests", link: "Requests", path: "/requests" },
  { name: "Settings", link: "Settings", path: "/settings" },
  { name: "Channels", path: "/channels/ch-1" },
];

type Pass = "cold" | "warm";

// Milliseconds from the click to real content, per route. Deliberately loose until the first CI
// run measures the real numbers; see the ratchet note above.
const BUDGET_MS: Record<Pass, Record<string, number>> = {
  cold: { Home: 2000, Guide: 2000, Library: 2000, Requests: 2000, Settings: 2000, Channels: 2000 },
  warm: { Home: 1000, Guide: 1000, Library: 1000, Requests: 1000, Settings: 1000, Channels: 1000 },
};

interface Visit {
  to: string;
  revisit: boolean;
  rendered?: number;
  content?: number;
}

const readVisits = (page: Page): Promise<Visit[]> =>
  page.evaluate(() => {
    const visits = new Map<number, Visit>();
    for (const phase of ["rendered", "content"] as const) {
      for (const entry of performance.getEntriesByName(`loomarr:route:request-to-${phase}`)) {
        const detail = (entry as PerformanceMeasure).detail as {
          visitId: number;
          to: string;
          revisit: boolean;
        };
        const visit = visits.get(detail.visitId) ?? { to: detail.to, revisit: detail.revisit };
        visit[phase] = entry.duration;
        visits.set(detail.visitId, visit);
      }
    }
    return [...visits.values()];
  });

const reached = (visits: Visit[], destination: Destination): Visit[] =>
  visits.filter(
    (visit) =>
      visit.content !== undefined &&
      (visit.to === destination.path || visit.to.startsWith(`${destination.path}/`)),
  );

// Asks for the page and waits for its content mark, returning that visit.
const visit = async (page: Page, destination: Destination): Promise<Visit> => {
  const before = reached(await readVisits(page), destination).length;
  if (destination.link) {
    await page
      .getByRole("navigation", { name: "Primary" })
      .getByRole("link", { name: destination.link, exact: true })
      .click();
  } else {
    await page.evaluate((path) => {
      history.pushState({}, "", `${path}/info`);
      window.dispatchEvent(new PopStateEvent("popstate"));
    }, destination.path);
  }
  await expect
    .poll(async () => reached(await readVisits(page), destination).length, {
      message: `${destination.name} never reported its content`,
      timeout: 15_000,
    })
    .toBe(before + 1);
  const last = reached(await readVisits(page), destination).at(-1);
  if (!last) throw new Error(`${destination.name} reported no visit`);
  return last;
};

const median = (values: number[]): number => {
  const ordered = [...values].sort((a, b) => a - b);
  return ordered[Math.floor(ordered.length / 2)] ?? Number.POSITIVE_INFINITY;
};

const samples: Record<Pass, Record<string, number[]>> = { cold: {}, warm: {} };
const renderedSamples: Record<Pass, Record<string, number[]>> = { cold: {}, warm: {} };

const record = (
  store: Record<Pass, Record<string, number[]>>,
  pass: Pass,
  name: string,
  ms: number | undefined,
) => {
  const list = store[pass][name] ?? [];
  list.push(ms ?? Number.NaN);
  store[pass][name] = list;
};

test.describe.configure({ mode: "serial" });
test.setTimeout(120_000);

for (let run = 1; run <= RUNS; run++) {
  test(`main nav, fresh session ${run} of ${RUNS}`, async ({ page }) => {
    await installMockBackend(page, {
      authed: true,
      role: "admin",
      guideChannels: 12,
      requests: true,
      channelDetail: true,
    });
    // Registered after the mock, so it runs first: hold every read for the fixed delay, then hand
    // it to the mock. The event stream is not a read and stays as the mock leaves it.
    await page.route("**/v1/**", async (route) => {
      if (!route.request().url().includes("/v1/events")) {
        await new Promise((resolve) => setTimeout(resolve, API_DELAY_MS));
      }
      await route.fallback();
    });

    // Open on Help, which has no data and no content mark, so every measured page is reached by
    // navigation from inside the app and Home is cold like the rest.
    await page.goto("/help");
    await expect(page.getByRole("heading", { level: 1, name: "Help", exact: true })).toBeVisible();

    for (const pass of ["cold", "warm"] as const) {
      for (const destination of DESTINATIONS) {
        const result = await visit(page, destination);
        expect(result.revisit, `${destination.name} ${pass}: was it shown before?`).toBe(pass === "warm");
        record(samples, pass, destination.name, result.content);
        record(renderedSamples, pass, destination.name, result.rendered);
      }
    }
  });
}

test("route content meets the budgets", () => {
  const rows = [
    "| route | pass | content median | content runs | rendered median | budget |",
    "| --- | --- | --- | --- | --- | --- |",
  ];
  const failures: string[] = [];
  for (const pass of ["cold", "warm"] as const) {
    for (const { name } of DESTINATIONS) {
      const content = samples[pass][name] ?? [];
      const rendered = renderedSamples[pass][name] ?? [];
      const budget = BUDGET_MS[pass][name] ?? Number.POSITIVE_INFINITY;
      const mid = median(content);
      rows.push(
        `| ${name} | ${pass} | ${mid.toFixed(0)} ms | ${content.map((ms) => ms.toFixed(0)).join(", ")} | ${median(rendered).toFixed(0)} ms | ${budget} ms |`,
      );
      if (!(mid <= budget))
        failures.push(`${name} ${pass}: ${mid.toFixed(0)} ms is over its ${budget} ms budget`);
    }
  }
  const table = `Route timing (API delay ${API_DELAY_MS} ms, median of ${RUNS} fresh sessions)\n\n${rows.join("\n")}\n`;
  console.log(table);
  if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, `\n${table}`);
  expect(failures, failures.join("\n")).toEqual([]);
});
