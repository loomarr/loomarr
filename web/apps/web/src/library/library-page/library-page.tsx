import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryItemDTO } from "@loomarr/api/models/libraryItemDTO";
import type { LibraryStatusOutputBody } from "@loomarr/api/models/libraryStatusOutputBody";
import { unwrap } from "@loomarr/api/unwrap";
import {
  LibraryCollections,
  type LibraryFilter,
  LibraryNeedsCards,
  LibraryNeedsStrip,
  LibraryShelf,
  type LibraryShelfTitle,
} from "@loomarr/ui";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useMemo, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { useLibraryCollections } from "../use-library-collections";
import { useLibraryNeeds } from "../use-library-needs";

// LIBRARY (#2151 decision 22): the titles Loomarr found in its media folders, as a poster grid
// under one line about the library. The filter and the search live in the URL; the grid loads a
// page at a time as it scrolls. While new files are being read, the status and the visible titles
// refetch every 5 s (decision 35), so "Getting ready" clears on its own.

const PAGE = 60;
const BUSY_REFETCH_MS = 5000;
const SEARCH_SETTLE_MS = 250;

// The poster grid is 6 across at most, so a poster is drawn at most about a sixth of the frame.
const POSTER_SIZES = "(min-width: 1024px) 180px, (min-width: 768px) 25vw, 33vw";

const titleOf = (item: LibraryItemDTO): LibraryShelfTitle => ({
  id: item.id,
  kind: item.kind,
  name: item.name,
  year: item.year,
  ready: item.ready,
  attention: item.unplayable > 0 || item.extraCopies > 0,
  poster: item.poster ? <ServedImage alt="" image={item.poster} sizes={POSTER_SIZES} /> : undefined,
});

// The scanner is walking or reading, or files are still waiting to be read.
const isBusy = (status: LibraryStatusOutputBody | undefined) =>
  status ? status.scan.stage !== "idle" || status.headersPending > 0 : false;

// The search as typed, settled for a moment before it asks the server.
const useSettled = (value: string) => {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), SEARCH_SETTLE_MS);
    return () => clearTimeout(t);
  }, [value]);
  return settled;
};

interface LibraryPageProps {
  filter: LibraryFilter;
  search: string;
  /**
   * "needs" is Needs you's own screen: its cards in place of the strip, and no grid.
   * "collections" is the Collections tab: the collections in place of the search and the grid.
   */
  view?: "titles" | "needs" | "collections";
}

const LibraryPage = ({ filter, search, view = "titles" }: LibraryPageProps) => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [typed, setTyped] = useState(search);
  const settled = useSettled(typed.trim());
  const needs = useLibraryNeeds();
  const collections = useLibraryCollections(view === "collections");
  const titlesView = view === "titles";
  const kind = filter === "movie" || filter === "series" ? filter : undefined;

  const status = libraryApi.useLibraryStatus({
    query: { refetchInterval: (query) => (isBusy(unwrap(query.state.data)) ? BUSY_REFETCH_MS : false) },
  });
  const current = unwrap(status.data);
  const busy = isBusy(current);
  // The offline folders' names come from the folder list; it's only asked for while one is offline.
  const roots = libraryApi.useListLibraryRoots({ query: { enabled: current?.health === "folder_offline" } });
  const refresh = () => queryClient.invalidateQueries({ queryKey: libraryApi.getLibraryStatusQueryKey() });
  const rescan = libraryApi.useRescanLibrary({ mutation: { onSettled: refresh } });
  const discover = libraryApi.useDiscoverLibraryRoots({ mutation: { onSettled: refresh } });

  // Keep the URL's search in step with what was typed, without a history entry per keystroke.
  const synced = useRef(search);
  useEffect(() => {
    if (!titlesView || settled === synced.current) return;
    synced.current = settled;
    void navigate({
      to: "/library",
      search: { kind, q: settled || undefined },
      replace: true,
    });
  }, [settled, kind, navigate, titlesView]);

  const params = {
    kind,
    search: settled || undefined,
    limit: PAGE,
  };
  const items = useInfiniteQuery({
    enabled: titlesView,
    queryKey: [...libraryApi.getListLibraryItemsQueryKey(params), "pages"],
    initialPageParam: 0,
    queryFn: async ({ pageParam, signal }) => {
      const res = await libraryApi.listLibraryItems({ ...params, offset: pageParam }, { signal });
      const body = unwrap(res);
      if (!body) throw new Error("Couldn't list the library");
      return body;
    },
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, page) => n + page.items.length, 0);
      return loaded < last.total && last.items.length > 0 ? loaded : undefined;
    },
    refetchInterval: busy ? BUSY_REFETCH_MS : false,
  });
  const titles = useMemo(
    () => items.data?.pages.flatMap((page) => page.items.map(titleOf)) ?? [],
    [items.data],
  );

  // The next page loads as the end of the grid scrolls into view.
  const end = useRef<HTMLDivElement>(null);
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = items;
  useEffect(() => {
    const node = end.current;
    if (!node || !hasNextPage) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting) && !isFetchingNextPage) void fetchNextPage();
      },
      { rootMargin: "600px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const error = status.error ?? items.error;
  if (error && !current) return <ErrorState error={error} onRetry={() => void status.refetch()} />;
  if (!current) return null;

  const offline = (unwrap(roots.data)?.roots ?? []).filter((f) => f.inUse && !f.readable).map((f) => f.path);

  const openNeeds = () => void navigate({ to: "/library/needs" });
  const openTitle = (id: string) => void navigate({ to: "/library/$id", params: { id } });
  const needsSlot =
    view !== "needs" ? (
      <LibraryNeedsStrip
        needs={needs.needs}
        onLink={needs.answers.onLink}
        onOpenNeeds={openNeeds}
        onOpenTitle={openTitle}
        orders={needs.orders}
        problems={needs.problems}
      />
    ) : needs.loaded ? (
      <LibraryNeedsCards
        needs={needs.needs}
        onChooseOrder={needs.answers.onChooseOrder}
        onIgnore={needs.answers.onIgnore}
        onLink={needs.answers.onLink}
        onOpenTitle={openTitle}
        onSearch={needs.onSearch}
        orders={needs.orders}
        problems={needs.problems}
      />
    ) : null;

  return (
    <>
      {items.error ? <ErrorState error={items.error} onRetry={() => void items.refetch()} /> : null}
      {needs.error && view === "needs" ? (
        <ErrorState error={needs.error} onRetry={() => void needs.refetch()} />
      ) : null}
      {collections.error && view === "collections" ? (
        <ErrorState error={collections.error} onRetry={() => void collections.refetch()} />
      ) : null}
      <LibraryShelf
        collections={
          collections.loaded ? (
            <LibraryCollections
              collections={collections.collections}
              onCopy={collections.onCopy}
              onEdit={(id) => void navigate({ to: "/library/collections/$id", params: { id } })}
              onNew={() => void navigate({ to: "/library/collections/new" })}
              sets={collections.sets}
            />
          ) : null
        }
        needs={needsSlot}
        showTitles={titlesView}
        checking={discover.isPending}
        filter={view === "collections" ? "collections" : filter}
        loading={items.isPending}
        onCheckAgain={() => discover.mutate()}
        onFilterChange={(next) =>
          void (next === "collections"
            ? navigate({ to: "/library/collections" })
            : navigate({
                to: "/library",
                search: { kind: next === "all" ? undefined : next, q: settled || undefined },
              }))
        }
        onOpenSettings={() => void navigate({ to: "/settings/library" })}
        onOpenTitle={openTitle}
        onRescan={() => rescan.mutate()}
        onSearchChange={setTyped}
        search={typed}
        status={{
          health: current.health,
          scan: current.scan,
          held: current.held,
          offline,
          needsYou: needs.needs.length + needs.problems.length > 0,
        }}
        titles={titles}
      />
      <div aria-hidden ref={end} />
    </>
  );
};

export type { LibraryPageProps };
export { LibraryPage };
