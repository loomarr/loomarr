import { getGetLibraryPersonMockHandler, getListLibraryFacetsMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { type LibraryUrlFilters, useLibraryFiltering } from "./use-library-filtering";

const facets = {
  studios: [{ value: "Example Studio", name: "Example Studio", titles: 4 }],
  audio: [],
  subtitles: [],
  hdr: [],
  uhd: 2,
  hdrAny: 1,
};

const renderFiltering = (filters: LibraryUrlFilters = {}, kind?: "movie" | "series") => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderHook(
    (props: { filters: LibraryUrlFilters }) =>
      useLibraryFiltering({ enabled: true, kind, filters: props.filters }),
    {
      initialProps: { filters },
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    },
  );
};

// Every people search asked for, as its words and kind.
const peopleAsked = () => {
  const asked: string[] = [];
  server.use(
    http.get("*/v1/library/people", ({ request }) => {
      const { searchParams } = new URL(request.url);
      asked.push(`${searchParams.get("q")} ${searchParams.get("kind")}`);
      return HttpResponse.json({ people: [{ id: "p1", name: "Ada Example", titles: 2 }] });
    }),
  );
  return asked;
};

// Past the settle time, so a request that was going to go has gone.
const settle = () => act(() => new Promise((done) => setTimeout(done, 400)));

describe("useLibraryFiltering", () => {
  beforeEach(() => {
    server.use(getListLibraryFacetsMockHandler(facets));
  });

  it("reads the tab's filter choices", async () => {
    const kinds: (string | null)[] = [];
    server.use(
      getListLibraryFacetsMockHandler(({ request }) => {
        kinds.push(new URL(request.url).searchParams.get("kind"));
        return facets;
      }),
    );
    const { result } = renderFiltering({}, "movie");

    await waitFor(() => expect(result.current.facets?.studios).toEqual(facets.studios));
    expect(kinds).toEqual(["movie"]);
  });

  it("names the URL's person, and drops the name when the URL drops them", async () => {
    server.use(getGetLibraryPersonMockHandler({ id: "p1", name: "Ada Example", titles: 2 }));
    const { result, rerender } = renderFiltering({ person: "p1" });

    await waitFor(() => expect(result.current.personName).toBe("Ada Example"));
    rerender({ filters: {} });
    expect(result.current.personName).toBeUndefined();
  });

  it("searches people from two characters, in the tab's kind", async () => {
    const asked = peopleAsked();
    const { result } = renderFiltering({}, "series");

    act(() => result.current.personSearch.onChange(" a "));
    await settle();
    expect(asked).toEqual([]);
    expect(result.current.personSearch.loading).toBe(false);

    act(() => result.current.personSearch.onChange("ad"));
    expect(result.current.personSearch.loading).toBe(true);
    await waitFor(() =>
      expect(result.current.personSearch.options).toEqual([
        { value: "p1", label: "Ada Example", trailing: "2" },
      ]),
    );
    expect(asked).toEqual(["ad series"]);
  });

  it("offers no one once the search is cleared", async () => {
    peopleAsked();
    const { result } = renderFiltering();
    act(() => result.current.personSearch.onChange("ad"));
    await waitFor(() => expect(result.current.personSearch.options).toHaveLength(1));

    act(() => result.current.personSearch.onChange(""));
    expect(result.current.personSearch.value).toBe("");
    expect(result.current.personSearch.options).toEqual([]);
    expect(result.current.personSearch.failed).toBe(false);
  });
});
