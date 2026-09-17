import type { FillerSourceDTO } from "@loomarr/api";
import {
  getAddFillerSourceMockHandler,
  getCleanupFillerStorageMockHandler,
  getDiscoverFillerMockHandler,
  getDiscoverFillerStatsMockHandler,
  getFetchFillerSourceMockHandler,
  getFillerReadinessMockHandler,
  getGetFillerAcquisitionMockHandler,
  getListFillerSourcesMockHandler,
  getLocationsSearchMockHandler,
  getMeMockHandler,
  getPreviewFillerStorageCleanupMockHandler,
  getQueueFillerSourceItemMockHandler,
  getResolveFillerSourceMockHandler,
  getSetFillerProviderEnabledMockHandler,
  getSetFillerSourceEnabledMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { SourcesPanel } from "./sources-panel";

// SourcesPanel owns the Sources tab's own state and mutations — the switches, the add form, the
// per-source search. It was extracted out of `filler-page.tsx` when that file reached ~1100 lines
// and every keystroke in the source-search box re-rendered the whole clip grid.
//
// ⚠ `sources`/`total` arrive as PROPS rather than from a query here: the page header's status
// line reads the same list, and a second `useListFillerSources` inside this panel would
// double-fetch it. These tests pass them directly, which is also what makes them fast.

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// Records method + url + parsed body, so a test can prove WHAT was sent rather than only that
// something was.
// ⚠ `/v1/auth/me` must answer with an ADMIN — note the path, it is not `/v1/me`. The panel reads
// `useAuth()`, and the add form, the switches and the per-source search are all admin-only, so a
// stub that misses this route renders a read-only list and every admin assertion fails for the
// wrong reason.
// ⚠ The catch-all answered EVERY non-`me` request with `{ sources: [], total: 0, results: [] }` —
// a UNION of three different endpoints' shapes, merged so that whichever one asked would find
// something plausible. That is the strongest possible form of the wrong-shape trap: it cannot fail,
// because it is pre-satisfied for every caller.
//
// ⚠ The `me` fixture also omitted `local`, which MeBody requires. `me()` carries it.
const stubSources = () => {
  const adds: unknown[] = [];
  const enables: { id: string; body: unknown }[] = [];
  const providers: { kind: string; body: unknown }[] = [];
  const discoveries: URL[] = [];
  const fetches: URL[] = [];
  const resolutions: { kind: string; input: string }[] = [];
  const sourceItems: { sourceId: string; body: unknown }[] = [];
  server.use(
    getMeMockHandler(me({ name: "Admin" })),
    getFillerReadinessMockHandler({
      ready: true,
      nextAction: "none",
      repairs: { count: 0 },
      fetch: { enabled: true, catalogClips: 0 },
      storage: {
        automatic: true,
        state: "healthy",
        totalBytes: 500 * 1024 ** 3,
        freeBytes: 200 * 1024 ** 3,
        managedBytes: 0,
        reservedBytes: 0,
        filesystemReservedBytes: 0,
        softBudgetBytes: 20 * 1024 ** 3,
        hardReserveBytes: 10 * 1024 ** 3,
        availableBytes: 20 * 1024 ** 3,
      },
      pipeline: {
        runnable: 0,
        scheduled: 0,
        inProgress: 0,
        needsDecision: 0,
        recoverable: 0,
        ready: 0,
        complete: 0,
        rejected: 0,
        dismissed: 0,
      },
      pool: { clips: 0, breakBody: 0, eligible: 0, untagged: 0, channels: [] },
      acquisitions: [],
    }),
    getAddFillerSourceMockHandler(async ({ request }) => {
      adds.push(await request.json());
      // ⚠ The add returns a RemoteSourceDTO — `{ id, label, uri, enabled }` — not a bare id.
      return { id: "src-new", label: "New source", uri: "/mnt/extra-ads", enabled: true };
    }),
    getSetFillerSourceEnabledMockHandler(async ({ request, params }) => {
      enables.push({ id: String(params.id), body: await request.json() });
      return { id: String(params.id), enabled: false };
    }),
    getSetFillerProviderEnabledMockHandler(async ({ request, params }) => {
      providers.push({ kind: String(params.kind), body: await request.json() });
      return { kind: String(params.kind), enabled: false };
    }),
    getListFillerSourcesMockHandler({ sources: [], total: 0 }),
    getDiscoverFillerMockHandler(({ request }) => {
      discoveries.push(new URL(request.url));
      return { items: [], total: 0, licenceNote: "Check licences." };
    }),
    getDiscoverFillerStatsMockHandler({ stats: {} }),
    getLocationsSearchMockHandler({
      locations: [
        {
          id: "6167865",
          label: "Toronto, Canada",
          country: "CA",
          market: "Toronto",
          region: "Ontario",
        },
      ],
    }),
    getFetchFillerSourceMockHandler(({ request }) => {
      fetches.push(new URL(request.url));
      const sourceId = new URL(request.url).searchParams.get("id") ?? "";
      return {
        sourceId,
        sourcesPolled: 1,
        queued: 2,
        skipped: 0,
        maxPerCheck: 10,
        total: 0,
        added: 0,
        updated: 0,
        pruned: 0,
      };
    }),
    getQueueFillerSourceItemMockHandler(async ({ request, params }) => {
      sourceItems.push({ sourceId: String(params.id), body: await request.json() });
      return { jobId: "acq-found-clip" };
    }),
    getGetFillerAcquisitionMockHandler({
      id: "acq-found-clip",
      trigger: "source",
      sourceId: "archive:classic",
      status: "queued",
      requested: 1,
      fetched: 0,
      skipped: 0,
      failed: 0,
      empty: 0,
      startedAt: "2026-09-13T14:00:00Z",
      updatedAt: "2026-09-13T14:00:00Z",
      outcome: {
        enrolled: 0,
        preparing: 0,
        needsDecision: 0,
        ready: 0,
        complete: 0,
        rejected: 0,
        dismissed: 0,
      },
      artifacts: { staged: 0, published: 0, consumed: 0, repair: 0 },
    }),
    getResolveFillerSourceMockHandler(async ({ request, params }) => {
      const body = (await request.json()) as { input: string };
      const kind = String(params.kind);
      resolutions.push({ kind, input: body.input });
      if (kind === "youtube") {
        return {
          provider: "youtube",
          targetType: "channel",
          canonicalId: "UC-retro",
          canonicalUrl: body.input,
          title: "Retro Ads",
          alreadyAdded: true,
          previewItems: [
            { title: "Local commercial break", url: "https://www.youtube.com/watch?v=video-one" },
          ],
        };
      }
      return {
        provider: "archive",
        targetType: "collection",
        canonicalId: body.input,
        canonicalUrl: `https://archive.org/details/${body.input}`,
        title: "Classic TV",
        alreadyAdded: true,
        previewItems: [
          { title: "First station break", url: "https://archive.org/details/station_break_one" },
          { title: "Second station break", url: "https://archive.org/details/station_break_two" },
        ],
      };
    }),
  );
  return { adds, enables, providers, discoveries, fetches, resolutions, sourceItems };
};

const source = (over: Partial<FillerSourceDTO> & Pick<FillerSourceDTO, "kind">): FillerSourceDTO => ({
  id: over.kind,
  target: "/data/filler",
  detail: "watched directly",
  count: 0,
  incoming: 0,
  configured: true,
  fetchable: true,
  enabled: true,
  effectiveEnabled: over.effectiveEnabled ?? over.enabled !== false,
  providerEnabled: true,
  switchable: true,
  removable: false,
  searchable: false,
  readiness:
    over.readiness ??
    (over.configured === false ? "not_configured" : over.enabled === false ? "off" : "ready"),
  ready: over.ready ?? (over.configured !== false && over.enabled !== false),
  locationSource: "installation",
  automaticDownloads:
    over.automaticDownloads ??
    ((over.kind === "archive" || over.kind === "youtube") && !over.group
      ? {
          mode: "defaults",
          everySeconds: 21600,
          maxPerCheck: 10,
          summary: "Uses your defaults: every 6 hours, up to 10 clips each check.",
        }
      : undefined),
  actions:
    over.actions ??
    (over.configured === false
      ? ["configure"]
      : over.enabled === false
        ? ["enable"]
        : [
            "fetch",
            "disable",
            ...(over.removable ? ["remove"] : []),
            ...(over.searchable ? ["search"] : []),
            ...(over.kind === "archive" || over.kind === "youtube" ? ["edit_location"] : []),
          ]),
  ...over,
});

const renderPanel = (sources: FillerSourceDTO[] = [source({ kind: "folder" })]) =>
  render(<SourcesPanel sources={sources} />, { wrapper: makeWrapper() });

beforeEach(() => sessionStorage.clear());

describe("SourcesPanel", () => {
  it("shows one calm server-owned storage summary", async () => {
    stubSources();
    renderPanel();

    const storage = await screen.findByRole("region", { name: "Storage" });
    expect(storage).toHaveTextContent("0 B used for filler");
    expect(storage).toHaveTextContent(/Automatic downloads will pause before this drive has less than/);
    expect(within(storage).getByRole("link", { name: "Storage options" })).toHaveAttribute(
      "href",
      "/filler/settings/storage",
    );
  });

  it("uses the server-owned approaching state without calculating a warning in the browser", async () => {
    stubSources();
    server.use(
      getFillerReadinessMockHandler({
        ready: true,
        nextAction: "none",
        repairs: { count: 0 },
        fetch: { enabled: true, catalogClips: 12 },
        storage: {
          automatic: true,
          state: "approaching",
          totalBytes: 64 * 1024 ** 3,
          freeBytes: 8 * 1024 ** 3,
          managedBytes: 5.5 * 1024 ** 3,
          reservedBytes: 0,
          filesystemReservedBytes: 0,
          softBudgetBytes: 6.4 * 1024 ** 3,
          hardReserveBytes: 6.4 * 1024 ** 3,
          availableBytes: 512 * 1024 ** 2,
        },
        pipeline: {
          runnable: 0,
          scheduled: 0,
          inProgress: 0,
          needsDecision: 0,
          recoverable: 0,
          ready: 12,
          complete: 0,
          rejected: 0,
          dismissed: 0,
        },
        pool: { clips: 12, breakBody: 10, eligible: 10, untagged: 0, channels: [] },
        acquisitions: [],
      }),
    );
    renderPanel();

    const storage = await screen.findByRole("region", { name: "Storage" });
    expect(storage).toHaveTextContent("512.0 MB is left for new filler");
  });

  it("previews and removes only safe old temporary downloads when the drive is low", async () => {
    stubSources();
    server.use(
      getFillerReadinessMockHandler({
        ready: false,
        nextAction: "free_disposable_space",
        repairs: { count: 0 },
        fetch: { enabled: true, catalogClips: 12 },
        storage: {
          automatic: true,
          state: "paused",
          totalBytes: 32 * 1024 ** 3,
          freeBytes: 2 * 1024 ** 3,
          managedBytes: 4 * 1024 ** 3,
          reservedBytes: 0,
          filesystemReservedBytes: 0,
          softBudgetBytes: 3.2 * 1024 ** 3,
          hardReserveBytes: 3.2 * 1024 ** 3,
          availableBytes: 0,
          pausedBy: "host_reserve",
        },
        pipeline: {
          runnable: 0,
          scheduled: 0,
          inProgress: 0,
          needsDecision: 0,
          recoverable: 0,
          ready: 12,
          complete: 0,
          rejected: 0,
          dismissed: 0,
        },
        pool: { clips: 12, breakBody: 10, eligible: 10, untagged: 0, channels: [] },
        acquisitions: [],
      }),
      getPreviewFillerStorageCleanupMockHandler({ items: 2, bytes: 2048 }),
      getCleanupFillerStorageMockHandler({
        removedItems: 2,
        removedBytes: 2048,
        failedItems: 0,
        remaining: { items: 0, bytes: 0 },
      }),
    );
    renderPanel();

    expect(await screen.findByText(/2.0 KB in 2 unfinished downloads is safe to remove/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Free 2.0 KB" }));
    expect(await screen.findByText(/Removed 2 temporary folders · freed 2.0 KB/)).toBeInTheDocument();
  });

  it("lists the sources it is given", () => {
    stubSources();
    renderPanel([
      source({ kind: "folder", id: "folder", target: "/data/filler" }),
      source({ kind: "archive", id: "archive:classic", target: "classic_tv_commercials" }),
    ]);

    expect(screen.getByText("/data/filler")).toBeInTheDocument();
    expect(screen.getByText("classic_tv_commercials")).toBeInTheDocument();
  });

  // ⚠ The kind is EXPLICIT on the wire, not sniffed from the URI. An archive identifier and a
  // YouTube playlist URL are validated by incompatible rules on the server — before the kind was
  // sent, the hardcoded archive validator rejected every playlist URL with a message about
  // archive.org collections.
  it("keeps folder and library setup in the local-source form", async () => {
    const { adds } = stubSources();
    renderPanel();

    // The add form is admin-only, so it appears only once `/v1/auth/me` has resolved.
    const local = await screen.findByRole("region", { name: "Your files" });
    const library = await within(local).findByRole("textbox", { name: "Library name" });
    await userEvent.type(library, "Commercials");
    await userEvent.click(within(local).getByRole("button", { name: /add source/i }));

    await waitFor(() => {
      expect(adds).toEqual([{ kind: "library", uri: "Commercials" }]);
    });
  });

  it("adds remote targets inside their provider without a kind selector", async () => {
    const { adds } = stubSources();
    server.use(
      getResolveFillerSourceMockHandler({
        provider: "youtube",
        targetType: "channel",
        canonicalId: "UC-retroads",
        canonicalUrl: "https://www.youtube.com/channel/UC-retroads/videos",
        title: "Retro Ads",
        alreadyAdded: false,
      }),
    );
    renderPanel([
      source({
        kind: "youtube",
        id: "provider:youtube",
        target: "YouTube",
        group: true,
        configured: false,
        fetchable: false,
        searchable: false,
      }),
    ]);

    const input = await screen.findByRole("combobox", { name: "Find a YouTube channel or playlist" });
    expect(screen.queryByRole("combobox", { name: /kind of source/i })).not.toBeInTheDocument();
    await userEvent.type(input, "https://www.youtube.com/@retroads/videos");
    await userEvent.click(await screen.findByRole("button", { name: "Add source" }));

    await waitFor(() => {
      expect(adds).toContainEqual({
        kind: "youtube",
        uri: "https://www.youtube.com/channel/UC-retroads/videos",
        label: "Retro Ads",
      });
    });
  });

  it("keeps an automatic-download save failure beside the controls", async () => {
    stubSources();
    server.use(
      http.patch("*/v1/filler/sources/:id", () =>
        HttpResponse.json(
          { title: "Could not save", detail: "The source changed. Try saving again." },
          { status: 409 },
        ),
      ),
    );
    renderPanel([source({ kind: "archive", id: "archive:classic", target: "Classic TV" })]);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV" }));
    await userEvent.click(screen.getByRole("button", { name: /source settings/i }));
    const save = screen.getByRole("button", { name: "Save automatic downloads" });
    await userEvent.click(save);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Automatic downloads couldn’t be saved. Try again.",
    );
    await waitFor(() => expect(save).toHaveFocus());
  });

  it("uses the provider command for the provider switch", async () => {
    const { providers } = stubSources();
    renderPanel([source({ kind: "archive", id: "provider:archive", target: "Archive.org", group: true })]);
    await userEvent.click(await screen.findByRole("switch", { name: "Use Archive.org" }));
    await waitFor(() => expect(providers).toEqual([{ kind: "archive", body: { enabled: false } }]));
  });

  // ⚠ THE promise the switch has to keep. Switching a source off withdraws it from future
  // scanning; it does NOT remove clips already in the catalog. An operator who reads "off" as
  // "my clips are gone" will switch it back on and re-download everything they already have.
  it("switching a source off sends only the enabled flag", async () => {
    const { enables } = stubSources();
    renderPanel([source({ kind: "folder", id: "folder", target: "/data/filler" })]);

    await userEvent.click(screen.getByRole("switch", { name: "Use /data/filler" }));

    await waitFor(() => {
      // The id is the PATH PARAM the resolver parsed, so this pins WHICH source was switched —
      // the old version asserted `patch?.url` contained a string the test had also written.
      expect(enables).toEqual([{ id: "folder", body: { enabled: false } }]);
    });
  });

  it("switching a remote source off leaves its automatic-download policy unchanged", async () => {
    const { enables } = stubSources();
    renderPanel([
      source({
        kind: "archive",
        id: "archive:classic",
        target: "Classic TV",
        automaticDownloads: {
          mode: "custom",
          everySeconds: 43200,
          maxPerCheck: 4,
          summary: "Every 12 hours, up to 4 clips each check.",
        },
      }),
    ]);

    await userEvent.click(screen.getByRole("switch", { name: "Use Classic TV" }));
    await waitFor(() => expect(enables).toEqual([{ id: "archive:classic", body: { enabled: false } }]));
  });

  // Search belongs to the selected source workspace. The panel reads the server's `searchable`
  // flag rather than guessing from the provider kind.
  it("offers search only in a searchable source's workspace", async () => {
    stubSources();
    const { unmount } = renderPanel([
      source({ kind: "archive", id: "a", target: "classic_tv", searchable: true }),
    ]);
    await userEvent.click(screen.getByRole("button", { name: "Manage classic_tv" }));
    expect(await screen.findByRole("dialog", { name: "classic_tv" })).toHaveTextContent(
      "Find a specific clip",
    );
    expect(screen.queryByRole("textbox", { name: "Search this source" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    expect(screen.getByRole("textbox", { name: "Search this source" })).toBeInTheDocument();
    unmount();

    renderPanel([source({ kind: "youtube", id: "y", target: "vintage_ads", searchable: false })]);
    await userEvent.click(screen.getByRole("button", { name: "Manage vintage_ads" }));
    expect(await screen.findByRole("dialog", { name: "vintage_ads" })).not.toHaveTextContent(
      "Find a specific clip",
    );
  });

  it("explains that new sources follow the installation location", async () => {
    stubSources();
    renderPanel();
    expect(await screen.findByText(/uses your location/i)).toBeInTheDocument();
    expect(screen.getByText(/every clip is checked before it can play/i)).toBeInTheDocument();
  });

  it("keeps registered rows compact and opens one coherent source workspace", async () => {
    stubSources();
    renderPanel([
      source({
        kind: "archive",
        id: "archive:classic",
        uri: "classic_tv",
        target: "Classic TV",
        searchable: true,
        removable: true,
      }),
    ]);

    expect(screen.queryByRole("button", { name: "Check now" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Advanced" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Browse clips" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV" }));
    const workspace = await screen.findByRole("dialog", { name: "Classic TV" });
    expect(workspace).toHaveTextContent("Look for new clips");
    expect(workspace).toHaveTextContent("Find a specific clip");
    expect(screen.queryByRole("textbox", { name: "Search this source" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    expect(screen.getByRole("textbox", { name: "Search this source" })).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Location" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /source settings/i }));
    expect(screen.getByRole("combobox", { name: "Location" })).toBeInTheDocument();
  });

  it("links a source's held clips to Incoming instead of reporting an empty source", async () => {
    stubSources();
    renderPanel([
      source({
        kind: "archive",
        id: "archive:tv_ads",
        uri: "tv_ads",
        target: "TV Ads",
        count: 0,
        incoming: 22,
      }),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Manage TV Ads" }));
    const workspace = await screen.findByRole("dialog", { name: "TV Ads" });
    expect(workspace).toHaveTextContent("0 ready");
    expect(within(workspace).getByRole("link", { name: "22 being checked" })).toHaveAttribute(
      "href",
      "/filler/incoming",
    );
  });

  it("opens secondary tools from the visible section rows, not only their chevrons", async () => {
    stubSources();
    renderPanel([
      source({
        kind: "archive",
        id: "archive:classic",
        uri: "classic_tv",
        target: "Classic TV",
        searchable: true,
        removable: true,
      }),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV" }));

    const findRow = screen.getByText("Find a specific clip");
    expect(findRow.closest("button")).not.toBeNull();
    await userEvent.click(findRow);
    expect(screen.getByRole("textbox", { name: "Search this source" })).toBeInTheDocument();

    const settingsRow = screen.getByText("Source settings");
    expect(settingsRow.closest("button")).not.toBeNull();
    await userEvent.click(settingsRow);
    expect(screen.getByRole("combobox", { name: "Location" })).toBeInTheDocument();
  });

  it("keeps a local folder’s path and latest check in its workspace", async () => {
    stubSources();
    renderPanel([
      source({
        kind: "folder",
        id: "folder",
        uri: "/data/filler/commercials",
        target: "/data/filler/commercials",
        lastCheckedAt: "2026-09-13T12:00:00Z",
      }),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Manage Drop folder" }));
    const workspace = await screen.findByRole("dialog", { name: "Drop folder" });
    expect(workspace).toHaveTextContent("/data/filler/commercials");
    expect(workspace).toHaveTextContent(/last checked/i);
  });

  it("previews an existing remote source from its saved target without searching", async () => {
    const { resolutions } = stubSources();
    renderPanel([
      source({
        kind: "archive",
        id: "archive:classic",
        uri: "classic_tv",
        target: "Classic TV",
        searchable: true,
      }),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV" }));
    const workspace = await screen.findByRole("dialog", { name: "Classic TV" });
    expect(await screen.findByRole("link", { name: "First station break" })).toBeInTheDocument();
    expect(resolutions).toEqual([{ kind: "archive", input: "classic_tv" }]);
    expect(workspace).not.toHaveTextContent("Search collections or paste");

    await userEvent.click(screen.getAllByRole("button", { name: "Preview" })[0]!);
    expect(screen.getByTitle("Preview First station break")).toHaveAttribute(
      "src",
      "https://archive.org/embed/station_break_one?autoplay=1",
    );
  });

  it("keeps a source-specific location and removal under Source settings", async () => {
    const { enables } = stubSources();
    renderPanel([
      source({
        kind: "archive",
        id: "archive:classic",
        target: "Classic TV",
        removable: true,
      }),
    ]);

    expect(screen.queryByRole("combobox", { name: "Location" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /remove classic tv/i })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV" }));
    await userEvent.click(screen.getByRole("button", { name: /source settings/i }));
    const location = screen.getByRole("combobox", { name: "Location" });
    await userEvent.clear(location);
    await userEvent.type(location, "Toronto");
    await userEvent.click(await screen.findByRole("option", { name: "Toronto, Canada" }));
    await userEvent.click(screen.getByRole("button", { name: "Save different area" }));

    await waitFor(() => {
      expect(enables).toEqual([
        {
          id: "archive:classic",
          body: { enabled: true, geography: { country: "CA", market: "Toronto" } },
        },
      ]);
    });
    expect(screen.getByRole("button", { name: /remove classic tv/i })).toBeInTheDocument();
  });

  it("keeps automatic downloads simple until a source chooses a different schedule", async () => {
    const { enables } = stubSources();
    renderPanel([
      source({
        kind: "archive",
        id: "archive:classic",
        target: "Classic TV",
        automaticDownloads: {
          mode: "defaults",
          everySeconds: 21600,
          maxPerCheck: 10,
          summary: "Uses your defaults: every 6 hours, up to 10 clips each check.",
        },
      }),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV" }));
    await userEvent.click(screen.getByRole("button", { name: /source settings/i }));
    expect(
      screen.getByText("Uses your defaults: every 6 hours, up to 10 clips each check."),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Source check interval")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("combobox", { name: "Automatic downloads for this source" }));
    await userEvent.click(screen.getByRole("option", { name: "Use a different schedule" }));
    await userEvent.click(screen.getByRole("combobox", { name: "Source check schedule" }));
    await userEvent.click(await screen.findByRole("option", { name: "Custom" }));
    fireEvent.change(screen.getByLabelText("Source check interval"), { target: { value: "12" } });
    fireEvent.change(screen.getByLabelText("Clips per source check"), { target: { value: "3" } });
    await userEvent.click(screen.getByRole("button", { name: "Save automatic downloads" }));

    await waitFor(() => {
      expect(enables).toContainEqual({
        id: "archive:classic",
        body: {
          enabled: true,
          automaticDownloads: { mode: "custom", everySeconds: 43200, maxPerCheck: 3 },
        },
      });
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Save automatic downloads" })).toHaveFocus(),
    );
  });

  it("uses the installation location again when a source exception is cleared", async () => {
    const { enables } = stubSources();
    renderPanel([
      source({
        kind: "archive",
        id: "archive:classic",
        target: "Classic TV",
        country: "CA",
        market: "Toronto",
        effectiveCountry: "CA",
        effectiveMarket: "Toronto",
        locationSource: "source",
      }),
    ]);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV" }));
    await userEvent.click(screen.getByRole("button", { name: /source settings/i }));
    expect(screen.getByRole("combobox", { name: "Location" })).toHaveValue("Toronto, Canada");
    await userEvent.click(screen.getByRole("button", { name: "Use my location" }));

    await waitFor(() => {
      expect(enables).toEqual([
        {
          id: "archive:classic",
          body: { enabled: true, geography: { country: "", market: "" } },
        },
      ]);
    });
  });

  it("fetches only the source row the operator selected", async () => {
    const { fetches } = stubSources();
    renderPanel([source({ kind: "archive", id: "archive:classic", target: "Classic TV" })]);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV" }));
    await userEvent.click(screen.getByRole("button", { name: "Look for new clips" }));

    await waitFor(() => expect(fetches).toHaveLength(1));
    expect(fetches[0]?.searchParams.get("id")).toBe("archive:classic");
    expect(await screen.findByRole("status")).toHaveTextContent("2 new clips were queued from Classic TV.");
  });
});

// The selected source owns the one open workspace and therefore the one search context (§10 V54 B6).
describe("SourcesPanel per-source search", () => {
  const twoCollections = [
    source({
      kind: "archive",
      id: "archive:classic",
      uri: "classic_tv",
      target: "Classic TV Commercials",
      searchable: true,
    }),
    source({
      kind: "archive",
      id: "archive:psas",
      uri: "vintage_psas",
      target: "Vintage PSAs",
      searchable: true,
    }),
  ];

  it("scopes a row search to that source's collection URI", async () => {
    const { discoveries } = stubSources();
    renderPanel(twoCollections);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV Commercials" }));
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    await userEvent.type(screen.getByRole("textbox", { name: /search this source/i }), "cereal");
    await userEvent.click(screen.getByRole("button", { name: /^search$/i }));

    await waitFor(() => expect(discoveries).toHaveLength(1));
    expect(discoveries[0]?.searchParams.get("q")).toBe("cereal");
    expect(discoveries[0]?.searchParams.get("collection")).toBe("classic_tv");
  });

  it("queues a found clip under its registered parent and shows the durable download state", async () => {
    const { sourceItems } = stubSources();
    server.use(
      getDiscoverFillerMockHandler({
        items: [
          {
            id: "CampbellsSoupAdvert",
            title: "Campbell's Soup",
            url: "https://archive.org/details/CampbellsSoupAdvert",
          },
        ],
        total: 1,
        licenceNote: "Check licences.",
      }),
    );
    renderPanel(twoCollections);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV Commercials" }));
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    await userEvent.type(screen.getByRole("textbox", { name: /search this source/i }), "soup");
    await userEvent.click(screen.getByRole("button", { name: /^search$/i }));
    await userEvent.click(await screen.findByRole("button", { name: /queue download: campbell's soup/i }));

    await waitFor(() => {
      expect(sourceItems).toEqual([
        {
          sourceId: "archive:classic",
          body: {
            remoteId: "CampbellsSoupAdvert",
            url: "https://archive.org/details/CampbellsSoupAdvert",
          },
        },
      ]);
    });
    expect(await screen.findByText("Downloading…")).toBeInTheDocument();
    expect(screen.queryByText("queued")).not.toBeInTheDocument();
  });

  it("replaces an accepted queue state with a recoverable background failure", async () => {
    stubSources();
    server.use(
      getDiscoverFillerMockHandler({
        items: [
          {
            id: "CampbellsSoupAdvert",
            title: "Campbell's Soup",
            url: "https://archive.org/details/CampbellsSoupAdvert",
          },
        ],
        total: 1,
        licenceNote: "Check licences.",
      }),
      getGetFillerAcquisitionMockHandler({
        id: "acq-found-clip",
        trigger: "source",
        sourceId: "archive:classic",
        status: "error",
        requested: 1,
        fetched: 0,
        skipped: 0,
        failed: 1,
        empty: 0,
        error: "Archive.org timed out",
        startedAt: "2026-09-13T14:00:00Z",
        completedAt: "2026-09-13T14:01:00Z",
        updatedAt: "2026-09-13T14:01:00Z",
        outcome: {
          enrolled: 0,
          preparing: 0,
          needsDecision: 0,
          ready: 0,
          complete: 0,
          rejected: 0,
          dismissed: 0,
        },
        artifacts: { staged: 0, published: 0, consumed: 0, repair: 0 },
      }),
    );
    renderPanel(twoCollections);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV Commercials" }));
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    await userEvent.type(screen.getByRole("textbox", { name: /search this source/i }), "soup");
    await userEvent.click(screen.getByRole("button", { name: /^search$/i }));
    await userEvent.click(await screen.findByRole("button", { name: /queue download: campbell's soup/i }));

    expect(await screen.findByText("Couldn’t add")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.queryByText("Downloading…")).not.toBeInTheDocument();
  });

  it("restores a found clip's durable download state after the Sources panel remounts", async () => {
    const { sourceItems } = stubSources();
    server.use(
      getDiscoverFillerMockHandler({
        items: [
          {
            id: "CampbellsSoupAdvert",
            title: "Campbell's Soup",
            url: "https://archive.org/details/CampbellsSoupAdvert",
          },
        ],
        total: 1,
        licenceNote: "Check licences.",
      }),
    );
    const first = renderPanel(twoCollections);
    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV Commercials" }));
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    await userEvent.type(screen.getByRole("textbox", { name: /search this source/i }), "soup");
    await userEvent.click(screen.getByRole("button", { name: /^search$/i }));
    await userEvent.click(await screen.findByRole("button", { name: /queue download: campbell's soup/i }));
    expect(await screen.findByText("Downloading…")).toBeInTheDocument();
    first.unmount();

    renderPanel(twoCollections);
    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV Commercials" }));
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    await userEvent.type(screen.getByRole("textbox", { name: /search this source/i }), "soup");
    await userEvent.click(screen.getByRole("button", { name: /^search$/i }));

    expect(await screen.findByText("Downloading…")).toBeInTheDocument();
    expect(sourceItems).toHaveLength(1);
  });

  it("opens one workspace for the row that was clicked", async () => {
    stubSources();
    renderPanel(twoCollections);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV Commercials" }));

    expect(await screen.findByRole("dialog", { name: "Classic TV Commercials" })).toBeInTheDocument();
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
  });

  // One panel at a time is what lets the query state stay single — and switching rows must not
  // leave the previous row's answers sitting under this row's name.
  it("resets search when a different source is opened", async () => {
    stubSources();
    renderPanel(twoCollections);

    await userEvent.click(screen.getByRole("button", { name: "Manage Classic TV Commercials" }));
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    await userEvent.type(screen.getByRole("textbox", { name: /search this source/i }), "cereal");
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("button", { name: "Manage Vintage PSAs" }));

    expect(await screen.findByRole("dialog", { name: "Vintage PSAs" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: /search this source/i })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /finding a specific clip/i }));
    expect(screen.getByRole("textbox", { name: /search this source/i })).toHaveValue("");
  });

  it("closes the workspace and returns focus to its source row", async () => {
    stubSources();
    renderPanel(twoCollections);

    const trigger = screen.getByRole("button", { name: "Manage Classic TV Commercials" });
    await userEvent.click(trigger);
    await userEvent.click(screen.getByRole("button", { name: "Close" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(trigger).toHaveFocus();
  });
});
