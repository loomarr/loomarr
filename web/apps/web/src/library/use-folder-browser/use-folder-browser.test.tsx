import type { BrowseLibraryFoldersOutputBody } from "@loomarr/api/models/browseLibraryFoldersOutputBody";
import { getBrowseLibraryFoldersMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";
import { LoomarrEventsProvider } from "@/events/events-provider";
import { FakeEventSource } from "@/test/fake-event-source";
import { server } from "@/test/msw/server";
import { useFolderBrowser } from "./use-folder-browser";

const wrapper = ({ children }: PropsWithChildren) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <LoomarrEventsProvider>{children}</LoomarrEventsProvider>
  </QueryClientProvider>
);

const listing = (path: string, names: string[]): BrowseLibraryFoldersOutputBody => ({
  path,
  parent: "",
  inUse: false,
  holds: [],
  folders: names.map((name) => ({ name, path: `${path}/${name}`, holds: [], inUse: false, named: false })),
});

// The folder browser lists the folder open now and fills each row's count from library_folder
// frames: nothing polls, and a slow share keeps the last folder listed while the next opens.
describe("useFolderBrowser", () => {
  it("lists nothing until it is opened", () => {
    const { result } = renderHook(() => useFolderBrowser(), { wrapper });
    expect(result.current.listing).toBeUndefined();
    expect(result.current.opening).toBeUndefined();
  });

  it("fills a folder's count from its frame", async () => {
    server.use(getBrowseLibraryFoldersMockHandler(listing("/media", ["Shows", "Films"])));
    const { result } = renderHook(() => useFolderBrowser(), { wrapper });
    act(() => result.current.onOpen("/media"));
    await waitFor(() => expect(result.current.listing?.path).toBe("/media"));
    expect(result.current.listing?.folders[0]?.summary).toBeUndefined();

    act(() =>
      FakeEventSource.broadcast("library_folder", {
        path: "/media/Shows",
        folders: 12,
        videos: 3,
        state: "counted",
      }),
    );
    await waitFor(() =>
      expect(result.current.listing?.folders[0]?.summary).toEqual({
        folders: 12,
        videos: 3,
        state: "counted",
      }),
    );
    // Only the folder the frame names.
    expect(result.current.listing?.folders[1]?.summary).toBeUndefined();
  });

  it("keeps the last folder listed while the next opens, and names the one opening", async () => {
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get("*/v1/library/browse", async ({ request }) => {
        const path = new URL(request.url).searchParams.get("path") ?? "";
        if (path === "/media/Shows") await held;
        return HttpResponse.json(listing(path, path === "/media" ? ["Shows"] : ["Night Ferry"]));
      }),
    );
    const { result } = renderHook(() => useFolderBrowser(), { wrapper });
    act(() => result.current.onOpen("/media"));
    await waitFor(() => expect(result.current.listing?.path).toBe("/media"));

    act(() => result.current.onOpen("/media/Shows"));
    await waitFor(() => expect(result.current.opening).toBe("/media/Shows"));
    expect(result.current.listing?.path).toBe("/media");

    release();
    await waitFor(() => expect(result.current.listing?.path).toBe("/media/Shows"));
    expect(result.current.opening).toBeUndefined();
  });

  it("says why a folder couldn't be opened", async () => {
    server.use(
      http.get("*/v1/library/browse", () =>
        HttpResponse.json(
          { title: "Not Found", status: 404, detail: "That folder isn't on this machine." },
          { status: 404 },
        ),
      ),
    );
    const { result } = renderHook(() => useFolderBrowser(), { wrapper });
    act(() => result.current.onOpen("/gone"));
    await waitFor(() => expect(result.current.error).toBe("Not Found. That folder isn't on this machine."));
  });
});
