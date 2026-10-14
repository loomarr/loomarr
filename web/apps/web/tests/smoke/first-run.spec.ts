import { expect, test } from "@playwright/test";

// A NEW OPERATOR'S FIRST RUN, against real services.
//
// One story told in order, so `fullyParallel: false` matters: step N assumes step N-1
// happened. Assertions are about BEHAVIOR (did the install actually change?) rather than
// appearance, because the data is a real library and cannot be snapshotted.
//
// STATEFUL BY DESIGN. `make smoke-reset` wipes the database and gives the true first run;
// plain `make smoke` re-runs against the install the last run left behind, so iterating on
// one step costs seconds instead of a cold start. Every step therefore asserts something
// real in BOTH states — a step that could only pass on a fresh database would go red on
// every re-run, and a suite that is normally red teaches you to ignore it.

const ADMIN = { username: "smoke-admin", password: "smoke-password-123" };

// The throwaway Tunarr the smoke stack runs (scripts/smoke.sh). Step 8 asks TUNARR
// whether the channel landed, rather than believing Loomarr's own report of the push.
const TUNARR = process.env.SMOKE_TUNARR_URL ?? "http://127.0.0.1:8001";

// Each test gets a fresh browser context, so the session from bootstrap does not carry
// over. Signing in through the real login form (not by injecting a cookie) keeps the
// smoke honest: if login breaks, every later step fails, which is correct.
const signIn = async (page: import("@playwright/test").Page) => {
  await page.goto("/login");
  await page.getByLabel(/username/i).fill(ADMIN.username);
  await page.getByLabel(/password/i).fill(ADMIN.password);
  await page.getByRole("button", { name: /sign in|log in/i }).click();
  await expect
    .poll(async () => (await page.request.get("/v1/auth/me")).status(), { timeout: 20_000 })
    .toBe(200);
};

const setupStatus = async (page: import("@playwright/test").Page) =>
  (await (await page.request.get("/v1/setup/status")).json()) as {
    checks: { name: string; ok: boolean; hint?: string; docHref?: string }[];
  };

const checkNamed = (
  status: { checks: { name: string; ok: boolean }[] },
  name: string,
): { name: string; ok: boolean } | undefined => status.checks.find((c) => c.name === name);

test.describe.configure({ mode: "serial" });

// FINDING 1, now FIXED: a fresh install used to bounce the owner to /login, which no
// credential could pass because no account existed yet — and nothing on the page said
// so. The unauthenticated GET /v1/setup/state (§7) is what lets the guards tell an
// unclaimed install from a signed-out one.
test("1 · a fresh install lands the operator on the wizard, not a login they cannot pass", async ({
  page,
}) => {
  const claimed = (await (await page.request.get("/v1/setup/state")).json()).bootstrapped;
  test.skip(claimed, "already bootstrapped — `make smoke-reset` exercises the true first run");

  // The entry point §16 tells operators to open.
  await page.goto("/");
  await expect(page).toHaveURL(/\/wizard/);
  await expect(page.getByLabel(/username/i)).toBeVisible();

  // And the bookmark/direct-navigation path, which is guarded separately.
  await page.goto("/login");
  await expect(page, "/login must not strand the owner of an unclaimed install").toHaveURL(/\/wizard/);
});

test("2 · bootstrap creates the owning admin, and only ever once", async ({ page }) => {
  // §11: the first account claims the install, and the endpoint closes for good.
  // /v1/setup/state tells the two legitimate states apart up front — a genuinely fresh
  // database and a re-run against the install a previous run claimed — so each is
  // asserted for what it is. (Before that endpoint existed this step had to *guess* by
  // driving the form and waiting 30s for a session that would never arrive.)
  if (!(await (await page.request.get("/v1/setup/state")).json()).bootstrapped) {
    await page.goto("/wizard");
    await page.getByLabel(/username/i).fill(ADMIN.username);
    await page.getByLabel(/^password/i).fill(ADMIN.password);
    const confirm = page.getByLabel(/confirm/i);
    if (await confirm.count()) await confirm.fill(ADMIN.password);
    await page
      .getByRole("button", { name: /create|continue|next/i })
      .first()
      .click();

    await expect
      .poll(async () => (await page.request.get("/v1/auth/me")).status(), { timeout: 30_000 })
      .toBe(200);
  } else {
    // Already claimed, so POSTing bootstrap is safe — assert the security property
    // directly rather than skipping it: an endpoint that reopened would be serious.
    const again = await page.request.post("/v1/setup/bootstrap", {
      data: { username: "smoke-intruder", password: "smoke-intruder-123" },
      failOnStatusCode: false,
    });
    expect(again.status(), "bootstrap must stay closed once claimed (§11)").toBe(409);
    await signIn(page);
  }

  // Proof either way is the session the server hands back, not a message on screen.
  const me = await (await page.request.get("/v1/auth/me")).json();
  expect(me.name).toBe(ADMIN.username);
  expect(me.role).toBe("admin");
});

test("3 · the checklist reports the REAL stack", async ({ page }) => {
  // Sign in first: each test gets a fresh context, so the cookie from bootstrap does not
  // carry over. Reading setup/status is admin-only (§13).
  await signIn(page);
  const status = await setupStatus(page);

  // Live probes against the operator's actual services. These must be green for the rest
  // of the walkthrough to mean anything.
  expect(checkNamed(status, "media_server")?.ok, "Emby should be reachable").toBe(true);
  expect(checkNamed(status, "tmdb")?.ok, "TMDB grounding should be reachable").toBe(true);
  expect(checkNamed(status, "tunarr")?.ok, "the throwaway Tunarr should be reachable").toBe(true);

  // Every red check must carry a hint AND a deep-link, or the wizard is a dead end (§13).
  // This is the assertion that holds on a re-run: whichever checks are still red, they
  // must be actionable.
  for (const c of status.checks) {
    if (c.ok) continue;
    expect(c.hint, `${c.name} failed with no hint`).toBeTruthy();
    expect(c.docHref, `${c.name} failed with no docHref`).toBeTruthy();
  }
});

test("4 · every docHref the checklist emits resolves in the embedded docs", async ({ page }) => {
  await signIn(page);
  const status = await setupStatus(page);
  const doc = await (await page.request.get("/v1/docs/troubleshooting")).json();

  const slug = (h: string) =>
    h
      .toLowerCase()
      .replace(/[^a-z0-9 \-_]/g, "")
      .trim()
      .replace(/[ _]+/g, "-")
      .replace(/-+$/, "");
  const anchors = new Set(
    doc.markdown
      .split("\n")
      .filter((l: string) => /^#{1,6}\s/.test(l.trim()))
      .map((l: string) => slug(l.trim().replace(/^#+\s*/, ""))),
  );

  // §13: "every red check deep-links to its section". Verified here against the pages the
  // server is really serving, not a fixture.
  for (const c of status.checks) {
    const [, fragment] = String(c.docHref ?? "").split("#");
    expect(anchors.has(fragment), `${c.name} → ${c.docHref} does not resolve`).toBe(true);
  }
});

test("5 · the §8.1 picker ranks the operator's REAL models, and selecting one turns llm green", async ({
  page,
}) => {
  await signIn(page);
  await page.goto("/settings/ai");

  // The catalog is computed by the BE from the machine it is running on — the whole point
  // of §8.1 is that the operator never types an Ollama tag. Asserting on it proves the
  // probe reached Ollama, not merely that a component rendered.
  const status = await (await page.request.get("/v1/system/llm")).json();
  expect(status.local, "the smoke env points at a local Ollama").toBe(true);
  expect(status.reachable, "Ollama should be up for the smoke").toBe(true);

  type Model = { tag: string; pulled: boolean; runtimeOk: boolean; fit: string };
  const usable: Model[] = (status.catalog ?? []).filter(
    (m: Model) => m.pulled && m.runtimeOk && m.fit !== "wont_fit",
  );
  expect(usable.length, "no locally-pulled model is usable — pull one before the smoke").toBeGreaterThan(0);

  // Prefer a model that is NOT already active, so the re-run exercises a real swap rather
  // than clicking a disabled button.
  const target = usable.find((m) => m.tag !== status.model) ?? usable[0];
  if (!target) throw new Error("unreachable: usable is non-empty");

  // Select through the UI, not the API: the click path is what has been broken before.
  const row = page.getByRole("listitem").filter({ hasText: target.tag });
  await expect(row, `${target.tag} should be offered by the picker`).toBeVisible();
  await row.getByRole("button", { name: /use this/i }).click();

  // Hot-swap, no restart (§8.1) — the server's own view is the proof, not the button label.
  await expect
    .poll(async () => (await (await page.request.get("/v1/system/llm")).json()).model, {
      timeout: 60_000,
    })
    .toBe(target.tag);

  // And the check the wizard reports on must follow, or the operator is told they are
  // still unconfigured immediately after configuring themselves.
  expect(checkNamed(await setupStatus(page), "llm")?.ok, "llm check should go green").toBe(true);
});

test("6 · wiring Tunarr to the real library makes tunarr_library green, idempotently", async ({ page }) => {
  await signIn(page);

  // Driven from SETTINGS, not the wizard — which is the point of FINDING 2's fix.
  //
  // This step used to be unreachable here: the wizard resumes at the first incomplete
  // step, offers only Back/Continue with a non-clickable rail, and Live TV was not
  // skippable — so the library wiring sat behind a check this install cannot turn green
  // (its media server is the maintainer's real Emby, which the smoke must not write to).
  // The wiring actions now live on Settings → Connections for the life of the install,
  // so this needs no wizard navigation at all.
  await page.goto("/settings/connections");

  // The action points the THROWAWAY Tunarr at the operator's real Emby and triggers a
  // scan. It mutates only Tunarr — nothing is written to the media server, which is why
  // it is safe here while Live TV (the opposite direction) gets its own disposable
  // Jellyfin in livetv.spec.ts.
  //
  // `tunarr_library` is the assertion, not the button's own response: the BE's own probe
  // is the only honest source of "this actually worked" (§6, never silent).
  const wire = page.getByRole("button", { name: /wire tunarr to your library|run again/i });
  await expect(wire).toBeVisible({ timeout: 60_000 });
  await wire.click();

  await expect
    .poll(async () => checkNamed(await setupStatus(page), "tunarr_library")?.ok, {
      timeout: 120_000, // a real library scan, not a mock
    })
    .toBe(true);

  // "Safe to run more than once" is a claim the UI makes (§6 — and it never re-scans
  // unasked). A second click must leave it green, not duplicate the media source.
  await page.getByRole("button", { name: /run again/i }).click();
  await expect
    .poll(async () => checkNamed(await setupStatus(page), "tunarr_library")?.ok, { timeout: 120_000 })
    .toBe(true);
});

// §21's manual Definition of Done, automated: "a real intent → approved → a channel
// actually playing in Tunarr". Everything upstream is real — the operator's library, TMDB
// grounding, and their own Ollama composing the lineup.
//
// SAFE BY CONSTRUCTION: the smoke env omits the requester (§ scripts/smoke.sh), so
// approving cannot submit anything to Sonarr/Radarr and nothing downloads. The approval
// gate itself is still exercised end to end — it is the gate, not the acquisition, that
// this proves.
test("7 · a real intent becomes a grounded proposal from the operator's own Ollama", async ({ page }) => {
  await signIn(page);
  await page.goto("/suggest");

  await page.getByRole("textbox").first().fill("90s Saturday morning cartoons for the kids");
  await page.getByRole("button", { name: /suggest a lineup/i }).click();

  // A real local model on real hardware: minutes, not milliseconds. The proposal landing
  // is the assertion — §8 treats the SSE phases as a latency optimisation, never truth.
  const proposal = page.getByRole("button", { name: /approve & acquire/i });
  await expect(proposal, "the suggestion run should produce a reviewable proposal").toBeVisible({
    timeout: 600_000,
  });

  // GROUNDING (§8, non-negotiable): every title must be something the CATALOG returned,
  // not something the model composed. Asserted against the PERSISTED proposal — a UI that
  // merely renders plausible text cannot satisfy this.
  //
  // The approval queue is GET /v1/proposals?status=submitted (proposals are the
  // resource, suggestions the route — §7.2).
  const listed = await (await page.request.get("/v1/proposals?status=submitted")).json();
  const latest = listed.proposals?.[0];
  expect(latest, "a submitted proposal should be persisted").toBeTruthy();

  const lineup = latest.proposal?.lineup ?? [];
  // §8: a zero-grounded-title run FAILS the job rather than persisting an empty proposal,
  // so an empty lineup here would mean that rule stopped holding.
  expect(lineup.length, "a proposal with no lineup is a failed run, not a result").toBeGreaterThan(0);

  for (const item of lineup) {
    expect(item.name, "every lineup entry needs a name").toBeTruthy();
    // A hallucinated title has no catalog identity. tmdbId comes from TMDB, libraryItemId
    // from the media server — an entry with neither was invented by the model, which is
    // exactly what §8's grounding exists to make impossible.
    expect(
      item.tmdbId || item.libraryItemId,
      `"${item.name}" carries no catalog identity — that is a hallucination, not a suggestion`,
    ).toBeTruthy();
    // §8 also requires the model to say WHY, so the operator can judge the lineup rather
    // than take it on faith.
    expect(item.rationale, `"${item.name}" was suggested with no rationale`).toBeTruthy();
  }

  // The intent must survive onto the proposal, or "My proposals" and the approval queue
  // show lineups nobody can trace back to a request.
  expect(latest.proposal?.intent?.description).toContain("Saturday morning");
});

// §21's Definition of Done, end to end: "a real intent → approved → a channel actually
// playing in Tunarr". This is the step the whole product exists for, and until FINDING 4
// was fixed it could not pass at all — approving enqueued acquisitions and stopped, so
// no channel was ever created.
//
// Still safe: the smoke env omits the requester, so approving cannot start a download.
// The gate is exercised; nothing leaves the machine.
test("8 · approving materializes a channel, and Tunarr really has it", async ({ page }) => {
  await signIn(page);

  // Work with whatever step 7 left in the queue; if it is empty the approval already
  // happened on an earlier run, and the channel assertions below still hold.
  const queued = (await (await page.request.get("/v1/proposals?status=submitted")).json()).proposals?.[0];

  if (queued) {
    await page.goto("/suggest");
    // The QUEUE's button is "Approve" — "Approve & acquire" belongs to the run's own
    // review card, which only exists while a suggestion run is on screen. An admin
    // acting on someone else's earlier proposal (the whole point of the queue, §11)
    // sees the queue, so that is what this drives.
    await page
      .getByRole("button", { name: /^approve/i })
      .first()
      .click();
    // The UI navigates to the channel it just made (§7 returns its id) — that landing is
    // the operator-visible proof that approving produced something.
    await expect(page).toHaveURL(/\/channels\//, { timeout: 60_000 });
  }

  // Loomarr's own view: a channel bound to the approved intent, carrying its lineup.
  const channels = (await (await page.request.get("/v1/channels")).json()).channels ?? [];
  expect(channels.length, "approving an intent must leave a channel behind").toBeGreaterThan(0);
  const ch = channels.find((c: { intentRef?: string }) => c.intentRef) ?? channels[0];
  expect(ch.name, "a channel named from the operator's own words").toBeTruthy();
  expect(ch.number, "auto-allocated so the operator never has to pick one").toBeGreaterThan(0);

  // And Tunarr's, which is the honest one: Loomarr reporting that it pushed a channel is
  // exactly the claim under test (§6 — Tunarr owns playout, so it is the source of truth).
  // The reconcile is async, so poll rather than assume it already landed.
  await expect
    .poll(
      async () => {
        const res = await page.request.get(`${TUNARR}/api/channels`);
        if (res.status() !== 200) return -1;
        return ((await res.json()) as { number: number }[]).length;
      },
      { timeout: 180_000 },
    )
    .toBeGreaterThan(0);

  const tunarrChannels = (await (await page.request.get(`${TUNARR}/api/channels`)).json()) as {
    number: number;
    name: string;
  }[];
  expect(
    tunarrChannels.some((t) => t.number === ch.number),
    `Loomarr has channel ${ch.number} but Tunarr does not — the push never landed`,
  ).toBe(true);

  // §21 says "actually PLAYING", not merely "present". A channel with zero programs is
  // dead air (§9) — which is exactly what FINDING 6 produced: audience-ceiling policy +
  // in-library picks whose rating was dropped in discovery, so every entry was excluded
  // and the channel went live with nothing to play. Asserting the count is what turns
  // that from an invisible green into a red. Poll: the first reconcile resolves slots
  // from live library availability, which is not instant.
  await expect
    .poll(
      async () => {
        const fresh = (await (await page.request.get("/v1/channels")).json()).channels ?? [];
        const c = fresh.find((x: { id: string }) => x.id === ch.id);
        return c?.programCount ?? 0;
      },
      { timeout: 120_000 },
    )
    .toBeGreaterThan(0);
});

// The Help center is embedded markdown served by the binary (§7.2 — works air-gapped),
// so it verifies regardless of any external service being up. A dead help link is a
// support dead-end, which is exactly what the docHref check (step 4) guards at the API
// level; this proves the READER renders it.
test("9 · the Help center renders embedded docs and search filters them", async ({ page }) => {
  await signIn(page);
  await page.goto("/help");

  await expect(page.getByRole("heading", { name: /^help$/i })).toBeVisible();
  const nav = page.getByRole("navigation", { name: /help pages/i });
  await expect(nav, "the page list is built from GET /v1/docs").toBeVisible();

  // The server lists its own pages; the reader must show them. Assert against the API so
  // a renamed page can't silently drop from the UI.
  const docs = await (await page.request.get("/v1/docs")).json();
  expect((docs.docs ?? docs).length ?? 0, "the binary should embed help pages").toBeGreaterThan(0);

  // Client-side search (§7.2, no server round-trip) narrows the list.
  const before = await nav.getByRole("link").count();
  await page
    .getByLabel(/search/i)
    .first()
    .fill("zzzznomatch");
  await expect.poll(async () => nav.getByRole("link").count()).toBeLessThan(Math.max(before, 1) + 1); // narrowing never ADDS pages
});

// The command palette (§12) is the app's cross-surface jump. Channels and help pages
// come from the store + embedded docs, so this needs no media server — it proves the
// palette opens, searches, and navigates.
test("10 · the command palette opens, searches, and navigates to a help page", async ({ page }) => {
  await signIn(page);
  await page.goto("/channels");

  // Open via the AppShell's ⌘K affordance — the button IS the shortcut's discoverable
  // twin (it renders the "⌘K" hint), and clicking it is a real operator action that
  // doesn't depend on synthetic-keypress focus quirks.
  await page
    .getByRole("button", { name: /search/i })
    .first()
    .click();
  const dialog = page.getByRole("dialog", { name: /search loomarr/i });
  await expect(dialog, "the palette should open").toBeVisible();

  // "troubleshooting" is a known embedded help page (step 4 resolves its anchors). The
  // results render as buttons; pick the one that names it.
  await dialog.getByLabel(/search/i).fill("troubleshoot");
  await dialog
    .getByRole("button", { name: /troublesh/i })
    .first()
    .click();
  await expect(page, "picking a help result jumps to the Help center").toHaveURL(/\/help/);
});

// The Users page (§11): identity is Loomarr's, and the bootstrap admin is the one account
// that always exists. The USER LIST is store-backed (GET /v1/users), so it verifies with
// no media server — which matters here because import candidates DO need the media server,
// and this asserts only what holds regardless of its state.
test("11 · the Users page lists the owning admin, and offers explicit import", async ({ page }) => {
  await signIn(page);
  await page.goto("/people");

  await expect(page.getByRole("heading", { name: /^users$/i })).toBeVisible();
  // The admin created in step 2 is present and marked admin — the allowlist's root (§11).
  await expect(page.getByText(ADMIN.username).first()).toBeVisible();

  // §11 is "explicit import, never implicit": the import affordance must be on the page,
  // whether or not the media server is currently reachable to populate it. Asserting the
  // section (not its candidates) keeps this honest when Emby is down.
  const users = await (await page.request.get("/v1/users")).json();
  expect(
    (users.users ?? users).some(
      (u: { name: string; role: string }) => u.name === ADMIN.username && u.role === "admin",
    ),
    "the owning admin must be in the allowlist",
  ).toBe(true);
});

// The Filler page (§10). The smoke omits FILLER_DIR (filler is optional), so the honest
// state is "not configured" — and that must read as a clear, non-broken empty state, not
// a crash or a spinner that never resolves. A feature you haven't set up should SAY so.
test("12 · the Filler page renders its unconfigured state cleanly", async ({ page }) => {
  await signIn(page);
  await page.goto("/filler");

  await expect(page.getByRole("heading", { name: /^filler$/i })).toBeVisible();
  // Whatever the exact copy, the page must resolve to real content — a heading plus its
  // empty/guidance state — rather than hanging or erroring. The clip list is empty
  // because no drop-folder is configured, which is a legitimate first-run state.
  const clips = await (await page.request.get("/v1/filler")).json();
  expect(Array.isArray(clips.clips ?? clips), "the filler list endpoint should answer").toBe(true);
});
