import type { SettingEntry } from "@loomarr/api";
import {
  getBootstrapMockHandler,
  getLoginMockHandler,
  getSettingsListMockHandler,
  getSettingsPatchMockHandler,
  getSetupStatusMockHandler,
  getSetupTestMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { setting } from "@/test/fixtures/settings";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// Wizard coverage driven through the REAL generated route tree (§13, config-design §6):
// first-run routing off `setup.completed`, the unauthenticated bootstrap step, and the
// bootstrap → auto-login → checklist advance.
const ADMIN = me();
const GREEN_CHECKS = [
  { name: "media_server", ok: true },
  { name: "tunarr", ok: true },
  { name: "llm", ok: false, hint: "No LLM configured — suggestions stay off until you connect one." },
];

// The connections step (config-design §6) renders the settings GROUP forms, so the
// settings list must carry a field per group for its blocks to appear. One essential key
// each is enough to exercise the block headings + Test/verdict.
const entry = (key: string, group: string): SettingEntry =>
  setting({ key, group, doc: `${key} for tests`, set: false, value: "" });
// An enum entry renders as the Base UI Select — the control the flavor-save regression
// (below) exercises. `enum` fills its options; the jsdom Select shims live in test/setup.ts.
const enumEntry = (key: string, group: string, options: string[]): SettingEntry => ({
  ...entry(key, group),
  kind: "enum",
  enum: options,
});
const CONNECTION_ENTRIES = [
  enumEntry("library.flavor", "connections.media_server", ["emby", "jellyfin"]),
  entry("media_server.url", "connections.media_server"),
  entry("tunarr.url", "connections.tunarr"),
  entry("seerr.url", "connections.requester"),
  entry("tmdb.api_key", "connections.tmdb"),
];

// `backend` is the saved `playout.backend` (§9.1), which decides which steps the wizard has
// at all. Defaults to `tunarr` here rather than to the registry default, because most of the
// assertions in this file predate the playout choice and describe the Tunarr-shaped flow;
// the internal path gets its own explicit cases below.
//
// ⚠ `me` is HAND-WRITTEN because it is STATEFUL (401 until a login lands) and status-bearing:
// the spec declares errors via `default:` with no 401, so orval has nothing to generate from.
// Everything else here is route-bound, and the request SEQUENCE is recorded because two
// assertions below are about order, not presence.
const stubWizard = (opts: {
  authed: boolean;
  setupCompleted?: boolean;
  checks?: Array<{ name: string; ok: boolean; hint?: string }>;
  backend?: "internal" | "tunarr";
  publicUrl?: string;
  publicUrlPinned?: boolean;
}) => {
  let authed = opts.authed;
  const seq: string[] = [];
  const patches: string[] = [];

  server.use(
    http.get("*/v1/auth/me", () =>
      authed ? HttpResponse.json(ADMIN) : HttpResponse.json({ title: "Unauthorized" }, { status: 401 }),
    ),
    getBootstrapMockHandler(() => {
      seq.push("bootstrap");
      return { id: ADMIN.id, name: ADMIN.name, role: ADMIN.role };
    }),
    getLoginMockHandler(() => {
      seq.push("login");
      authed = true;
      return ADMIN;
    }),
    getSetupStatusMockHandler({ checks: opts.checks ?? GREEN_CHECKS }),
    // The real endpoint tests PERSISTED settings, so the FE must save first; the mock just
    // acks. The flavor-save regression asserts the PATCH landed BEFORE this call.
    getSetupTestMockHandler(() => {
      seq.push("test");
      return { ok: true, hint: "Connection OK" };
    }),
    getSettingsPatchMockHandler(async ({ request }) => {
      seq.push("patch");
      patches.push(JSON.stringify(await request.json()));
      return { results: [] };
    }),
    getSettingsListMockHandler({
      features: {},
      settings: [
        ...CONNECTION_ENTRIES,
        {
          ...entry("playout.backend", "playout"),
          kind: "enum",
          enum: ["internal", "tunarr"],
          value: opts.backend ?? "tunarr",
        },
        setting({
          key: "server.public_url",
          label: "Loomarr address",
          group: "playout",
          kind: "url",
          doc: "Loomarr's own address as your media server can reach it.",
          value: opts.publicUrl ?? "http://loomarr:8080",
          provenance: opts.publicUrlPinned ? "env" : "db",
          envVar: "SERVER_PUBLIC_URL",
        }),
        setting({
          key: "setup.completed",
          group: "advanced",
          kind: "bool",
          doc: "First-run wizard completed.",
          advanced: true,
          value: String(opts.setupCompleted ?? true),
        }),
      ],
    }),
    ...appHandlers(),
  );

  return { seq, patches };
};

const renderAt = (initialPath: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

describe("first-run routing", () => {
  it("sends the operator to the wizard while setup.completed is false", async () => {
    stubWizard({ authed: true, setupCompleted: false });
    renderAt("/");
    expect(await screen.findByText(/first-run setup/i)).toBeInTheDocument();
  });

  it("goes straight to Channels once setup is completed", async () => {
    stubWizard({ authed: true, setupCompleted: true });
    renderAt("/");
    expect(await screen.findByRole("heading", { name: "Channels" })).toBeInTheDocument();
  });
});

describe("wizard", () => {
  it("opens on bootstrap when no one is signed in", async () => {
    stubWizard({ authed: false });
    renderAt("/wizard");
    expect(await screen.findByRole("heading", { name: /create your admin account/i })).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByLabelText("Confirm password")).toBeInTheDocument();
  });

  it("creates the admin, signs in, and advances to the checklist", async () => {
    const { seq } = stubWizard({ authed: false });
    renderAt("/wizard");

    await userEvent.type(await screen.findByLabelText("Username"), "ada");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2!");
    await userEvent.type(screen.getByLabelText("Confirm password"), "hunter2!");
    await userEvent.click(screen.getByRole("button", { name: /create admin/i }));

    // Advances to PLAYOUT, not Connections: the playout choice decides which connections are
    // even required (§9.1), so it has to be answered before the checklist is meaningful.
    expect(
      await screen.findByRole("heading", { name: /how should loomarr play your channels/i }),
    ).toBeInTheDocument();
    // Bootstrap THEN login, in that order — the auto-login is the whole point of the step, and
    // "both were called" would also be true if they fired the other way round.
    expect(seq.filter((s) => s === "bootstrap" || s === "login")).toEqual(["bootstrap", "login"]);
  });

  it("rejects a mismatched confirmation before calling the API", async () => {
    const { seq } = stubWizard({ authed: false });
    renderAt("/wizard");

    await userEvent.type(await screen.findByLabelText("Username"), "ada");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2!");
    await userEvent.type(screen.getByLabelText("Confirm password"), "different!");
    await userEvent.click(screen.getByRole("button", { name: /create admin/i }));

    expect(await screen.findByText(/passwords don't match/i)).toBeInTheDocument();
    expect(seq).not.toContain("bootstrap");
  });

  it("shows the connections as inline forms and blocks while a required one is red", async () => {
    stubWizard({
      authed: true,
      setupCompleted: false,
      checks: [
        { name: "media_server", ok: true },
        { name: "tunarr", ok: false, hint: "Tunarr didn't answer on that URL." },
      ],
    });
    renderAt("/wizard");

    // Each connection is a collapsible block AND a rail sub-item (§13 sub-nav), so the
    // names appear twice — that is the point, not a bug.
    expect(await screen.findAllByText("Media server")).not.toHaveLength(0);

    // The connections step renders the settings-group FORM, not a read-only checklist —
    // configure in place (§6). A Test-connection button per block is the tell (there is no
    // "Fix ↗ go to Settings" here anymore).
    expect(screen.getAllByRole("button", { name: /test connection/i }).length).toBeGreaterThan(0);

    // A required check that is red keeps Continue disabled. Tunarr is required here (this
    // install is Tunarr-backed) even though its FORM now lives on the Playout step — being
    // configured elsewhere does not make it stop blocking.
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
    // …and Tunarr is not offered as a connection block on this screen any more.
    const rail = within(screen.getByRole("complementary"));
    expect(rail.queryByRole("button", { name: "Tunarr" })).not.toBeInTheDocument();
  });

  it("saves the flavor before testing — Test checks what's on screen, not stale settings", async () => {
    // Regression: /v1/setup/test evaluates PERSISTED settings, so testing an unsaved edit
    // ran against the OLD (empty) flavor — picking "emby" then Testing showed "set a media
    // server flavor". Test must PATCH the dirty edits first, THEN test. Asserted by call
    // ORDER: the PATCH (carrying library.flavor) must precede the /v1/setup/test POST.
    const { seq, patches } = stubWizard({
      authed: true,
      setupCompleted: false,
      // Both required checks red so the wizard STAYS on the connections step (a
      // satisfied checklist auto-advances — see the "resumes past" test below).
      checks: [
        { name: "media_server", ok: false, hint: "Not connected yet" },
        { name: "tunarr", ok: false, hint: "Not connected yet" },
      ],
    });
    renderAt("/wizard");

    // Wait for the connections step to mount (auth + route resolve async), then open
    // the media-server block from the rail and pick a flavor in the Select.
    await screen.findByRole("heading", { name: /connect your services/i });
    const rail = within(screen.getByRole("complementary"));
    await userEvent.click(rail.getByRole("button", { name: "Media server" }));
    await userEvent.click(await screen.findByRole("combobox", { name: /library flavor/i }));
    await userEvent.click(await screen.findByRole("option", { name: "emby" }));

    // The media-server block is the one open, so its Test button is the first.
    const [testButton] = screen.getAllByRole("button", { name: /test connection/i });
    if (!testButton) throw new Error("no Test connection button rendered");
    await userEvent.click(testButton);

    // The save landed before the test — and carried the flavor the operator just picked.
    // ⚠ Order comes from two ROUTE-BOUND resolvers appending to `seq`, not from indices into
    // the stub's own call log filtered by url substring.
    await vi.waitFor(() => {
      expect(seq.filter((s) => s === "patch" || s === "test")).toEqual(["patch", "test"]);
    });
    expect(patches.join()).toContain("library.flavor");
  });

  it("resumes past the checklist when only optional integrations are red", async () => {
    // media_server + tunarr green, llm red: the checklist is satisfied (config-design §6
    // — Seerr/AI/TMDB are feature-gating, not blocking), so the wizard moves the operator
    // on to the next unfinished step rather than stranding them on a red X. With no standalone
    // Live TV or Webhooks step (Live TV auto-wires on the Tunarr save; availability is polled),
    // that next step is Library.
    stubWizard({ authed: true, setupCompleted: false });
    renderAt("/wizard");

    expect(await screen.findByRole("heading", { name: /give tunarr your library/i })).toBeInTheDocument();
  });

  // `?step=` / `?conn=` deep links (§13), through the REAL router so validateSearch runs —
  // the narrowing lives there, and a unit test of resolveStep alone would not exercise it.
  describe("deep links", () => {
    it("opens the step a link names", async () => {
      // The frontier here is Library (media_server + tunarr green, tunarr_library red), so a
      // link to the earlier Connections step is behind it and honoured.
      stubWizard({ authed: true, setupCompleted: false });
      renderAt("/wizard?step=checklist");

      expect(await screen.findByRole("heading", { name: /connect your services/i })).toBeInTheDocument();
    });

    // ⚠ The stranding case. Honouring this link would drop an unauthenticated operator on a
    // step whose Continue can never enable, with no clickable rail and no way forward.
    it("clamps a link that points past what the server says is done", async () => {
      stubWizard({ authed: false });
      renderAt("/wizard?step=channel");

      expect(await screen.findByRole("heading", { name: /create your admin account/i })).toBeInTheDocument();
    });

    it("lands somewhere real when a link names a step that no longer exists", async () => {
      stubWizard({ authed: true, setupCompleted: false });
      renderAt("/wizard?step=not-a-step");

      expect(await screen.findByRole("heading", { name: /give tunarr your library/i })).toBeInTheDocument();
    });

    // The support case this feature is really for: point someone at ONE service's form.
    it("reveals the connection block a link names", async () => {
      stubWizard({ authed: true, setupCompleted: false });
      renderAt("/wizard?step=checklist&conn=requester");

      expect(await screen.findByRole("heading", { name: /connect your services/i })).toBeInTheDocument();
      // ⚠ Asserted on aria-expanded, NOT on the presence of a field. ConnectionBlock keeps
      // every body MOUNTED and reveals it with a CSS grid transition, so a findByLabelText
      // succeeds whether the block is open or shut — an assertion that would pass with the
      // deep link doing nothing at all.
      const blocks = await screen.findAllByRole("button", { expanded: true });
      expect(blocks.map((b) => b.textContent)).toEqual([expect.stringContaining("Requester")]);
    });

    // ⚠ Bootstrap runs ONCE, so revisiting it must show the OUTCOME, never the form. The
    // defect: an operator walking Back (or deep-linking) got a full username/password/confirm
    // form for an action guaranteed to 409 — discoverable only by filling it in and
    // submitting. The backend was never at risk; the UI was advertising an impossible action.
    it("shows the completed bootstrap step read-only instead of a form that can only fail", async () => {
      stubWizard({ authed: true, setupCompleted: false });
      renderAt("/wizard?step=bootstrap");

      // Names the account, so the operator learns WHICH admin owns the instance — the
      // question that brings them back to this step.
      expect(await screen.findByText(/signed in as Ada/i)).toBeInTheDocument();
      // The form is GONE — not merely disabled.
      expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Confirm password")).not.toBeInTheDocument();
    });

    // ⚠ The trap the fix above creates if left alone: `advances` was hardcoded false on
    // bootstrap because the step self-advanced via its own submit. With the form gone there
    // would be no form AND no Continue — a worse dead end than the one being fixed.
    it("still offers Continue on a completed bootstrap step, which has no form to self-advance", async () => {
      stubWizard({ authed: true, setupCompleted: false });
      renderAt("/wizard?step=bootstrap");

      await screen.findByText(/signed in as Ada/i);
      const next = await screen.findByRole("button", { name: "Continue" });
      expect(next).toBeEnabled();

      // And it actually moves: bootstrap is done, so Continue lands on the next step, which
      // is now the playout choice.
      await userEvent.click(next);
      expect(
        await screen.findByRole("heading", { name: /how should loomarr play your channels/i }),
      ).toBeInTheDocument();
    });

    // The internal path, end to end through the real route tree. ⚠ This is the case the whole
    // change exists for: before it, an operator who wanted Loomarr to play its own channels
    // was shown a Tunarr connection block, given a "Give Tunarr your library" step, and
    // BLOCKED on a `tunarr` check they could never turn green.
    it("hides Tunarr's connection block when Loomarr does the streaming", async () => {
      stubWizard({
        authed: true,
        setupCompleted: false,
        backend: "internal",
        checks: [{ name: "media_server", ok: true }],
      });
      renderAt("/wizard?step=checklist");

      expect(await screen.findByRole("heading", { name: /connect your services/i })).toBeInTheDocument();
      // The media server is still there (the library lives there either way)…
      expect(await screen.findByLabelText("Media server URL")).toBeInTheDocument();
      // …and Tunarr is absent, not merely collapsed: its field is not in the DOM at all.
      expect(screen.queryByLabelText("Tunarr URL")).not.toBeInTheDocument();
      // Nor is it offered in the rail.
      const rail = within(screen.getByRole("complementary"));
      expect(rail.queryByRole("button", { name: "Tunarr" })).not.toBeInTheDocument();
    });

    it("does not block the internal path on a Tunarr check it can never satisfy", async () => {
      stubWizard({
        authed: true,
        setupCompleted: false,
        backend: "internal",
        // Tunarr is RED and stays red: an internal install has no Tunarr to fix.
        checks: [
          { name: "media_server", ok: true },
          { name: "tunarr", ok: false, hint: "Tunarr didn't answer on that URL." },
        ],
      });
      renderAt("/wizard?step=checklist");

      await screen.findByRole("heading", { name: /connect your services/i });
      expect(screen.getByRole("button", { name: "Continue" })).toBeEnabled();
    });

    it("drops the Tunarr library step from the rail on the internal path", async () => {
      stubWizard({ authed: true, setupCompleted: false, backend: "internal" });
      renderAt("/wizard?step=checklist");

      await screen.findByRole("heading", { name: /connect your services/i });
      const rail = within(screen.getByRole("complementary"));
      expect(rail.queryByText("Library")).not.toBeInTheDocument();
      // The playout choice itself IS in the rail, on both paths.
      expect(rail.getByText("Playout")).toBeInTheDocument();
    });

    it("keeps the Tunarr library step when Tunarr does the streaming", async () => {
      stubWizard({ authed: true, setupCompleted: false, backend: "tunarr" });
      renderAt("/wizard?step=checklist");

      await screen.findByRole("heading", { name: /connect your services/i });
      const rail = within(screen.getByRole("complementary"));
      expect(rail.getByText("Library")).toBeInTheDocument();
    });

    // Tunarr's own settings live on the Playout step, under the choice that makes them
    // relevant — not in Connections, which would split one decision across two screens.
    it("puts Tunarr's connection form on the playout step when Tunarr is chosen", async () => {
      stubWizard({ authed: true, setupCompleted: false, backend: "tunarr" });
      renderAt("/wizard?step=playout");

      await screen.findByRole("heading", { name: /how should loomarr play your channels/i });
      expect(await screen.findByLabelText("Tunarr URL")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /test connection/i })).toBeInTheDocument();
    });

    it("shows no Tunarr form on the playout step when Loomarr does the streaming", async () => {
      stubWizard({ authed: true, setupCompleted: false, backend: "internal" });
      renderAt("/wizard?step=playout");

      await screen.findByRole("heading", { name: /how should loomarr play your channels/i });
      expect(screen.queryByLabelText("Tunarr URL")).not.toBeInTheDocument();
    });

    it("requires a reachable Loomarr address on the internal playout step", async () => {
      stubWizard({ authed: true, setupCompleted: false, backend: "internal", publicUrl: "" });
      renderAt("/wizard?step=playout");

      expect(await screen.findByLabelText("Loomarr address")).toBeEnabled();
      expect(screen.getByText(/media server must be able to reach this address/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
    });

    it("persists the internal playout address through the shared settings API", async () => {
      const { patches } = stubWizard({
        authed: true,
        setupCompleted: false,
        backend: "internal",
        publicUrl: "",
      });
      renderAt("/wizard?step=playout");

      await userEvent.type(await screen.findByLabelText("Loomarr address"), "http://loomarr:8080");
      await userEvent.click(screen.getByRole("button", { name: "Save address" }));

      await vi.waitFor(() => {
        expect(patches).toContain(JSON.stringify({ edits: { "server.public_url": "http://loomarr:8080" } }));
      });
    });

    it("keeps an environment-pinned reachable address locked and accepts it", async () => {
      stubWizard({
        authed: true,
        setupCompleted: false,
        backend: "internal",
        publicUrl: "http://loomarr:8080",
        publicUrlPinned: true,
      });
      renderAt("/wizard?step=playout");

      expect(await screen.findByLabelText("Loomarr address")).toBeDisabled();
      expect(screen.getByText(/set via environment/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Continue" })).toBeEnabled();
    });

    it("falls back to the default block when a link names one that isn't a connection", async () => {
      stubWizard({ authed: true, setupCompleted: false });
      renderAt("/wizard?step=checklist&conn=library");

      // `library` is a STEP id, not a connection — narrowed away, so the step opens on its
      // default block (media server) instead of nothing.
      // Same aria-expanded signal, for the same reason: every block's body is mounted, so
      // only the expanded state distinguishes "opened" from "present".
      const blocks = await screen.findAllByRole("button", { expanded: true });
      expect(blocks.map((b) => b.textContent)).toEqual([expect.stringContaining("Media server")]);
    });
  });
});
