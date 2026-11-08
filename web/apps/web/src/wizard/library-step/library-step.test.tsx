import { getLibraryStatusMockHandler, getListLibraryRootsMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { libraryStatus } from "@/test/fixtures/library";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { LibraryStep } from "./library-step";

const renderStep = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <LibraryStep />
    </QueryClientProvider>,
  );

const MOVIES = {
  path: "/media/movies",
  inUse: false,
  readable: true,
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
            stage: "reading",
            headersDone: 10,
            headersTotal: 40,
            paused: false,
            arrIncomplete: false,
          },
        }),
      ),
    );
    renderStep();

    expect(await screen.findByText("You can carry on — titles appear as they're ready.")).toBeInTheDocument();
  });
});
