import { getListCollectionsMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useLibraryCollections } from "./use-library-collections";

const wrapper = ({ children }: PropsWithChildren) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

const title = (id: string, name: string) => ({
  id,
  kind: "movie" as const,
  name,
  files: 1,
  ready: true,
  unplayable: 0,
  extraCopies: 0,
});

const titles = [
  title("90000801", "Harbour Lights"),
  title("90000802", "Paper Kites"),
  title("90000803", "Night Ferry"),
  title("90000804", "Undated"),
];

// The wiring: a collection says how many titles it holds, where it came from and who uses it,
// piling its first three; a set says how many of its movies the library has; a copy the server
// refuses rejects, so the card can say so, and one it makes refreshes the list.
describe("useLibraryCollections", () => {
  it("maps the collections and the sets for their cards", async () => {
    server.use(
      getListCollectionsMockHandler({
        collections: [
          {
            id: "c1",
            name: "Rainy Sundays",
            description: "",
            importedFrom: "emby",
            titles,
            usedBy: [{ id: "90000901", name: "Saturday Morning", number: 4 }],
          },
        ],
        sets: [
          { id: "90000611", name: "The Lantern Keeper Collection", total: 3, titles: titles.slice(0, 2) },
        ],
      }),
    );
    const { result } = renderHook(() => useLibraryCollections(), { wrapper });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.collections).toEqual([
      expect.objectContaining({
        id: "c1",
        titles: 4,
        importedFrom: "Emby",
        usedBy: [{ name: "Saturday Morning", number: 4 }],
      }),
    ]);
    expect(result.current.collections[0]?.pile.map((p) => p.name)).toEqual([
      "Harbour Lights",
      "Paper Kites",
      "Night Ferry",
    ]);
    expect(result.current.sets).toEqual([expect.objectContaining({ id: "90000611", have: 2, total: 3 })]);
  });

  it("rejects a copy the server refuses, and refreshes after one it makes", async () => {
    let lists = 0;
    let copied = false;
    server.use(
      http.get("*/v1/collections", () => {
        lists++;
        return HttpResponse.json({ collections: [], sets: [] });
      }),
      http.post("*/v1/collections/sets/:id/copy", () =>
        copied
          ? HttpResponse.json(
              { id: "c9", name: "Copy", description: "", titles: [], usedBy: [] },
              { status: 201 },
            )
          : HttpResponse.json(
              { title: "Not Found", status: 404 },
              { status: 404, headers: { "Content-Type": "application/problem+json" } },
            ),
      ),
    );
    const { result } = renderHook(() => useLibraryCollections(), { wrapper });
    await waitFor(() => expect(result.current.loaded).toBe(true));

    await expect(result.current.onCopy("90000611")).rejects.toThrow();
    copied = true;
    const before = lists;
    await act(() => result.current.onCopy("90000611"));
    await waitFor(() => expect(lists).toBeGreaterThan(before));
  });
});
