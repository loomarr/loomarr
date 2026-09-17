import {
  getMeMockHandler,
  getSettingsListMockHandler,
  getSettingsPatchMockHandler,
  getSetupStatusMockHandler,
  getSetupTestMockHandler,
  getSystemLlmDiscoverMockHandler,
  getSystemLlmPullMockHandler,
  getSystemLlmPullOperationMockHandler,
  getSystemLlmStatusMockHandler,
  getTunarrConnectMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { setting } from "@/test/fixtures/settings";
import { me } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

afterEach(() => window.sessionStorage.clear());

// ⚠ Was a local `entry()` helper duplicating what `setting()` now carries — and the local one
// was the reason four required SettingEntry fields could go missing elsewhere in the suite
// without anything noticing they were required at all.
const SETTINGS = [
  setting({
    key: "library.url",
    owner: "settings.connections",
    group: "connections.media_server",
    kind: "url",
    value: "http://emby:8096",
  }),
  setting({
    key: "library.token",
    owner: "settings.connections",
    group: "connections.media_server",
    kind: "secret",
    secret: true,
    preview: "…a1b2",
    value: "",
  }),
  setting({
    key: "tunarr.url",
    owner: "settings.connections",
    group: "connections.tunarr",
    kind: "url",
    value: "http://tunarr:8000",
  }),
  setting({
    key: "session.ttl",
    owner: "settings.access",
    label: "Sign-in lifetime",
    group: "users_security",
    kind: "duration",
    value: "720h",
  }),
  setting({
    key: "cookie.secure",
    owner: "settings.access",
    label: "Secure cookies",
    group: "users_security",
    kind: "enum",
    value: "auto",
    advanced: true,
    enumOptions: [
      { value: "auto", label: "Auto (match the request)" },
      { value: "always", label: "Always" },
      { value: "never", label: "Never (local dev only)" },
    ],
  }),
  setting({
    key: "job.workers",
    owner: "settings.advanced",
    group: "advanced",
    kind: "int",
    value: "2",
    provenance: "env",
  }),
  setting({ key: "llm.provider", owner: "settings.ai", group: "ai", kind: "enum", value: "ollama" }),
  setting({
    key: "llm.url",
    owner: "settings.ai",
    group: "ai",
    kind: "url",
    value: "http://localhost:11434",
  }),
  setting({
    key: "access.public_url",
    owner: "settings.sharing",
    label: "Recipient-facing Loomarr address",
    group: "general",
    kind: "url",
    value: "https://loomarr.example.com",
  }),
  setting({
    key: "filler.fetch.max_disk_gb",
    label: "Automatic-download storage limit",
    owner: "filler.storage",
    envVar: "FILLER_FETCH_MAX_DISK_GB",
    group: "filler",
    kind: "int",
    value: "20",
    advanced: true,
  }),
];

// Every write this page can make, route-bound, with the request SEQUENCE recorded.
//
// ⚠ The sequence is the point for one test below: `/v1/setup/test` evaluates PERSISTED settings,
// so a dirty edit has to be PATCHed before the test runs. Proving that needs ORDER, not presence.
// The old version derived order from indices into `fetchMock.mock.calls` filtered by url
// substring — which is order in the STUB, and the stub matched `/v1/settings` for the PATCH and
// `/v1/setup/test` for the test with no route binding behind either.
const stubSettings = (settings = SETTINGS) => {
  const seq: string[] = [];
  const patches: unknown[] = [];
  const connects: string[] = [];

  server.use(
    getMeMockHandler(me()),
    getSetupStatusMockHandler({
      checks: [
        { name: "media_server", ok: false, hint: "Emby refused the token." },
        { name: "tunarr", ok: false, hint: "Tunarr is not connected." },
      ],
    }),
    getSetupTestMockHandler(() => {
      seq.push("test");
      return { ok: true };
    }),
    getSettingsPatchMockHandler(async ({ request }) => {
      seq.push("patch");
      const body = (await request.json()) as { edits: Record<string, string> };
      patches.push(body);
      // ⚠ `results` is a `SettingResult[]`. A sibling file was serving `{ results: {} }` — a
      // shape the API cannot produce — and passing. Return the per-key success contract too:
      // connection Test must stop when even one staged value is rejected.
      return { results: Object.keys(body.edits).map((key) => ({ key, status: "saved" as const })) };
    }),
    getSettingsListMockHandler({ features: {}, settings }),
    // Registered so the negative assertion below has something to be negative ABOUT. If the FE
    // ever calls it, `connects` is non-empty and the test says why; if this handler were absent
    // the unhandled-request guard would fail the test instead, which is a fine second net but a
    // worse error message.
    // ⚠ The response is `{ librariesEnabled, sourceId }`, NOT `{ ok }` — a shape the API has
    // never produced. Nothing caught it before because the FE is not supposed to call this at
    // all, which is exactly what the assertion below asserts.
    getTunarrConnectMockHandler(({ request }) => {
      connects.push(request.url);
      return { librariesEnabled: 0, sourceId: "src-1" };
    }),
    ...appHandlers(),
  );

  return { seq, patches, connects };
};

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

describe("Settings", () => {
  it("opens on a task-based home and finds the exact owner of a setting", async () => {
    stubSettings();
    renderAt("/settings");

    expect(await screen.findByRole("heading", { name: "Settings" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Set up Loomarr" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Access and devices" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "This server" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Troubleshoot" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Filler" })).toBeInTheDocument();

    const finder = screen.getByRole("searchbox", { name: "Find a setting" });
    await userEvent.type(finder, "FILLER_FETCH_MAX_DISK_GB");
    const match = await screen.findByRole("link", { name: /Automatic-download storage limit/ });
    expect(match).toHaveAttribute("href", "/filler/settings/storage");

    await userEvent.type(finder, "{ArrowDown}");
    expect(match).toHaveFocus();
  });

  it("keeps task browsing available when a search has no matches", async () => {
    stubSettings();
    renderAt("/settings");

    await userEvent.type(
      await screen.findByRole("searchbox", { name: "Find a setting" }),
      "not a real setting",
    );
    expect(await screen.findByText("No settings found")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Connections/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "This server" })).toBeInTheDocument();
  });

  it("uses a grouped server landing instead of a second tab matrix", async () => {
    stubSettings();
    renderAt("/settings/system");

    expect(await screen.findByRole("heading", { name: "This server" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Run this server" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Troubleshoot" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Playback/ })).toHaveAttribute(
      "href",
      "/settings/system/playback",
    );
    expect(screen.getByRole("link", { name: /Diagnostics/ })).toHaveAttribute(
      "href",
      "/settings/system/diagnostics",
    );
    expect(screen.queryByRole("navigation", { name: "System settings" })).not.toBeInTheDocument();
  });

  it("shows one notification provider list with SMTP as a peer provider", async () => {
    stubSettings();
    renderAt("/settings/notifications");

    expect(await screen.findByRole("heading", { name: "Notifications" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Notification providers" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add provider" })).toBeInTheDocument();
    expect(screen.getByText(/Add SMTP, Slack, Discord, or another provider/i)).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Email account messages" })).not.toBeInTheDocument();
    expect(screen.queryByRole("switch", { name: "Send email notifications" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Send test email" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Find another setting" })).toHaveAttribute("href", "/settings");
  });

  it("owns the recipient-facing Loomarr address under Access and devices", async () => {
    stubSettings();
    renderAt("/settings/access");

    expect(await screen.findByRole("heading", { name: "Access and devices" })).toBeInTheDocument();
    expect(
      await screen.findByRole("heading", { name: "Share invitation and recovery links" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Defaults to the browser address you're using now. Change it if recipients reach Loomarr at a different address. Loomarr uses the saved value for copied links, QR codes, and account email.",
      ),
    ).toBeInTheDocument();
    expect(await screen.findByLabelText("Recipient-facing Loomarr address")).toHaveValue(
      "https://loomarr.example.com",
    );
    expect(screen.getByRole("link", { name: "Find another setting" })).toHaveAttribute("href", "/settings");
  });

  it("defaults an unset recipient-facing address to the current browser origin", async () => {
    stubSettings(
      SETTINGS.map((entry) => (entry.key === "access.public_url" ? { ...entry, value: "" } : entry)),
    );
    renderAt("/settings/access");

    expect(await screen.findByLabelText("Recipient-facing Loomarr address")).toHaveValue(
      window.location.origin,
    );
  });

  it("does not replace an environment-managed recipient address with the browser origin", async () => {
    stubSettings(
      SETTINGS.map((entry) =>
        entry.key === "access.public_url" ? { ...entry, provenance: "env" as const, value: "" } : entry,
      ),
    );
    renderAt("/settings/access");

    expect(await screen.findByLabelText("Recipient-facing Loomarr address")).toHaveValue("");
  });

  it("keeps routing implementation details out of notification setup", async () => {
    stubSettings();
    renderAt("/settings/notifications");

    expect(await screen.findByRole("heading", { name: "Notification providers" })).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent(
      /destination draft|audience|installation scope|credentials map/i,
    );
  });

  it("self-diagnoses each connection on its own block (§5 status-per-block)", async () => {
    stubSettings();
    renderAt("/settings/connections");
    // media_server's check fails, so its ConnectionBlock opens and shows the BE's hint
    // inline — diagnosis on the thing that fixes it, not in a separate checklist above.
    expect(await screen.findByText("Emby refused the token.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /media server/i })).toHaveAttribute("aria-expanded", "true");
    // Tunarr has no passing verdict either, but a fresh install must not expand every problem
    // into one long wall of controls. Its collapsed header still says it needs attention.
    const tunarr = screen.getByRole("button", { name: /tunarr/i });
    expect(tunarr).toHaveAttribute("aria-expanded", "false");
    expect(tunarr).toHaveTextContent("needs attention");
    // Exploring the next service stays focused: connection forms behave as an accordion.
    await userEvent.click(tunarr);
    expect(tunarr).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("button", { name: /media server/i })).toHaveAttribute("aria-expanded", "false");
    // No standalone "connection checklist" duplicating the block statuses — the wiring
    // actions self-report on their own blocks, quiet once set up (§5, §13).
    expect(screen.queryByRole("heading", { name: /connection checklist/i })).not.toBeInTheDocument();
  });

  it("opens the TMDB block when a channel-suggestion recovery link targets it", async () => {
    const tmdb = setting({
      key: "tmdb.api_key",
      label: "TMDB API key",
      group: "connections.tmdb",
      kind: "secret",
      secret: true,
      set: false,
    });
    stubSettings([...SETTINGS, tmdb]);
    server.use(
      getSetupStatusMockHandler({
        checks: [
          { name: "media_server", ok: false, hint: "Emby refused the token." },
          { name: "tmdb", ok: false, hint: "Add a TMDB API key." },
        ],
      }),
    );

    renderAt("/settings/connections?focus=tmdb");

    expect(await screen.findByRole("button", { name: /^TMDB/i })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("button", { name: /media server/i })).toHaveAttribute("aria-expanded", "false");
    expect(await screen.findByLabelText("TMDB API key")).toBeInTheDocument();
  });

  it("saves the whole page from one bar, sending only what changed", async () => {
    const { patches } = stubSettings();
    renderAt("/settings/connections");

    // The bar is absent until something is dirty — a page being read stays quiet.
    expect(screen.queryByRole("region", { name: /unsaved changes/i })).not.toBeInTheDocument();

    await userEvent.type(await screen.findByLabelText("Library URL"), "9");
    const bar = await screen.findByRole("region", { name: /unsaved changes/i });
    expect(bar).toHaveTextContent("1 unsaved change");

    await userEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await expect.poll(() => patches).toHaveLength(1);
    const body = patches[0] as { edits: Record<string, string> };
    // Only the edited key — an untouched secret must not be sent, or it would be cleared (§9).
    expect(Object.keys(body.edits)).toEqual(["library.url"]);
    expect(body.edits["library.url"]).toBe("http://emby:80969");
  });

  it("discards edits without saving", async () => {
    stubSettings();
    renderAt("/settings/connections");

    await userEvent.type(await screen.findByLabelText("Library URL"), "9");
    await userEvent.click(screen.getByRole("button", { name: /discard/i }));
    expect(screen.queryByRole("region", { name: /unsaved changes/i })).not.toBeInTheDocument();
  });

  it("runs a per-block connection test", async () => {
    const { seq } = stubSettings();
    renderAt("/settings/connections");

    const tests = await screen.findAllByRole("button", { name: /test connection/i });
    await userEvent.click(tests[0] as HTMLElement);
    await expect.poll(() => seq).toContain("test");
  });

  // Regression: /v1/setup/test evaluates PERSISTED settings, so testing an UNSAVED edit
  // probes the OLD stored value — typing an Emby token then pressing Test 401'd against the
  // empty stored token, even though the right value was on screen. Test must PATCH the dirty
  // edits FIRST, then test. Asserted by call ORDER: the PATCH must precede the /setup/test.
  it("saves a dirty edit before testing, so Test checks what's on screen", async () => {
    const { seq } = stubSettings();
    renderAt("/settings/connections");

    // Type into the media-server block (its Test button is the first), then Test WITHOUT Save.
    await userEvent.type(await screen.findByLabelText("Library URL"), "9");
    const tests = await screen.findAllByRole("button", { name: /test connection/i });
    await userEvent.click(tests[0] as HTMLElement);

    // Both must happen, and in this order. `seq` is appended by the two ROUTE-BOUND resolvers,
    // so "patch" can only mean PATCH /v1/settings and "test" can only mean POST /v1/setup/test.
    await expect.poll(() => seq).toEqual(["patch", "test"]);
  });

  // Advanced settings is a table, so its controls carry the raw key rather than a humanized
  // label. The env lock still applies — and this is the surface where someone would most likely
  // try to work around it, since it is the editor of last resort.
  it("locks an env-pinned key on the advanced-settings table", async () => {
    stubSettings();
    const { container } = renderAt("/settings/advanced");
    await screen.findByText("job.workers");
    expect(container.querySelector("#setting-job\\.workers")).toBeDisabled();
  });
});

// V9's central claim: "dirty state survives tab switches".
//
// ⚠ This was BROKEN before V9 and invisible: `SettingsPage` held `edits` in its own useState,
// so navigating to another tab unmounted the page and discarded them with no warning. An
// operator editing a connection, stepping over to check a default, and coming back found their
// work gone. The buffer now lives in the layout, above the <Outlet />.
describe("Settings — the save bar spans tabs (V9)", () => {
  it("keeps an unsaved edit when you switch tabs and come back", async () => {
    stubSettings();
    renderAt("/settings/connections");

    const url = await screen.findByLabelText("Library URL");
    await userEvent.clear(url);
    await userEvent.type(url, "http://emby:9999");
    // The bar counts it, which is what tells the operator anything is staged at all.
    expect(await screen.findByText(/1 unsaved/i)).toBeInTheDocument();

    // Leave through the task home and come back — navigation is not a commit boundary.
    await userEvent.click(screen.getByRole("link", { name: "Find another setting" }));
    await userEvent.click(await screen.findByRole("link", { name: "Advanced settings" }));
    await screen.findByText("job.workers");
    await userEvent.click(screen.getByRole("link", { name: "Find another setting" }));
    await userEvent.click(await screen.findByRole("link", { name: /Connections/ }));

    expect(await screen.findByLabelText("Library URL")).toHaveValue("http://emby:9999");
    expect(screen.getByText(/1 unsaved/i)).toBeInTheDocument();
  });

  // The count is global BY DESIGN. Per-tab buffers would make "2 unsaved" mean something
  // different depending on where you were standing, which is worse than no count.
  it("counts edits made on different tabs together", async () => {
    stubSettings();
    renderAt("/settings/connections");

    const url = await screen.findByLabelText("Library URL");
    await userEvent.clear(url);
    await userEvent.type(url, "http://emby:9999");

    await userEvent.click(screen.getByRole("link", { name: "Find another setting" }));
    await userEvent.click(await screen.findByRole("link", { name: "Advanced settings" }));
    await screen.findByText("job.workers");
    // `job.workers` is env-pinned in the fixture, so edit the OTHER connection key instead —
    // asserting against a disabled field would prove nothing about the buffer.

    await userEvent.click(screen.getByRole("link", { name: "Find another setting" }));
    await userEvent.click(await screen.findByRole("link", { name: /Connections/ }));
    const tunarr = await screen.findByLabelText("Tunarr URL");
    await userEvent.clear(tunarr);
    await userEvent.type(tunarr, "http://tunarr:9999");

    expect(await screen.findByText(/2 unsaved/i)).toBeInTheDocument();
  });
});

// A pull that fails must SAY so. Before this, an "error" frame cleared the progress and
// refreshed — indistinguishable from success, leaving the operator to eventually notice
// the row still said "Download". The frame carries the reason; it belongs on screen.
describe("AI model pull", () => {
  // ⚠ EventSource is stubbed GLOBALLY in src/test/setup.ts; this test replaces it to capture the
  // listener so it can push a frame. `unstubAllGlobals` puts the shared one back — without it the
  // capture leaks into whichever test runs next.
  afterEach(() => vi.unstubAllGlobals());

  it("surfaces a failed download instead of silently clearing it", async () => {
    let emit: ((e: MessageEvent) => void) | undefined;

    server.use(
      getMeMockHandler(me()),
      getSystemLlmStatusMockHandler({
        local: true,
        reachable: true,
        provider: "ollama",
        model: "",
        catalog: [
          {
            tag: "qwen3:8b",
            label: "Qwen3 8B",
            approxVramGiB: 5,
            fit: "fits",
            pulled: false,
            recommended: true,
            runtimeOk: true,
            // ⚠ Required, and it is a CAPABILITY flag (whether the model supports tool calls),
            // not a cosmetic one — the picker uses it to decide what a model can be used for.
            tools: true,
            vision: false,
            why: "Good fit.",
          },
        ],
        hosted: [],
      }),
      getSystemLlmPullMockHandler({ jobId: "pull-error" }),
      getSystemLlmPullOperationMockHandler({
        jobId: "pull-error",
        model: "qwen3:8b",
        status: "error",
        percent: -1,
        completed: 0,
        total: 0,
        error: "no space left on device",
        startedAt: "2026-09-01T12:00:00Z",
        updatedAt: "2026-09-01T12:01:00Z",
      }),
      // ⚠ FOUND BY THE GUARD. The AI settings page fetches the downloadable-model catalogue on
      // mount, and the old `json({})` catch-all answered it with an empty object — so the whole
      // discover path ran against `{}` with `models` undefined and nothing said so. Same endpoint,
      // same silent answer, as the one `wizard-ai-block` turned up in batch 2.
      getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
      getSettingsListMockHandler({ features: {}, settings: SETTINGS }),
      ...appHandlers(),
    );

    vi.stubGlobal(
      "EventSource",
      class {
        addEventListener(type: string, cb: (e: MessageEvent) => void) {
          if (type === "llm_pull") emit = cb;
        }
        removeEventListener() {}
        close() {}
      },
    );

    renderAt("/settings/ai");
    await userEvent.click(await screen.findByRole("button", { name: /download/i }));

    // The BE reports failure with a reason and percent -1 (a sentinel, not a percentage).
    emit?.({
      data: JSON.stringify({
        model: "qwen3:8b",
        status: "error",
        percent: -1,
        error: "no space left on device",
      }),
    } as MessageEvent);

    expect(await screen.findByText(/no space left on device/i)).toBeInTheDocument();
  });

  it("recovers a terminal download after the SSE frame is dropped", async () => {
    server.use(
      getMeMockHandler(me()),
      getSystemLlmStatusMockHandler({
        local: true,
        reachable: true,
        provider: "ollama",
        model: "",
        catalog: [
          {
            tag: "qwen3:8b",
            label: "Qwen3 8B",
            approxVramGiB: 5,
            fit: "fits",
            pulled: false,
            recommended: true,
            runtimeOk: true,
            tools: true,
            vision: false,
            why: "Good fit.",
          },
        ],
        hosted: [],
      }),
      getSystemLlmPullMockHandler({ jobId: "pull-1" }),
      getSystemLlmPullOperationMockHandler({
        jobId: "pull-1",
        model: "qwen3:8b",
        status: "error",
        percent: 42,
        completed: 420,
        total: 1000,
        error: "download interrupted by restart",
        startedAt: "2026-09-01T12:00:00Z",
        updatedAt: "2026-09-01T12:01:00Z",
      }),
      getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
      getSettingsListMockHandler({ features: {}, settings: SETTINGS }),
      ...appHandlers(),
    );

    renderAt("/settings/ai");
    await userEvent.click(await screen.findByRole("button", { name: /download/i }));

    expect(await screen.findByText(/download interrupted by restart/i)).toBeInTheDocument();
  });
});

// Each Settings page mounts its footer panel. These are one-line wirings in the route
// files that a component test can't see: both the model picker and the secrets panel were
// imported-but-never-rendered, so the feature was absent while every unit test stayed
// green. Asserting the panel reaches the page is what the component tests can't do.
describe("Settings page footers", () => {
  it("keeps generated secrets under Access and devices advanced controls", async () => {
    stubSettings();

    renderAt("/settings/access");
    await userEvent.click(await screen.findByRole("button", { name: /advanced access controls/i }));
    // The two operator-facing credentials are a closed set held in the component (config-design
    // §4), not a fetched list — so the assertion is that the panel is on the page at all.
    expect(await screen.findByText(/API token/i)).toBeInTheDocument();
    expect(screen.getByText(/Playback token/i)).toBeInTheDocument();
  });
});

describe("Settings progressive disclosure", () => {
  it("keeps cookie transport policy behind Access and devices' Advanced disclosure", async () => {
    stubSettings();
    renderAt("/settings/access");

    expect(await screen.findByLabelText("Sign-in lifetime")).toBeInTheDocument();
    expect(screen.queryByLabelText("Secure cookies")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /show advanced/i }));
    expect(await screen.findByLabelText("Secure cookies")).toBeInTheDocument();
  });
});

describe("Settings honesty", () => {
  it("prefills the backend-owned OpenRouter API base instead of asking the operator to guess", async () => {
    server.use(
      getMeMockHandler(me()),
      getSettingsListMockHandler({
        features: {},
        settings: [
          setting({
            key: "llm.provider",
            label: "Lineup AI provider",
            group: "ai",
            kind: "enum",
            value: "openai",
            enumOptions: [
              { value: "ollama", label: "Ollama" },
              { value: "openai", label: "OpenAI-compatible" },
            ],
          }),
          setting({
            key: "llm.url",
            label: "AI service address",
            group: "ai",
            kind: "url",
            value: "",
          }),
          setting({ key: "llm.model", label: "Hosted lineup model", group: "ai", value: "" }),
          setting({ key: "llm.api_key", group: "ai", kind: "secret", secret: true, set: false }),
          setting({ key: "suggest.max_acquisitions", group: "ai", kind: "int", value: "5" }),
          setting({ key: "filler.vision.provider", group: "filler", value: "inherit" }),
          setting({ key: "filler.vision.model", group: "filler", value: "" }),
          setting({ key: "filler.transcribe.provider", group: "filler", value: "whisper" }),
          setting({
            key: "filler.transcribe.model",
            group: "filler",
            value: "openai/whisper-large-v3",
          }),
        ],
      }),
      getSystemLlmStatusMockHandler({
        provider: "openai",
        local: false,
        reachable: false,
        model: "",
        catalog: [],
        hosted: [
          {
            key: "openrouter",
            label: "OpenRouter",
            baseUrl: "https://openrouter.ai/api/v1",
            keysUrl: "https://openrouter.ai/keys",
            keyConfigured: false,
            active: false,
            modelsLive: false,
            models: [
              {
                id: "google/gemini-vision",
                label: "Gemini Vision",
                tools: false,
                vision: true,
              },
              {
                id: "openai/whisper-large-v3",
                label: "Whisper large v3",
                tools: false,
                transcription: true,
              },
            ],
          },
        ],
      }),
      ...appHandlers(),
    );

    renderAt("/settings/ai");

    await waitFor(() =>
      expect(screen.getByLabelText("AI service address")).toHaveValue("https://openrouter.ai/api/v1"),
    );
    expect(screen.getByRole("button", { name: "Save & check AI setup" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Hosted lineup model")).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: /unsaved changes/i })).toHaveTextContent("1 unsaved change");
    expect(screen.getByText(/Add your OpenRouter key above/i)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Choose a lineup model" })).toBeInTheDocument();
    expect(screen.getByText("TMDB grounding is still needed")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Connect TMDB" })).toHaveAttribute(
      "href",
      "/settings/connections?focus=tmdb",
    );
    expect(screen.queryByRole("heading", { name: "Automatic model policy" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Filler analysis models" })).not.toBeInTheDocument();
    const modelHeading = screen.getByRole("heading", { name: "Choose a lineup model" });
    const behaviorHeading = screen.getByRole("heading", { name: "AI behavior" });
    expect(
      modelHeading.compareDocumentPosition(behaviorHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Vision" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Gemini Vision/i })).not.toBeInTheDocument();
  });

  it("offers a return to the saved channel draft from AI settings", async () => {
    window.sessionStorage.setItem(
      "loomarr.pendingProposalIntent",
      JSON.stringify({ description: "Saturday morning cartoons" }),
    );
    stubSettings();

    renderAt("/settings/ai");

    expect(await screen.findByText("Your channel draft is saved")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Return to channel" })).toHaveAttribute("href", "/guide");
  });

  it("stages capability-filtered vision and transcription roles from a configured hosted provider", async () => {
    server.use(
      getMeMockHandler(me()),
      getSettingsListMockHandler({
        features: {},
        settings: [
          setting({ key: "llm.provider", group: "ai", kind: "enum", value: "openai" }),
          setting({ key: "llm.url", group: "ai", kind: "url", value: "https://openrouter.ai/api/v1" }),
          setting({ key: "llm.api_key", group: "ai", kind: "secret", secret: true, set: true }),
          setting({ key: "filler.vision.provider", group: "filler", value: "inherit" }),
          setting({ key: "filler.vision.model", group: "filler", value: "" }),
          setting({ key: "filler.transcribe.provider", group: "filler", value: "whisper" }),
          setting({ key: "filler.transcribe.model", group: "filler", value: "openai/whisper-large-v3" }),
        ],
      }),
      getSystemLlmStatusMockHandler({
        provider: "openrouter",
        local: false,
        reachable: true,
        model: "openai/gpt-4o-mini",
        catalog: [],
        hosted: [
          {
            key: "openrouter",
            label: "OpenRouter",
            baseUrl: "https://openrouter.ai/api/v1",
            keysUrl: "https://openrouter.ai/keys",
            keyConfigured: true,
            active: true,
            modelsLive: true,
            models: [
              { id: "openai/gpt-4o-mini", label: "GPT-4o mini", tools: true },
              { id: "google/gemini-vision", label: "Gemini Vision", vision: true },
              { id: "openai/whisper-large-v3", label: "Whisper large v3", transcription: true },
              { id: "vendor/text-only", label: "Text only" },
            ],
          },
        ],
      }),
      ...appHandlers(),
    );

    renderAt("/settings/ai");
    expect(await screen.findByRole("heading", { name: "Choose a lineup model" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Advanced model roles/i }));
    const vision = await screen.findByRole("region", { name: "Vision" });
    expect(within(vision).queryByRole("button", { name: /Text only/i })).not.toBeInTheDocument();
    await userEvent.click(within(vision).getByRole("button", { name: /Gemini Vision/i }));

    const transcription = screen.getByRole("region", { name: "Transcription" });
    expect(within(transcription).getByRole("button", { name: /Bundled local Whisper/i })).toBeDisabled();
    await userEvent.click(within(transcription).getByRole("button", { name: /Whisper large v3/i }));
    expect(screen.getByRole("region", { name: /unsaved changes/i })).toHaveTextContent("4 unsaved changes");
  });

  it("disables the spoken-language filter with the reason when its local model is missing", async () => {
    server.use(
      getMeMockHandler(me()),
      getSettingsListMockHandler({
        features: {},
        settings: [
          setting({
            key: "filler.language",
            label: "Expected spoken language",
            group: "filler",
            presentation: "language",
            value: "en",
            advanced: true,
          }),
          setting({ key: "filler.language_provider", group: "filler", value: "whisper", advanced: true }),
          setting({ key: "filler.language_model", group: "filler", value: "", advanced: true }),
          setting({ key: "ingest.whisper_path", group: "filler", value: "/usr/bin/whisper", advanced: true }),
          setting({ key: "playout.ffmpeg_path", group: "playout", value: "/usr/bin/ffmpeg", advanced: true }),
        ],
      }),
      ...appHandlers(),
    );

    renderAt("/filler/settings/playback");

    expect(await screen.findByLabelText("Expected spoken language")).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Language filtering is off because no multilingual model is selected. Choose one in AI settings.",
    );
  });

  it("keeps hosted language filtering available for a configured keyless endpoint", async () => {
    server.use(
      getMeMockHandler(me()),
      getSettingsListMockHandler({
        features: {},
        settings: [
          setting({
            key: "filler.language",
            label: "Expected spoken language",
            group: "filler",
            presentation: "language",
            value: "en",
            advanced: true,
          }),
          setting({ key: "filler.language_provider", group: "filler", value: "hosted", advanced: true }),
          setting({ key: "llm.url", group: "ai", value: "http://ai.internal/v1" }),
          setting({ key: "llm.model", group: "ai", value: "audio-model" }),
          setting({ key: "llm.api_key", group: "ai", kind: "secret", secret: true, set: false }),
          setting({ key: "playout.ffmpeg_path", group: "playout", value: "/usr/bin/ffmpeg", advanced: true }),
        ],
      }),
      ...appHandlers(),
    );

    renderAt("/filler/settings/playback");

    expect(await screen.findByLabelText("Expected spoken language")).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

// Wiring (Tunarr → the guide; Tunarr → the library) is no longer a manual button on
// Connections: it's an idempotent effect the server runs on save (config-design §5). So the
// Connections page must NOT show wiring actions, and saving a connection must just PATCH
// settings — the BE auto-wires (its own test proves the connectors fire). This guards that
// the confusing manual scaffolding stayed gone and didn't creep back.
describe("Connections auto-wires on save (no manual wiring UI)", () => {
  const stubWiring = () => {
    const base = stubSettings();
    // Two failing wiring checks, replacing the single media-server one. A second `server.use`
    // PREPENDS, so this one wins over the status handler registered above — the across-calls
    // half of the precedence rule in handlers.ts.
    server.use(
      getSetupStatusMockHandler({
        checks: [
          { name: "livetv", ok: false, hint: "Tunarr is not a tuner yet." },
          { name: "tunarr_library", ok: false, hint: "Tunarr has no media source." },
        ],
      }),
    );
    return base;
  };

  it("shows no manual wiring buttons on Connections", async () => {
    stubWiring();
    renderAt("/settings/connections");
    // Wait for the page to settle (the Tunarr block renders).
    await screen.findByLabelText("Library URL");
    expect(screen.queryByRole("button", { name: /connect tunarr to the guide/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /wire tunarr to your library/i })).not.toBeInTheDocument();
  });

  it("saving a connection PATCHes settings and never calls a connect endpoint from the FE", async () => {
    const { patches, connects } = stubWiring();
    renderAt("/settings/connections");

    await userEvent.type(await screen.findByLabelText("Library URL"), "9");
    await userEvent.click(await screen.findByRole("button", { name: /save changes/i }));

    // The FE only saves — the server does the wiring. No /v1/setup/*-connect from here.
    await expect.poll(() => patches, { message: "saving must PATCH settings" }).toHaveLength(1);
    expect(connects, "the FE must not wire directly — the BE auto-wires on save").toEqual([]);
  });
});
