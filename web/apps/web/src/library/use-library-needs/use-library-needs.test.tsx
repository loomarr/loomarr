import { getListLibraryNeedsMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useLibraryNeeds } from "./use-library-needs";

const wrapper = ({ children }: PropsWithChildren) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

const candidate = (tmdbId: string, name: string, nameMatches: boolean) => ({
  tmdbId,
  kind: "movie" as const,
  name,
  nameMatches,
  yearMatches: false,
  lengthMatches: false,
});

const need = (id: string, candidates: ReturnType<typeof candidate>[]) => ({
  item: { id, kind: "movie" as const, name: "Untitled", files: 1, ready: true },
  files: [{ path: `/media/movies/${id}.mkv`, size: 1, ready: true }],
  candidates,
  searched: true,
});

// The wiring: a guess is offered as the best only when its name agrees; an answer the server
// refuses rejects, so the card puts the question back; an accepted one refreshes the list.
describe("useLibraryNeeds", () => {
  it("offers the first guess as the best only when its name agrees", async () => {
    server.use(
      getListLibraryNeedsMockHandler({
        needs: [
          need("90000011", [candidate("90000101", "Alpha", true), candidate("90000102", "Alpha II", false)]),
          need("90000012", [candidate("90000103", "Bravo", false)]),
        ],
      }),
    );
    const { result } = renderHook(() => useLibraryNeeds(), { wrapper });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    const [named, unnamed] = result.current.needs;
    expect(named?.best?.tmdbId).toBe("90000101");
    expect(named?.others.map((c) => c.tmdbId)).toEqual(["90000102"]);
    expect(named?.files).toEqual(["/media/movies/90000011.mkv"]);
    expect(unnamed?.best).toBeUndefined();
    expect(unnamed?.others.map((c) => c.tmdbId)).toEqual(["90000103"]);
  });

  it("rejects an answer the server refuses, and refreshes the list after one it takes", async () => {
    let lists = 0;
    let linked: unknown;
    server.use(
      http.get("*/v1/library/needs", () => {
        lists += 1;
        return HttpResponse.json({ needs: [need("90000011", [])] });
      }),
      http.post("*/v1/library/items/:id/link", async ({ request }) => {
        linked = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
      http.post("*/v1/library/items/:id/ignore", () =>
        HttpResponse.json(
          { title: "Boom", status: 500 },
          { status: 500, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { result } = renderHook(() => useLibraryNeeds(), { wrapper });
    await waitFor(() => expect(lists).toBe(1));

    await expect(result.current.answers.onIgnore("90000011")).rejects.toThrow();
    await result.current.answers.onLink("90000011", "90000101");
    expect(linked).toEqual({ tmdbId: "90000101" });
    await waitFor(() => expect(lists).toBeGreaterThanOrEqual(2));
  });
});
