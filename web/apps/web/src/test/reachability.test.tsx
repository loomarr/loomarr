import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

// REACHABILITY — the gate this phase earned.
//
// Seven times in 13.4 something was built, unit-tested, and unreachable: two settings
// panels never mounted; a formatter never called, so "·til 8:00 PM" was dead UI on every
// channel card; a 323-line settings form rendered by nothing; a clip's tag action gated
// so the one clip that needed correcting couldn't be; a search scope that always returned
// empty; a Search button wired to a discarded setState. Every component test passed in
// every case, because a component test cannot see whether anything mounts it.
//
// So this asserts REACHABILITY rather than behavior: every route in the generated tree
// renders real content, and every feature-gated panel appears when its flag is on. The
// route list is derived from the router itself — a hand-maintained list is the same
// mistake one level up (see structure.test.ts, which learned this the hard way).

// `local: true` mirrors the meBody field (§11 credential path) — the Account screen
// offers a password form only for a Loomarr-stored credential, so a fixture without it
// would silently exercise the media-server branch instead.
const ADMIN = {
  id: "u1",
  name: "Ada",
  role: "admin",
  autoApprove: true,
  disabled: false,
  quota: 0,
  local: true,
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// Every feature on, so gated panels are expected to appear rather than explain themselves.
const FEATURES = {
  filler: true,
  suggestions: true,
  acquisition: true,
  user_sync: true,
  ingest: true,
};

const stubFetch = () => {
  const mock = vi.fn((url: string) => {
    const u = String(url);
    if (u.includes("/v1/auth/me")) return Promise.resolve(json(ADMIN));
    if (u.includes("/v1/setup/status")) return Promise.resolve(json({ checks: [] }));
    if (u.includes("/v1/settings/secrets")) return Promise.resolve(json({ value: "" }));
    if (u.includes("/v1/settings")) {
      return Promise.resolve(
        json({
          features: FEATURES,
          settings: [
            {
              key: "library.url",
              group: "connections.media_server",
              kind: "url",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "http://emby:8096",
            },
            {
              key: "filler.dir",
              group: "filler",
              kind: "string",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "/filler",
            },
            {
              key: "job.workers",
              group: "advanced",
              kind: "int",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "2",
            },
            {
              key: "channel.reconcile_every",
              group: "channels",
              kind: "duration",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "5m",
            },
            {
              key: "session.ttl",
              group: "users_security",
              kind: "duration",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "720h",
            },
            {
              key: "llm.url",
              group: "ai",
              kind: "url",
              doc: "help",
              advanced: false,
              secret: false,
              set: true,
              provenance: "db",
              value: "http://ollama:11434",
            },
          ],
        }),
      );
    }
    if (u.includes("/v1/system/llm")) {
      return Promise.resolve(
        json({
          local: true,
          reachable: true,
          provider: "ollama",
          model: "qwen3:8b",
          catalog: [],
          hosted: [],
        }),
      );
    }
    if (u.includes("/v1/system/version")) {
      return Promise.resolve(json({ version: "dev", ready: true }));
    }
    if (u.includes("/v1/users/candidates")) return Promise.resolve(json({ candidates: [] }));
    if (u.includes("/sessions")) return Promise.resolve(json({ sessions: [] }));
    if (u.includes("/v1/users")) return Promise.resolve(json({ users: [{ ...ADMIN, local: true }] }));
    if (u.includes("/v1/docs/")) {
      return Promise.resolve(
        json({ slug: "troubleshooting", title: "Troubleshooting", markdown: "# Troubleshooting\n\nHere." }),
      );
    }
    if (u.includes("/v1/docs"))
      return Promise.resolve(json({ docs: [{ slug: "troubleshooting", title: "Troubleshooting" }] }));
    // Before the /v1/filler catalog match below: this path contains "/filler" but is the
    // per-CHANNEL coverage route, and a clips payload would not satisfy the meter.
    if (u.includes("/filler/discover")) {
      return Promise.resolve(
        json({ items: [], total: 0, licenceNote: "Licence information isn't available." }),
      );
    }
    if (u.includes("/filler/coverage")) {
      return Promise.resolve(json({ level: "exact", total: 4, rungs: [{ level: "exact", clips: 4 }] }));
    }
    // Before the /v1/filler catalog match: the Sources tab's rows.
    //
    // ⚠ Without this branch the request fell through to the clips payload below, so the tab
    // rendered ZERO source rows — and the old "Find clips" assertion still passed, because
    // that heading sat OUTSIDE the list. V35b moved the search inside the row that owns it,
    // which is what surfaced the gap. A stub that answers the wrong shape is indistinguishable
    // from a working page until something depends on the shape.
    if (u.includes("/v1/filler/sources")) {
      return Promise.resolve(
        json({
          // ⚠ V37: a flat list. `remote` was the CONTAINER row and is retired — a registered
          // archive collection is a peer carrying `searchable`, which is what the search
          // expander now keys on (the page no longer tests the kind itself).
          sources: [
            {
              id: "archive:classic_tv_commercials",
              enabled: true,
              switchable: true,
              removable: true,
              kind: "archive",
              target: "Classic TV Commercials",
              detail: "an archive.org collection — searchable here",
              count: 0,
              configured: true,
              fetchable: true,
              searchable: true,
            },
          ],
          total: 1,
        }),
      );
    }
    // Before the /v1/filler catalog match: the Incoming queue (§10 V35/V38).
    //
    // ⚠ Same gap as the sources one above — without this branch the request fell through to the
    // clips payload, so the tab rendered whatever `asks: undefined` produces. That is the second
    // time this stub's catch-all has quietly answered the wrong shape; a route added to the app
    // needs a branch here, or its assertion tests the fallback rather than the feature.
    if (u.includes("/v1/filler/incoming")) {
      return Promise.resolve(
        json({
          asks: [
            {
              path: "held.mp4",
              name: "Unidentified toy spot",
              durationMs: 30000,
              kind: "commercial",
              reason: "Loomarr couldn't work out what this is, so it will only match broadly.",
              confidence: 45,
            },
          ],
          reels: [],
          // V38's audit half — what was filed with nobody looking.
          recentlyFiled: [
            {
              path: "auto.mp4",
              name: "Hot Wheels spot",
              durationMs: 30000,
              kind: "commercial",
              reason: "Loomarr was confident enough about these tags to file it without asking.",
              confidence: 88,
              autoFiled: true,
            },
          ],
          total: 1,
        }),
      );
    }
    // Before the /v1/filler catalog match: a persisted split proposal (V34) for the
    // /filler/splits/$proposalId review route this suite derives from the router.
    if (u.includes("/v1/filler/splits/")) {
      return Promise.resolve(
        json({
          id: "sp-1",
          clipPath: "comp.mp4",
          createdAt: "2026-07-25T20:00:00Z",
          segments: [{ index: 0, startMs: 0, endMs: 30000, name: "First ad" }],
        }),
      );
    }
    // One clip, not an empty catalog: the per-clip actions (split, tag, pin) only render
    // when there is a card to hang them on, and this suite exists to prove they mount.
    if (u.includes("/v1/filler")) {
      return Promise.resolve(
        json({
          clips: [
            {
              hash: "hash-comp",
              name: "80s compilation",
              kind: "commercial",
              durationMs: 900000,
              tagged: false,
              aiTagged: false,
              playCount: 0,
              playsCounted: true,
            },
          ],
        }),
      );
    }
    if (u.includes("/v1/channels/now-next")) return Promise.resolve(json({ channels: [] }));
    if (u.includes("/pods")) return Promise.resolve(json({ entries: [], totalMs: 0, matchLevel: "exact" }));
    if (u.includes("/v1/channels/")) {
      return Promise.resolve(
        json({
          id: "ch-1",
          name: "Cartoons",
          number: 42,
          status: "live",
          strategy: "shuffle",
          programCount: 3,
          pendingCount: 1,
          breakCount: 0,
          slotCount: 4,
          policy: {},
        }),
      );
    }
    if (u.includes("/v1/channels")) {
      return Promise.resolve(
        json({
          channels: [
            {
              id: "ch-1",
              name: "Cartoons",
              number: 42,
              status: "live",
              strategy: "shuffle",
              programCount: 3,
              pendingCount: 1,
              breakCount: 0,
              slotCount: 4,
              policy: {},
            },
          ],
        }),
      );
    }
    if (u.includes("/v1/titles")) return Promise.resolve(json({ titles: [] }));
    if (u.includes("/v1/proposals")) return Promise.resolve(json({ proposals: [] }));
    if (u.includes("/v1/proposals")) return Promise.resolve(json({ proposals: [] }));
    return Promise.resolve(json({}));
  });
  vi.stubGlobal("fetch", mock);
  vi.stubGlobal(
    "EventSource",
    class {
      addEventListener() {}
      close() {}
    },
  );
};

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

// Turn a generated route id into a navigable path: strip pathless layout segments
// (`_authed`), drop the trailing index marker, and fill params with a value the stub
// serves. Deriving these from the router means a NEW route is covered the day it lands.
const pathOf = (id: string): string => {
  const path = id
    .split("/")
    .filter((seg) => seg !== "" && !seg.startsWith("_"))
    .map((seg) => (seg.startsWith("$") ? "ch-1" : seg))
    .join("/");
  return `/${path}`;
};

const routeIds = (): string[] => {
  const queryClient = new QueryClient();
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  return Object.keys(router.routesById).filter(
    (id) =>
      id !== "__root__" &&
      // Layout routes render only an <Outlet>; their children are covered individually.
      id !== "/_authed" &&
      id !== "/_authed/settings" &&
      // The catch-all is SUPPOSED to be a placeholder — it is the 404 page.
      id !== "/_authed/$" &&
      // Login and the wizard redirect an authenticated admin away; they have their own
      // suites (auth + wizard) that drive them unauthenticated.
      id !== "/login" &&
      id !== "/wizard",
  );
};

afterEach(() => vi.restoreAllMocks());

describe("every route is reachable", () => {
  it.each(routeIds().map((id) => [id, pathOf(id)] as const))("%s renders real content", async (_id, path) => {
    stubFetch();
    renderAt(path);

    // A heading proves the screen composed, not just that the shell painted around an
    // empty pane. The shell's own nav has no headings, so this cannot pass vacuously.
    await waitFor(
      () => {
        const headings = screen.getAllByRole("heading");
        expect(headings.length).toBeGreaterThan(0);
      },
      { timeout: 3000 },
    );

    // The catch-all's copy appearing anywhere else means the path did not match the
    // route we think it did.
    expect(screen.queryByText("Off the air")).not.toBeInTheDocument();
  });
});

describe("feature-gated panels mount when their flag is on", () => {
  // Each entry names a panel that EXISTS as a component but has, at least once in this
  // phase, not been rendered by the page that owns it.
  it.each([
    ["/settings/ai", /probing your llm host|model|provider/i, "the §8.1 model picker"],
    ["/settings/security", /api token|session secret/i, "the generated-secrets panel"],
    // ⚠ The SSO block AND the note stating what SSO does not do (§11, V8). The note is the
    // part most likely to be lost in a tidy-up — it looks like prose rather than a control —
    // and losing it leaves §11's unusual model (most apps DO auto-create) reading as an
    // oversight.
    ["/settings/security", /does not create an account here/i, "the SSO scope note"],
    ["/people", /import from your media server/i, "the §11 import panel"],
    // ⚠ The ingest panel moved to INCOMING (V35). Discover was retired as a tab — finding
    // clips is now something you do to a source — and the download tooling went with the tab
    // that is about how clips ARRIVE. Keeping this pointed at a real URL is what stops a tab
    // nobody can navigate to from passing as "reachable".
    //
    // ⚠ The ingest panel's assertion was here and is DELETED with it (V38b, retired-ok). This
    // suite guards that a built thing is REACHABLE; a row for a deleted surface asserts the
    // opposite of what it looks like, and would have to be satisfied by re-adding the panel.
    //
    // What replaced it is the Sources row below — "add a source" is now the door clips come
    // through, and it is asserted there.
    // V35: the queue of clips waiting on a human decision. Same reason as the ingest panel —
    // this suite exists because eight things were built, unit-tested and imported by nothing.
    ["/filler/incoming", /nothing needs you|needs? a decision/i, "the incoming queue"],
    // ⚠ V38's AUDIT half. Auto-filing is on by default, so clips enter the catalog unattended;
    // if this section stops rendering, an operator has no way to find what was filed without
    // them, and nothing else on the page would look wrong. That is precisely the silent-loss
    // shape this suite exists for.
    ["/filler/incoming", /filed .* without asking/i, "the auto-filed audit list"],
    // ⚠ And the tab itself must be reachable FROM the catalog, or the assertions above only
    // prove a deep link works. This is the V1/V17a/V23 failure in tab form.
    ["/filler", /^incoming$/i, "the Incoming tab's own entry point"],
    // V35: catalog health is a strip above the tabs rather than a tab of its own, so it has
    // no nav entry to assert — it must simply BE on the page, on every tab.
    ["/filler", /fits a break/i, "the pool-health strip"],
    // V34: the split review route exists, but if no card offers the entry point the
    // operator can never reach it. The action lives on each clip card (admin).
    ["/filler", /split into clips/i, "the compilation-split entry point"],
    // V35 item 1.7: the per-channel override picker. Its entry point is on each clip card
    // (admin) — the picker itself is behind a click, so this asserts the DOOR, which is the
    // half that has gone missing eight times before.
    ["/filler", /use in a channel/i, "the channel-override entry point"],
    // V35: per-source search, on the Sources tab. ⚠ `GET /v1/filler/discover` was API-ONLY for
    // a whole phase — the route shipped, `DiscoverPanel` was deleted rather than left orphaned,
    // and nothing called it. This is the assertion that stops it going back to that state.
    //
    // ⚠ V35b moved the search INSIDE the archive source row, behind a "Search it" toggle, and
    // this assertion correctly went red when the old "Find clips" heading disappeared. It now
    // names the new DOOR. It deliberately does not reach past the toggle: what this suite
    // guards is that an entry point exists, and asserting on something behind the click would
    // pass just as happily with no way in — the exact state it was written to catch. The
    // panel's own contents are covered by the expander test below.
    ["/filler/sources", /search it/i, "the per-source search's entry point"],
    // V37: adding a source. ⚠ `POST /v1/filler/sources` shipped in V35 and NOTHING called it for
    // two phases — the route existed, the store wrote, and there was no way to reach it from the
    // app. That is the same API-only state this suite caught for `discover`, and the reason
    // YouTube could not be registered was partly that no UI ever tried.
    ["/filler/sources", /add a source/i, "the add-a-source form"],
  ])("%s mounts %s", async (path, pattern) => {
    stubFetch();
    renderAt(path);
    // findAllBy, not findBy: a panel legitimately renders its name more than once (a
    // heading plus a row label). Presence is the assertion here, not uniqueness.
    const found = await screen.findAllByText(pattern, undefined, { timeout: 3000 });
    expect(found.length).toBeGreaterThan(0);
  });

  // V35b: the per-source search moved INSIDE the archive row, behind a "Search it" toggle. The
  // row above proves the door exists; this proves it OPENS onto the real panel.
  //
  // ⚠ The assertion is the search's downloads-nothing promise, not merely the input. That line
  // is a behaviour claim — it is why an operator dares to browse a collection at all — and it
  // previously sat in a card that was always mounted. Behind a click it needs a click to guard,
  // or the promise could silently stop rendering with every test still green.
  it("/filler/sources opens the archive row's search onto its downloads-nothing promise", async () => {
    stubFetch();
    renderAt("/filler/sources");
    await userEvent.click(await screen.findByRole("button", { name: /search it/i }));
    const found = await screen.findAllByText(/nothing downloads until you queue it/i, undefined, {
      timeout: 3000,
    });
    expect(found.length).toBeGreaterThan(0);
  });

  // The §12 pod preview lives in the channel-detail "Filler" tab (admin-only) — the detail page
  // is now a tabbed layout, one section shown at a time. This guards the tab is WIRED +
  // reachable: the Filler tab is present, and selecting it reveals the live draft-sandbox break.
  it("/channels/ch-1 reaches the §12 filler section (its tab) with the break preview", async () => {
    stubFetch();
    // ⚠ Rendered AT the section's own URL rather than clicking through from `/channels/ch-1`.
    // The section bar moved onto `NavTabs` (V-nav-paths), so each section is a real route — and
    // a deep link is the stronger reachability claim: it proves the URL an operator can bookmark
    // or be sent actually mounts the panel, not merely that a click within one page swaps it.
    // The tab's own presence is asserted separately below.
    renderAt("/channels/ch-1/filler");
    const found = await screen.findAllByText(/this channel's break/i, undefined, { timeout: 3000 });
    expect(found.length).toBeGreaterThan(0);
  });

  // ⚠ And the section must be reachable FROM the channel page, or the deep-link test above only
  // proves a URL works for someone who already knows it. This is the V1/V17a/V23 failure in tab
  // form — a panel that exists at an address nobody can navigate to.
  it("/channels/ch-1 offers the Filler section as a link", async () => {
    stubFetch();
    renderAt("/channels/ch-1");
    // ⚠ Scoped to the section bar by NAME, because the app SIDEBAR also has a "Filler" link
    // (to `/filler`, the catalog) and it wins a bare `findByRole("link", { name: "Filler" })`.
    // Two different destinations sharing one accessible name is fine for a sighted user — the
    // bars are far apart — but a test has to say which one it means.
    const sectionBar = await screen.findByRole("navigation", { name: /channel sections/i });
    const tab = within(sectionBar).getByRole("link", { name: "Filler" });
    // A real destination, not a button that swaps state: middle-clickable, copyable, bookmarkable.
    expect(tab).toHaveAttribute("href", expect.stringContaining("/channels/ch-1/filler"));
  });

  // V29b's meter, in the same tab. Guarded here rather than trusted because this suite exists
  // for exactly this shape of miss: the component has stories, six unit tests and a Go test
  // proving it agrees with pod assembly, and every one of those passes whether or not anything
  // renders it. Only a route test answers "can an operator see it".
  it("/channels/ch-1 reaches the filler coverage meter", async () => {
    stubFetch();
    renderAt("/channels/ch-1/filler");
    expect(await screen.findByText(/catalog coverage/i, undefined, { timeout: 3000 })).toBeInTheDocument();
    // And the meter itself rendered, not just its heading.
    expect(await screen.findByText("Exact match")).toBeInTheDocument();
  });

  // The eighth instance of this file's founding bug: ChannelIconField shipped complete —
  // stories, five visual baselines, an admin gate — and was imported by nothing, so the
  // channel icon was unreachable in the app. Its component tests all passed, which is
  // exactly the blind spot this suite exists for.
  it("/channels/ch-1 reaches the channel icon field on the info panel", async () => {
    stubFetch();
    renderAt("/channels/ch-1");
    // Info is the default panel (and the viewer's only one), so no tab click is needed.
    expect(await screen.findByText("Channel icon")).toBeInTheDocument();
  });

  // The ninth instance, and the one this suite failed to prevent: V7 shipped
  // POST /v1/auth/password with 19 tests and no screen, so a user still could not
  // change their password by clicking anything. A route test is the gate — the
  // endpoint being correct was never the question.
  it("/account reaches the change-password form for a local user", async () => {
    stubFetch();
    renderAt("/account");
    expect(await screen.findByText("Your account")).toBeInTheDocument();
    expect(await screen.findByLabelText("Current password")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /change password/i })).toBeInTheDocument();
  });
});
