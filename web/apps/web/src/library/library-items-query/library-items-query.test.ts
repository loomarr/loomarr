import { QueryClient } from "@tanstack/react-query";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { libraryItemsQueryOptions } from "./library-items-query";

const item = (id: string) => ({ id, name: id });

describe("libraryItemsQueryOptions", () => {
  it("asks for the first page of a filter, and a second page only while titles remain", async () => {
    const asked: string[] = [];
    server.use(
      http.get("*/v1/library/items", ({ request }) => {
        const url = new URL(request.url);
        asked.push(`${url.searchParams.get("kind")}:${url.searchParams.get("offset")}`);
        return HttpResponse.json({ items: [item("a"), item("b")], total: 3 });
      }),
    );
    const options = libraryItemsQueryOptions({ kind: "movie" });
    const client = new QueryClient();

    const data = await client.ensureInfiniteQueryData(options);

    expect(asked).toEqual(["movie:0"]);
    expect(options.getNextPageParam(data.pages[0] as never, data.pages as never, 0, [0])).toBe(2);
  });

  it("gives a different filter its own entry, under the list's key so invalidating the list reaches it", () => {
    const all = libraryItemsQueryOptions({}).queryKey;
    const movies = libraryItemsQueryOptions({ kind: "movie" }).queryKey;
    expect(all).not.toEqual(movies);
    expect(all[0]).toBe("/v1/library/items");
  });
});
