import { expect, test } from "@playwright/test";

// THE LIVE TV WIRING, against a DISPOSABLE media server.
//
// Run by `make smoke-livetv`, which stands up its own Jellyfin, points a separate
// loomarr instance at it, and destroys the container afterwards. That teardown IS the
// revert: POST /v1/setup/livetv-connect writes to the media server (registering Tunarr
// as an M3U tuner + XMLTV guide source) and there is no product code path to undo it —
// internal/library/livetv.go only ever adds. Against a real media server that would
// leave a tuner pointing at a Tunarr which no longer exists.
//
// The assertions read JELLYFIN's own API, not Loomarr's. Loomarr reporting that it wired
// something is precisely the claim under test; §6's rule is that the check, never the
// click, is what says it worked — and the most honest check of all is the other system.

const JELLYFIN = process.env.SMOKE_JELLYFIN_URL ?? "";
const TOKEN = process.env.SMOKE_JELLYFIN_TOKEN ?? "";

const ADMIN = { username: "smoke-admin", password: "smoke-password-123" };

// This instance has its own database, so it needs its own owning admin.
const bootstrapAndSignIn = async (page: import("@playwright/test").Page) => {
  if (!(await (await page.request.get("/v1/setup/state")).json()).bootstrapped) {
    await page.request.post("/v1/setup/bootstrap", { data: ADMIN, failOnStatusCode: false });
  }
  await page.goto("/login");
  await page.getByLabel(/username/i).fill(ADMIN.username);
  await page.getByLabel(/password/i).fill(ADMIN.password);
  await page.getByRole("button", { name: /sign in|log in/i }).click();
  await expect
    .poll(async () => (await page.request.get("/v1/auth/me")).status(), { timeout: 20_000 })
    .toBe(200);
};

// Read Jellyfin's Live TV config directly. Deliberately NOT /LiveTv/TunerHosts: those
// lineage endpoints are write-only on Jellyfin and answer 405 — which is the whole bug
// this spec exists to guard, and which the first draft of this helper reproduced.
const liveTVConfig = async (page: import("@playwright/test").Page) => {
  const res = await page.request.get(`${JELLYFIN}/System/Configuration/livetv`, {
    headers: { Authorization: `MediaBrowser Token="${TOKEN}"` },
  });
  expect(res.status(), "Jellyfin should serve its Live TV config").toBe(200);
  return (await res.json()) as {
    TunerHosts: { Id: string; Type: string; Url: string }[];
    ListingProviders: { Id: string; Type: string; Path: string }[];
  };
};

test.describe.configure({ mode: "serial" });

test.beforeEach(() => {
  test.skip(!JELLYFIN || !TOKEN, "run via `make smoke-livetv`, which provisions the Jellyfin");
});

test("livetv · wiring registers Tunarr as a tuner AND a guide source in the media server", async ({
  page,
}) => {
  await bootstrapAndSignIn(page);

  // Nothing should be wired yet — otherwise the assertions below would pass on a
  // pre-existing tuner and prove nothing.
  expect((await liveTVConfig(page)).TunerHosts.length).toBe(0);

  await page.goto("/settings/connections");
  await page.getByRole("button", { name: /connect tunarr to the guide/i }).click();

  // Loomarr's own view first…
  await expect
    .poll(
      async () => {
        const status = await (await page.request.get("/v1/setup/status")).json();
        return status.checks.find((c: { name: string }) => c.name === "livetv")?.ok;
      },
      { timeout: 120_000 },
    )
    .toBe(true);

  // …then the media server's, which is the one that actually matters. §6 wires BOTH a
  // tuner and a listing provider; registering only the tuner would give channels with
  // no guide data, which looks fine in Loomarr and broken on the TV.
  const cfg = await liveTVConfig(page);
  expect(cfg.TunerHosts.length, "Tunarr should be registered as a tuner").toBeGreaterThan(0);
  expect(
    String(cfg.TunerHosts[0]?.Type ?? "").toLowerCase(),
    "M3U is preferred over HDHomeRun emulation (§6)",
  ).toBe("m3u");
  expect(
    cfg.ListingProviders.length,
    "Tunarr's XMLTV guide should be registered — a tuner with no guide looks fine in " +
      "Loomarr and shows an empty EPG on the TV",
  ).toBeGreaterThan(0);
});

test("livetv · running it again is idempotent, not a second tuner", async ({ page }) => {
  await bootstrapAndSignIn(page);

  const before = (await liveTVConfig(page)).TunerHosts.length;
  expect(before, "the previous test should have wired one").toBeGreaterThan(0);

  await page.goto("/settings/connections");
  await page
    .getByRole("button", { name: /run again/i })
    .first()
    .click();

  // §6 claims these actions are "safe to run more than once", and this is the failure
  // that claim is about: a duplicate tuner makes the media server poll Tunarr twice and
  // show every channel twice in the guide. It is also precisely what the Jellyfin bug
  // caused — the enumerate-first check 405'd, so nothing could ever be found already
  // registered, and each run added another.
  await expect
    .poll(async () => (await liveTVConfig(page)).TunerHosts.length, { timeout: 60_000 })
    .toBe(before);
  expect((await liveTVConfig(page)).ListingProviders.length).toBe(1);
});
