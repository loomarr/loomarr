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

// FINDING 1 (see the run notes): on a fresh install `/` redirects to /login, which a new
// operator cannot pass because no account exists yet. The wizard IS public and reachable
// directly — the entry point is what is wrong. Asserted as the CURRENT behavior so the
// walkthrough can continue; the fix flips this expectation.
test("1 · a fresh install reaches the wizard (entry point is broken, see notes)", async ({ page }) => {
  await page.goto("/");
  await expect(page, "known bug: a new operator is sent to a login they cannot pass").toHaveURL(/\/login/);

  await page.goto("/wizard");
  await expect(page.getByLabel(/username/i)).toBeVisible();
});

test("2 · bootstrap creates the owning admin, and only ever once", async ({ page }) => {
  // §11: the first account claims the install, and the endpoint closes for good.
  //
  // There is NO unauthenticated "is this install claimed?" signal to branch on — that
  // absence is FINDING 1 above. So the fresh path is attempted FIRST, through the real
  // form: a POST to /v1/setup/bootstrap cannot be used to probe, because on a genuinely
  // fresh database it would succeed and claim the install with a throwaway account,
  // destroying the first run it was meant to detect.
  await page.goto("/wizard");
  await page.getByLabel(/username/i).fill(ADMIN.username);
  await page.getByLabel(/^password/i).fill(ADMIN.password);
  const confirm = page.getByLabel(/confirm/i);
  if (await confirm.count()) await confirm.fill(ADMIN.password);
  await page.getByRole("button", { name: /create|continue|next/i }).first().click();

  const claimed = await expect
    .poll(async () => (await page.request.get("/v1/auth/me")).status(), { timeout: 30_000 })
    .toBe(200)
    .then(
      () => true,
      () => false,
    );

  if (!claimed) {
    // A re-run against the install a previous run already claimed. Now — and only now,
    // once it is closed — POSTing bootstrap is safe, so assert the security property
    // directly: an endpoint that reopened would be a serious regression.
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
