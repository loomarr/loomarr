import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryItemDTO } from "@loomarr/api/models/libraryItemDTO";
import { LibraryStatusOutputBodyHealth } from "@loomarr/api/models/libraryStatusOutputBodyHealth";
import { ListLibraryItemsKind } from "@loomarr/api/models/listLibraryItemsKind";
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
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useHouseholdTimeZone } from "@/channels/use-household-time-zone";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { useRouteContent } from "@/lib/use-route-content";
import { libraryItemsQueryOptions } from "../library-items-query";
import { useLibraryCollections } from "../use-library-collections";
import { useLibraryNeeds } from "../use-library-needs";
import { useLibraryScanFrames } from "../use-library-scan-frames";

// LIBRARY (#2151 decision 22): the titles Loomarr found in its media folders, as a poster grid
// under one line about the library. The filter and the search live in the URL; the grid loads a
// page at a time as it scrolls. Nothing polls: an import's progress arrives as library_scan frames
// written into the status, and each library_titles frame refetches the titles (the events bus), so
// new titles appear and "Getting ready" clears on their own.

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
  const timeZone = useHouseholdTimeZone();
  const [typed, setTyped] = useState(search);
  const settled = useSettled(typed.trim());
  const needs = useLibraryNeeds();
  const collections = useLibraryCollections(view === "collections");
  const titlesView = view === "titles";
  const kind =
    filter === ListLibraryItemsKind.movie || filter === ListLibraryItemsKind.series ? filter : undefined;

  useLibraryScanFrames();
  const status = libraryApi.useLibraryStatus();
  const current = unwrap(status.data);
  // The offline folders' names come from the folder list; it's only asked for while one is offline.
  const roots = libraryApi.useListLibraryRoots({
    query: { enabled: current?.health === LibraryStatusOutputBodyHealth.folder_offline },
  });
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

  const items = useInfiniteQuery({
    ...libraryItemsQueryOptions({ kind, search: settled || undefined }),
    enabled: titlesView,
  });
  const titles = useMemo(
    () => items.data?.pages.flatMap((page) => page.items.map(titleOf)) ?? [],
    [items.data],
  );
  // The library's content is its status line and the first page of titles under it.
  useRouteContent(current !== undefined && !items.isPending);

  // The next page loads once the grid's last rows are on screen.
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = items;
  const loadMore = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const error = status.error ?? items.error;
  if (error && !current) return <ErrorState error={error} onRetry={() => void status.refetch()} />;
  // The route's loader ensured the status, so it is here before this page mounts.
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
        onEndReached={loadMore}
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
          details: current.details,
        }}
        timeZone={timeZone}
        titles={titles}
      />
    </>
  );
};

export type { LibraryPageProps };
export { LibraryPage };
