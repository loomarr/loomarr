import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryItemDTO } from "@loomarr/api/models/libraryItemDTO";
import { LibraryStatusOutputBodyHealth } from "@loomarr/api/models/libraryStatusOutputBodyHealth";
import { ListLibraryItemsKind } from "@loomarr/api/models/listLibraryItemsKind";
import { unwrap } from "@loomarr/api/unwrap";
import {
  LibraryCollections,
  type LibraryFilter,
  LibraryShelf,
  type LibraryShelfCounts,
  type LibraryShelfLetter,
  type LibraryShelfTitle,
  type LibrarySort,
} from "@loomarr/ui";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { useHouseholdTimeZone } from "@/channels/use-household-time-zone";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { useRouteContent } from "@/lib/use-route-content";
import { LIBRARY_COUNTS, LIBRARY_PAGE_SIZE, libraryItemsQueryOptions } from "../library-items-query";
import { useLibraryCollections } from "../use-library-collections";
import { type LibraryUrlFilters, useLibraryFiltering } from "../use-library-filtering";
import { useLibraryScanFrames } from "../use-library-scan-frames";

// THE WALL (#4476): the titles Loomarr found in its media folders, as a poster grid. An admin sees
// one line about the library above it, with Needs you's count; a member browses the same wall
// read-only and the page asks only the routes a member reads (the titles and the collections).
// The filters, the order and the search live in the URL; the grid loads a page at a time as it
// scrolls. Nothing polls: an import's progress arrives as library_scan frames written into the
// status, and each library_titles frame refetches the titles (the events bus), so new titles
// appear and "Getting ready" clears on their own.

const SEARCH_SETTLE_MS = 250;

// The poster grid is 6 across at most, so a poster is drawn at most about a sixth of the frame.
const POSTER_SIZES = "(min-width: 1024px) 180px, (min-width: 768px) 25vw, 33vw";

const titleOf = (item: LibraryItemDTO): LibraryShelfTitle => ({
  id: item.id,
  kind: item.kind,
  name: item.name,
  year: item.year,
  ready: item.ready,
  // Both counts are admin-only; a member's titles carry neither, so none needs a look.
  attention: (item.unplayable ?? 0) > 0 || (item.extraCopies ?? 0) > 0,
  // Admin-only too: the title waits in Needs you.
  unidentified: item.unidentified,
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

// The tabs' counts: the whole library's total and each kind's, whatever the search. Each asks for
// one title, since only the total is read.
const useTabCounts = (enabled: boolean): LibraryShelfCounts | undefined => {
  const options = { query: { enabled } };
  const [allParams, movieParams, seriesParams] = LIBRARY_COUNTS;
  const all = unwrap(libraryApi.useListLibraryItems(allParams, options).data)?.total;
  const movie = unwrap(libraryApi.useListLibraryItems(movieParams, options).data)?.total;
  const series = unwrap(libraryApi.useListLibraryItems(seriesParams, options).data)?.total;
  return all === undefined || movie === undefined || series === undefined
    ? undefined
    : { all, movie, series };
};

// A letter the rail was asked for, in the list it was asked of: another filter drops it.
interface Jump {
  list: string;
  index: number;
  letter: string;
  request: number;
  /** When the list's last failure had happened as the letter was pressed. */
  failedAt: number;
}

// The keys that scroll a page.
const SCROLL_KEYS = ["ArrowUp", "ArrowDown", "PageUp", "PageDown", "Home", "End", " "];

// The rail's jump. The wall is one list from its first title, so a jump loads pages until the
// title and a page below it are in, then the grid scrolls to it; scrolling back up needs nothing.
const useLetterJump = (list: string) => {
  const [asked, setAsked] = useState<Jump>();
  // Dropped as the list changes, so coming back to a list does not jump again.
  if (asked && asked.list !== list) setAsked(undefined);
  const jump = asked?.list === list ? asked : undefined;
  const requests = useRef(0);
  // Read by the query as it asks for each page, so it is a ref: how far this list was jumped.
  const reached = useRef({ list, to: 0 });
  const reach = useCallback(() => (reached.current.list === list ? reached.current.to : 0), [list]);
  const onJump = useCallback(
    ({ letter, offset }: LibraryShelfLetter, failedAt: number) => {
      const to = Math.max(reach(), offset + LIBRARY_PAGE_SIZE);
      reached.current = { list, to };
      requests.current += 1;
      setAsked({ list, index: offset, letter, request: requests.current, failedAt });
    },
    [list, reach],
  );
  const cancel = useCallback(() => {
    reached.current = { list, to: 0 };
    setAsked(undefined);
  }, [list]);
  return { jump, onJump, cancel, reach };
};

// While a jump is loading, a reader who scrolls has moved on: the jump is dropped rather than
// pulling them to the letter later.
const useCancelOnScroll = (pending: boolean, cancel: () => void) => {
  useEffect(() => {
    if (!pending) return;
    const onKey = (event: KeyboardEvent) => {
      if (SCROLL_KEYS.includes(event.key)) cancel();
    };
    window.addEventListener("wheel", cancel, { passive: true });
    window.addEventListener("touchmove", cancel, { passive: true });
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("wheel", cancel);
      window.removeEventListener("touchmove", cancel);
      window.removeEventListener("keydown", onKey);
    };
  }, [pending, cancel]);
};

interface LibraryPageProps {
  filter: LibraryFilter;
  /** Only titles with this genre; absent is every genre. */
  genre?: string;
  /** The filters past the genre, as the URL holds them. */
  filters?: LibraryUrlFilters;
  search: string;
  /** The wall's order; by name unless the URL says recent. */
  sort?: LibrarySort;
  /** "collections" is the Collections tab: the collections in place of the search and the grid. */
  view?: "titles" | "collections";
}

const NO_FILTERS: LibraryUrlFilters = {};

const LibraryPage = ({
  filter,
  genre,
  filters = NO_FILTERS,
  search,
  sort = "name",
  view = "titles",
}: LibraryPageProps) => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const timeZone = useHouseholdTimeZone();
  const { isAdmin, isLoading: authLoading } = useAuth();
  const [typed, setTyped] = useState(search);
  const settled = useSettled(typed.trim());
  const collections = useLibraryCollections(view === "collections");
  const titlesView = view === "titles";
  const counts = useTabCounts(true);
  const kind =
    filter === ListLibraryItemsKind.movie || filter === ListLibraryItemsKind.series ? filter : undefined;
  const recent = sort === "recent" ? sort : undefined;

  // The status, its folders and its actions are an admin's; a member's page never asks for them.
  useLibraryScanFrames();
  const status = libraryApi.useLibraryStatus({ query: { enabled: isAdmin } });
  const current = isAdmin ? unwrap(status.data) : undefined;
  // The offline folders' names come from the folder list; it's only asked for while one is offline.
  const roots = libraryApi.useListLibraryRoots({
    query: { enabled: isAdmin && current?.health === LibraryStatusOutputBodyHealth.folder_offline },
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
      search: { ...filters, kind, q: settled || undefined, sort: recent, genre },
      replace: true,
    });
  }, [settled, kind, recent, genre, filters, navigate, titlesView]);

  const list = { ...filters, kind, search: settled || undefined, sort: recent, genre };
  const { jump, onJump, cancel, reach } = useLetterJump(JSON.stringify(list));
  const items = useInfiniteQuery({
    ...libraryItemsQueryOptions({ ...list, reach }),
    enabled: titlesView,
  });
  const filtering = useLibraryFiltering({ enabled: titlesView, kind, filters });
  // The tab's genres, each with its titles; a member reads them too.
  const genreList = libraryApi.useListLibraryGenres({ kind }, { query: { enabled: titlesView } });
  const titles = useMemo(
    () => items.data?.pages.flatMap((page) => page.items.map(titleOf)) ?? [],
    [items.data],
  );
  // The page's content is the admin's status line and whatever the view shows under it.
  const viewReady = titlesView ? !items.isPending : collections.loaded;
  useRouteContent(!authLoading && (!isAdmin || current !== undefined) && viewReady);

  // The next page loads once the grid's last rows are on screen.
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = items;
  const loadMore = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  // A jump loads on until its title and a page below it are in, or the list ends. A page that
  // fails after the press stops it, so it is not asked for again and again; the next press asks
  // once more. The failure shows above the wall with the other page errors.
  const jumpShort =
    jump !== undefined && titles.length < jump.index + LIBRARY_PAGE_SIZE && Boolean(hasNextPage);
  const jumpLoading = jumpShort && items.errorUpdatedAt === jump?.failedAt;
  // How many titles the jump still wants. It falls with each page, which asks for the next one even
  // when a page arrives too fast for the fetching state to be seen changing.
  const wanted = jumpLoading && jump ? jump.index + LIBRARY_PAGE_SIZE - titles.length : 0;
  useEffect(() => {
    if (wanted > 0) loadMore();
  }, [wanted, loadMore]);
  useCancelOnScroll(jumpLoading, cancel);
  // The title is scrolled to once everything asked for is in, or when no more is coming.
  const arrived = jump && !jumpLoading && jump.index < titles.length ? jump : undefined;
  // The server works the letters out again with each page, so the newest page's are current.
  const letters = items.data?.pages.at(-1)?.letters;

  if (authLoading) return null;
  if (isAdmin && status.error && !current) {
    return <ErrorState error={status.error} onRetry={() => void status.refetch()} />;
  }
  // The route's loader ensured an admin's status, so it is here before this page mounts.
  if (isAdmin && !current) return null;

  const offline = (unwrap(roots.data)?.roots ?? []).filter((f) => f.inUse && !f.readable).map((f) => f.path);

  const openTitle = (id: string) => void navigate({ to: "/library/$id", params: { id } });
  const showWall = (next: LibraryUrlFilters & { kind?: typeof kind; sort?: typeof recent; genre?: string }) =>
    void navigate({ to: "/library", search: { ...next, q: settled || undefined } });
  const shown = { ...filters, kind, sort: recent, genre };

  return (
    <>
      {items.error ? <ErrorState error={items.error} onRetry={() => void items.refetch()} /> : null}
      {collections.error && view === "collections" ? (
        <ErrorState error={collections.error} onRetry={() => void collections.refetch()} />
      ) : null}
      <LibraryShelf
        admin={
          current
            ? {
                status: {
                  health: current.health,
                  scan: current.scan,
                  held: current.held,
                  offline,
                  needsYou: current.needsYou,
                  details: current.details,
                },
                checking: discover.isPending,
                onCheckAgain: () => discover.mutate(),
                onRescan: () => rescan.mutate(),
                onOpenSettings: () => void navigate({ to: "/settings/library" }),
                onOpenNeeds: () => void navigate({ to: "/library/needs" }),
              }
            : undefined
        }
        collections={
          collections.loaded ? (
            <LibraryCollections
              collections={collections.collections}
              manage={
                isAdmin
                  ? {
                      onCopy: collections.onCopy,
                      onEdit: (id) => void navigate({ to: "/library/collections/$id", params: { id } }),
                      onNew: () => void navigate({ to: "/library/collections/new" }),
                    }
                  : undefined
              }
              sets={collections.sets}
            />
          ) : null
        }
        counts={counts}
        filter={view === "collections" ? "collections" : filter}
        genre={genre}
        genres={unwrap(genreList.data)?.genres}
        jump={arrived}
        jumpingTo={jumpLoading ? jump?.letter : undefined}
        letters={recent ? undefined : letters}
        loading={items.isPending}
        onEndReached={loadMore}
        onFilterChange={(next) =>
          next === "collections"
            ? void navigate({ to: "/library/collections" })
            : showWall({ ...shown, kind: next === "all" ? undefined : next })
        }
        filtering={{
          ...filtering,
          filters,
          // The panel offers only values the list takes (`hdr` is "any" or a listed kind).
          onChange: (patch) => showWall({ ...shown, ...(patch as LibraryUrlFilters) }),
          onClear: () => showWall({ kind, sort: recent }),
          total: items.data?.pages.at(-1)?.total,
        }}
        onGenreChange={(next) => showWall({ ...shown, genre: next })}
        onJump={(letter) => onJump(letter, items.errorUpdatedAt)}
        onOpenTitle={openTitle}
        titleLink={(id) => <Link params={{ id }} to="/library/$id" />}
        onSearchChange={setTyped}
        onSortChange={(next) => showWall({ ...shown, sort: next === "recent" ? next : undefined })}
        search={typed}
        sort={sort}
        timeZone={timeZone}
        titles={titles}
      />
    </>
  );
};

export type { LibraryPageProps };
export { LibraryPage };
