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

const candidate = (value: string, name: string, nameMatches: boolean) => ({
  externalId: { namespace: "tmdb", value },
  kind: "movie" as const,
  name,
  nameMatches,
  yearMatches: false,
  lengthMatches: false,
  episodesMatches: false,
});

const need = (id: string, candidates: ReturnType<typeof candidate>[]) => ({
  item: {
    id,
    kind: "movie" as const,
    name: "Untitled",
    files: 1,
    ready: true,
    unplayable: 0,
    extraCopies: 0,
  },
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
        orders: [],
        problems: [],
      }),
    );
    const { result } = renderHook(() => useLibraryNeeds(), { wrapper });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    const [named, unnamed] = result.current.needs;
    expect(named?.best?.externalId.value).toBe("90000101");
    expect(named?.others.map((c) => c.externalId.value)).toEqual(["90000102"]);
    expect(named?.files).toEqual(["/media/movies/90000011.mkv"]);
    expect(unnamed?.best).toBeUndefined();
    expect(unnamed?.others.map((c) => c.externalId.value)).toEqual(["90000103"]);
  });

  it("lists the titles that need a look with what's wrong with them", async () => {
    server.use(
      getListLibraryNeedsMockHandler({
        needs: [],
        orders: [],
        problems: [
          {
            item: { ...need("90000013", []).item, name: "Charlie", files: 2, extraCopies: 1 },
            extraBytes: 4096,
          },
        ],
      }),
    );
    const { result } = renderHook(() => useLibraryNeeds(), { wrapper });

    await waitFor(() => expect(result.current.problems).toHaveLength(1));
    expect(result.current.problems[0]).toMatchObject({
      id: "90000013",
      kind: "movie",
      name: "Charlie",
      unplayable: 0,
      extraBytes: 4096,
    });
  });

  it("rejects an answer the server refuses, and refreshes the list after one it takes", async () => {
    let lists = 0;
    let linked: unknown;
    server.use(
      http.get("*/v1/library/needs", () => {
        lists += 1;
        return HttpResponse.json({ needs: [need("90000011", [])], orders: [], problems: [] });
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
    await result.current.answers.onLink("90000011", { namespace: "imdb", value: "tt90000101" });
    expect(linked).toEqual({ externalId: { namespace: "imdb", value: "tt90000101" } });
    await waitFor(() => expect(lists).toBeGreaterThanOrEqual(2));
  });

  it("lists the shows whose episode order to say, and sends the household's answer", async () => {
    let chosen: unknown;
    const order = {
      item: { ...need("90000014", []).item, kind: "series" as const, name: "Tidewater" },
      disk: [{ season: 1, episodes: 12 }],
      files: [{ path: "/media/tv/Tidewater.S01E12.mkv", season: 1, episode: 12 }],
      choices: [
        { choice: "catalog" as const, seasons: [{ season: 1, episodes: 13 }], episodes: 13, fits: false },
        {
          choice: "order" as const,
          orderId: "90000799a",
          kind: "dvd" as const,
          name: "DVD Order",
          seasons: [{ season: 1, episodes: 12 }],
          episodes: 12,
          fits: true,
        },
      ],
    };
    server.use(
      getListLibraryNeedsMockHandler({ needs: [], orders: [order], problems: [] }),
      http.post("*/v1/library/items/:id/episode-order", async ({ request, params }) => {
        chosen = { id: params.id, body: await request.json() };
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { result } = renderHook(() => useLibraryNeeds(), { wrapper });

    await waitFor(() => expect(result.current.orders).toHaveLength(1));
    expect(result.current.orders[0]).toMatchObject({ id: "90000014", name: "Tidewater", disk: order.disk });
    expect(result.current.orders[0]?.choices[1]).toMatchObject({
      orderId: "90000799a",
      kind: "dvd",
      fits: true,
    });
    await result.current.answers.onChooseOrder("90000014", { choice: "order", orderId: "90000799a" });
    expect(chosen).toEqual({ id: "90000014", body: { choice: "order", orderId: "90000799a" } });
  });
});
