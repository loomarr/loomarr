import { getLibraryStatusMockHandler, getListLibraryRootsMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";
import { libraryStatus } from "@/test/fixtures/library";
import { server } from "@/test/msw/server";
import { useLibraryFolders } from "./use-library-folders";

const wrapper = ({ children }: PropsWithChildren) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

// The wiring: with no folder ever recorded it looks for them once by itself, and a folder the
// server refuses comes back as a sentence saying why.
describe("useLibraryFolders", () => {
  it("looks for folders once when none were ever recorded", async () => {
    let looks = 0;
    server.use(
      getListLibraryRootsMockHandler({ roots: [], pathMap: [] }),
      getLibraryStatusMockHandler(libraryStatus({ health: "needs_folders", inUse: 0 })),
      http.post("*/v1/library/roots/discover", () => {
        looks++;
        return HttpResponse.json({ roots: [], pathMap: [] });
      }),
    );
    const { result } = renderHook(() => useLibraryFolders(), { wrapper });

    await waitFor(() => expect(result.current.checked).toBe(true));
    expect(looks).toBe(1);
  });

  it("says why a folder was refused", async () => {
    server.use(
      getListLibraryRootsMockHandler({
        roots: [
          { path: "/media/tv", inUse: true, readable: true, empty: false, addedByHand: false, foundIn: [] },
        ],
        pathMap: [],
      }),
      getLibraryStatusMockHandler(libraryStatus()),
      http.post("*/v1/library/roots", () =>
        HttpResponse.json(
          { title: "Loomarr can't open that folder", status: 422 },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { result } = renderHook(() => useLibraryFolders(), { wrapper });
    await waitFor(() => expect(result.current.roots).toBeDefined());

    expect(await result.current.onAddFolder("/nowhere")).toContain("Loomarr can't open that folder");
  });
});
