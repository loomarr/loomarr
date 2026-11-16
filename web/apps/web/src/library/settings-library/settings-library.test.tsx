import {
  getHouseholdMockHandler,
  getLibraryStatusMockHandler,
  getListLibraryRootsMockHandler,
} from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it } from "vitest";
import { libraryStatus } from "@/test/fixtures/library";
import { server } from "@/test/msw/server";
import { SettingsLibrary } from "./settings-library";

const renderPage = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <LoomarrProvider theme="dark">
        <SettingsLibrary />
      </LoomarrProvider>
    </QueryClientProvider>,
  );
};

const inUse = {
  path: "/media/movies",
  inUse: true,
  readable: true,
  empty: false,
  addedByHand: false,
  checkedAt: "2026-07-25T19:55:00Z",
  foundIn: [{ authority: "radarr" as const, path: "/media/movies" }],
};

// The wiring: the page reads the folders and status, looks for folders by itself once when none
// were ever recorded, and shows the server's refusal of a typed folder in its own words.
describe("SettingsLibrary", () => {
  beforeEach(() => {
    // The page tells times in the household's zone, which the household read carries.
    server.use(getHouseholdMockHandler({ timeZone: "" }));
    // Add a folder opens the folder browser at the top: nothing in it unless a test says so.
    server.use(
      http.get("*/v1/library/browse", () =>
        HttpResponse.json({ path: "", parent: "", inUse: false, holds: [], folders: [] }),
      ),
    );
  });

  it("looks for folders once when none were ever recorded", async () => {
    let discoveries = 0;
    server.use(
      getListLibraryRootsMockHandler({ roots: [], pathMap: [] }),
      getLibraryStatusMockHandler(libraryStatus({ health: "needs_folders", inUse: 0 })),
      http.post("*/v1/library/roots/discover", () => {
        discoveries += 1;
        return HttpResponse.json({ roots: [], pathMap: [], sources: [] });
      }),
    );
    renderPage();

    await waitFor(() => expect(discoveries).toBe(1));
    // The refetch after discovery still finds nothing; the page must not ask again.
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(discoveries).toBe(1);
  });

  it("does not look for folders when some are recorded", async () => {
    let discoveries = 0;
    server.use(
      getListLibraryRootsMockHandler({ roots: [inUse], pathMap: [] }),
      getLibraryStatusMockHandler(libraryStatus()),
      http.post("*/v1/library/roots/discover", () => {
        discoveries += 1;
        return HttpResponse.json({ roots: [], pathMap: [], sources: [] });
      }),
    );
    renderPage();

    expect(await screen.findByLabelText("Movies")).toBeInTheDocument();
    expect(discoveries).toBe(0);
  });

  it("sends the maintainer's answer to a held removal for that folder", async () => {
    let confirmed: unknown;
    server.use(
      getListLibraryRootsMockHandler({ roots: [inUse], pathMap: [] }),
      getLibraryStatusMockHandler(
        libraryStatus({
          held: [
            {
              path: "/media/movies",
              movies: 3,
              episodes: 0,
              other: 0,
              titles: ["Amber Hollow", "Brightwater"],
              titleCount: 3,
            },
          ],
        }),
      ),
      http.post("*/v1/library/roots/confirm-removal", async ({ request }) => {
        confirmed = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage();

    // The maintainer sees what they would let go before saying it.
    expect(await screen.findByText(/That's Amber Hollow, Brightwater and 1 more\./)).toBeInTheDocument();

    await userEvent.click(await screen.findByRole("button", { name: "They're gone, remove them" }));
    await waitFor(() => expect(confirmed).toEqual({ path: "/media/movies" }));
  });

  it("shows why a typed folder was refused", async () => {
    server.use(
      getListLibraryRootsMockHandler({ roots: [inUse], pathMap: [] }),
      getLibraryStatusMockHandler(libraryStatus()),
      http.post("*/v1/library/roots", () =>
        HttpResponse.json(
          { title: "Loomarr can't open this folder", detail: "Check that it is mounted", status: 422 },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    renderPage();

    await userEvent.click(await screen.findByRole("button", { name: "Add a folder" }));
    await userEvent.click(await screen.findByRole("button", { name: "Type a path instead" }));
    await userEvent.type(screen.getByLabelText("Where is the folder?"), "/media/shows");
    await userEvent.click(screen.getByRole("button", { name: "Add folder" }));

    expect(
      await screen.findByText("Loomarr can't open this folder. Check that it is mounted"),
    ).toBeInTheDocument();
  });

  // Add a folder opens the browser at the top; opening a folder reads it, and Use this folder adds
  // the folder open now, in place of the folders in use inside it when it holds some.
  it("adds a folder picked by opening folders", async () => {
    const asked: string[] = [];
    let added: unknown;
    server.use(
      getListLibraryRootsMockHandler({ roots: [inUse], pathMap: [] }),
      getLibraryStatusMockHandler(libraryStatus()),
      http.get("*/v1/library/browse", ({ request }) => {
        const path = new URL(request.url).searchParams.get("path") ?? "";
        asked.push(path);
        return HttpResponse.json(
          path === ""
            ? {
                path: "",
                parent: "",
                inUse: false,
                holds: [],
                folders: [
                  { path: "/media", name: "/media", named: false, inUse: false, holds: ["/media/movies"] },
                ],
              }
            : { path: "/media", parent: "", inUse: false, holds: ["/media/movies"], folders: [] },
        );
      }),
      http.post("*/v1/library/roots", async ({ request }) => {
        added = await request.json();
        return HttpResponse.json({ roots: [], pathMap: [] });
      }),
    );
    renderPage();

    await userEvent.click(await screen.findByRole("button", { name: "Add a folder" }));
    await userEvent.click(await screen.findByText("/media"));
    await userEvent.click(await screen.findByRole("button", { name: "Use this folder instead" }));

    await waitFor(() => expect(added).toEqual({ path: "/media", replace: true }));
    // The top, then /media; an added folder reads the listing again, for what is in use now.
    expect(asked.slice(0, 2)).toEqual(["", "/media"]);
  });
});
