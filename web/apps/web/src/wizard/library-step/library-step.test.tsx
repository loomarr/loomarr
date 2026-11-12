import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import {
  getLibraryStatusMockHandler,
  getListLibraryItemsMockHandler,
  getListLibraryRootsMockHandler,
  getSettingsListMockHandler,
  getSetupStatusMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { FakeEventSource } from "@/test/fake-event-source";
import { libraryStatus } from "@/test/fixtures/library";
import { setting } from "@/test/fixtures/settings";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { LibraryStep } from "./library-step";

// Sonarr and Radarr both set, and Loomarr's address: nothing for Seerr to fill, nothing to ask.
const CONNECTED: SettingEntry[] = [
  setting({ key: "sonarr.url", group: "connections.sonarr", value: "http://sonarr.invalid:8989" }),
  setting({ key: "radarr.url", group: "connections.radarr", value: "http://radarr.invalid:7878" }),
  setting({ key: "server.public_url", group: "server", value: "http://loomarr.invalid:8080" }),
];

const renderStep = (settings: SettingEntry[] = CONNECTED) => {
  server.use(
    getSettingsListMockHandler({ settings, features: {} }),
    getSetupStatusMockHandler({ checks: [] }),
  );
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <LibraryStep />
    </QueryClientProvider>,
  );
};

const MOVIES = {
  path: "/media/movies",
  inUse: false,
  readable: true,
  empty: false,
  addedByHand: false,
  foundIn: [{ authority: "radarr" as const, path: "/movies" }],
};

// The wiring: the step shows the folders the server found and where, and Use this folder records
// the choice; once one is in use, the import shows without holding the step.
describe("LibraryStep", () => {
  it("uses a folder the server found", async () => {
    let chosen: unknown;
    server.use(
      getListLibraryRootsMockHandler({
        roots: [MOVIES],
        pathMap: [
          { authority: "radarr", authorityPath: "/movies", localPath: "/media/movies", byHand: false },
        ],
      }),
      getLibraryStatusMockHandler(libraryStatus({ health: "needs_folders", inUse: 0 })),
      http.put("*/v1/library/roots/in-use", async ({ request }) => {
        chosen = await request.json();
        return HttpResponse.json({ roots: [{ ...MOVIES, inUse: true }], pathMap: [] });
      }),
    );
    renderStep();

    expect(await screen.findByText(/We found these through Radarr/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Use this folder" }));
    expect(chosen).toEqual({ path: "/media/movies", inUse: true });
  });

  it("shows the import going without holding the step", async () => {
    server.use(
      getListLibraryRootsMockHandler({ roots: [{ ...MOVIES, inUse: true }], pathMap: [] }),
      getLibraryStatusMockHandler(
        libraryStatus({
          scan: {
            enabled: true,
            step: "reading",
            stepNumber: 2,
            done: 10,
            total: 40,
            paused: false,
            quiet: false,
            arrIncomplete: false,
          },
        }),
      ),
      getListLibraryItemsMockHandler({
        items: [
          {
            id: "90000961",
            kind: "movie",
            name: "Quiet Harbor",
            files: 1,
            ready: true,
            unplayable: 0,
            extraCopies: 0,
          },
        ],
        total: 1,
      }),
    );
    renderStep();

    expect(await screen.findByText("You can carry on — titles appear as they're ready.")).toBeInTheDocument();
    expect(await screen.findByText("Ready so far")).toBeInTheDocument();
    expect(screen.getByText("Quiet Harbor")).toBeInTheDocument();
    expect(screen.getByText("Step 2 of 3 · Reading each file · 10 of 40")).toBeInTheDocument();

    // The step opens its own event stream, and each library_scan frame moves the progress.
    const stream = FakeEventSource.instances.at(-1);
    if (!stream) throw new Error("the step opened no event stream");
    act(() =>
      stream.emit("library_scan", {
        step: "reading",
        stepNumber: 2,
        done: 30,
        total: 40,
        current: "Quiet Harbor (2001).mkv",
        etaSeconds: 120,
        paused: false,
        quiet: false,
      }),
    );
    expect(await screen.findByText("Step 2 of 3 · Reading each file · 30 of 40")).toBeInTheDocument();
    expect(screen.getByText("About 2 minutes left in this step")).toBeInTheDocument();
    expect(screen.getByText("Quiet Harbor (2001).mkv")).toBeInTheDocument();
  });

  // Sonarr without an address: the step asks Seerr once, says what it filled in, and, with no
  // address of Loomarr's own yet, asks for the one the arrs report new downloads to.
  it("fills Sonarr in from Seerr and asks for Loomarr's address", async () => {
    let asked = 0;
    let settings: SettingEntry[] = [
      setting({ key: "sonarr.url", group: "connections.sonarr", set: false }),
      setting({ key: "radarr.url", group: "connections.radarr", value: "http://radarr.invalid:7878" }),
      setting({ key: "server.public_url", group: "server", set: false }),
    ];
    server.use(
      getListLibraryRootsMockHandler({ roots: [MOVIES], pathMap: [] }),
      getLibraryStatusMockHandler(libraryStatus({ health: "needs_folders", inUse: 0 })),
      http.post("*/v1/library/roots/discover", () => HttpResponse.json({ roots: [MOVIES], pathMap: [] })),
      http.post("*/v1/setup/arrs-from-requester", () => {
        asked++;
        settings = [
          setting({ key: "sonarr.url", group: "connections.sonarr", value: "http://sonarr:8989" }),
          ...settings.slice(1),
        ];
        return HttpResponse.json({
          arrs: [
            { app: "sonarr", url: "http://sonarr:8989", filled: true, reachable: true },
            { app: "radarr", url: "http://radarr.invalid:7878", filled: false, reachable: false },
          ],
        });
      }),
    );
    renderStep(settings);
    server.use(http.get("*/v1/settings", () => HttpResponse.json({ settings, features: {} })));

    expect(await screen.findByText(/Filled in from Seerr: Sonarr\./)).toBeInTheDocument();
    expect(await screen.findByText(/tell Loomarr about new downloads at this address/)).toBeInTheDocument();
    expect(asked).toBe(1);
  });
});
