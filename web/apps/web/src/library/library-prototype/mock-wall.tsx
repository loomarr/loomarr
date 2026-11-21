// PROTOTYPE — throwaway (#4476 A1). The Library wall: posters first, Needs you as one line beside
// the status, tabs that read as tabs, and the controls a 3,400-title library needs.
import {
  Action,

  Field,
  ProgressTrack,
  Skeleton,
  StatusDot,
  Text,
  TextLink,
} from "@loomarr/design-system";
import { useState } from "react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { COUNTS, GENRES, HOME_VIDEOS, type MockTitle, NEEDS, READY_MADE, TITLES } from "./mock-data";
import { JumpRail, Picker, PosterArt, UnderlineTabs } from "./mock-parts";

type WallState = "needs" | "clear" | "importing" | "loading" | "search-empty" | "home-video";
type Tab = "all" | "movies" | "shows" | "collections";
type Role = "admin" | "member";

const fmt = (n: number) => n.toLocaleString("en-US");

// The one status line. Needs you rides on it as a count and a link, never a box (decided). Every
// part sits on one flat row with the same gap and separator; the import's progress bar runs under it.
const Sep = () => (
  <span aria-hidden className="text-static-500">
    ·
  </span>
);

const StatusLine = ({ state, onReview }: { state: WallState; onReview: () => void }) => {
  const needs = state === "importing" ? 3 : state === "clear" ? 0 : NEEDS.length;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        {state === "importing" ? (
          <>
            <StatusDot label="Adding titles" tone="progress" />
            <Text textRole="compact" tone="secondary">
              Adding titles · {fmt(COUNTS.ready)} of {fmt(COUNTS.all)}
            </Text>
          </>
        ) : state === "loading" ? (
          <Skeleton width={120} />
        ) : (
          <>
            <StatusDot label="Up to date" tone="ok" />
            <Text textRole="compact" tone="secondary">
              Up to date
            </Text>
          </>
        )}
        {needs > 0 ? (
          <>
            <Sep />
            <span className="flex items-center gap-2 whitespace-nowrap">
              <StatusDot label="Needs you" tone="warn" />
              <Text textRole="compact">{needs} need you</Text>
              <TextLink onPress={onReview} tone="info">
                Review
              </TextLink>
            </span>
          </>
        ) : null}
      </div>
      {state === "importing" ? (
        <div className="w-56">
          <ProgressTrack label="Adding your titles" percent={(COUNTS.ready / COUNTS.all) * 100} tone="tune" />
        </div>
      ) : null}
    </div>
  );
};

// One title on the wall: the poster, then its name and one line under it. A title that is still
// being read says so in that line (not in a badge over the art, which overflowed on phone).
const Tile = ({ title, preparing }: { title: MockTitle; preparing: boolean }) => (
  <a className="group focus-visible:outline-signal block rounded-md focus-visible:outline-2 focus-visible:outline-offset-4" href="#title">
    <PosterArt dim={preparing} title={title} />
    <div className="mt-2 min-w-0">
      <Text as="p" numberOfLines={1} strong textRole="label">
        {title.name}
      </Text>
      <Text as="p" numberOfLines={1} textRole="compact" tone="secondary">
        {preparing
          ? "Getting ready"
          : title.kind === "home"
            ? `${title.year} · Home video`
            : `${title.year} · ${title.kind === "show" ? "Show" : "Movie"}`}
      </Text>
    </div>
  </a>
);

const GRID = "grid grid-cols-3 gap-x-2.5 gap-y-4 sm:grid-cols-4 sm:gap-x-4 sm:gap-y-6 lg:grid-cols-6";

const Wall = ({ titles, state }: { titles: MockTitle[]; state: WallState }) => (
  <ul className={GRID}>
    {titles.map((t, i) => (
      <li key={t.id}>
        <Tile preparing={state === "importing" && i % 5 === 3} title={t} />
      </li>
    ))}
  </ul>
);

const LoadingWall = () => (
  <ul aria-busy className={GRID} aria-label="Loading titles">
    {Array.from({ length: 18 }, (_, i) => (
      <li className="flex flex-col gap-2" key={i}>
        <div className="aspect-[2/3] w-full overflow-hidden rounded-md">
          <Skeleton height="100%" shape="media" width="100%" />
        </div>
        <Skeleton width="80%" />
        <Skeleton width="40%" />
      </li>
    ))}
  </ul>
);

// An empty search is not a dead end: clear it, or ask for the title (the household's way in).
const SearchEmpty = ({ query, onClear }: { query: string; onClear: () => void }) => (
  <div className="flex flex-col items-start gap-3 py-12 sm:items-center sm:py-20 sm:text-center">
    <Text as="h2" textRole="heading">
      Nothing here matches “{query}”
    </Text>
    <Text as="p" textRole="body" tone="secondary">
      Search looks at titles, years and genres across all {fmt(COUNTS.all)} titles.
    </Text>
    <div className="mt-2 flex flex-wrap gap-2">
      <Action icon="requests" tone="primary">
        Ask for it
      </Action>
      <Action onPress={onClear} variant="outline">
        Clear search
      </Action>
    </div>
  </div>
);

// Collections with none yet: what one is, the one next action, and ready-made sets to start from.
const CollectionsEmpty = ({ role }: { role: Role }) => (
  <div className="flex max-w-3xl flex-col gap-8 py-8">
    <div className="flex flex-col items-start gap-3">
      <Text as="h2" textRole="heading">
        No collections yet
      </Text>
      <Text as="p" textRole="body" tone="secondary">
        {role === "admin"
          ? "A collection is a set of titles you put together, like “Rainy Sunday films”. A channel can play from it."
          : "When someone in your household puts titles together, like “Rainy Sunday films”, they show up here."}
      </Text>
      {role === "admin" ? (
        <Action icon="plus" tone="primary">
          New collection
        </Action>
      ) : null}
    </div>
    {role === "admin" ? (
      <div className="flex flex-col gap-3">
        <Text as="h3" textRole="label" strong>
          Or start from a ready-made set
        </Text>
        <ul className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          {READY_MADE.map((set) => (
            <li className="bg-static-900 flex items-center gap-3 rounded-lg p-3" key={set.name}>
              <div className="relative h-16 w-[68px] shrink-0">
                {set.hues.map((hue, i) => (
                  <div className="absolute top-0 w-10" key={hue} style={{ left: i * 14, zIndex: 3 - i }}>
                    <PosterArt small title={{ name: "", year: 0, hue, hue2: hue + 2, layout: [5, 2, 7][i] ?? 0 }} />
                  </div>
                ))}
              </div>
              <div className="min-w-0">
                <Text as="p" numberOfLines={2} strong textRole="label">
                  {set.name}
                </Text>
                <Text as="p" textRole="compact" tone="secondary">
                  You have {set.have} of {set.of}
                </Text>
              </div>
            </li>
          ))}
        </ul>
      </div>
    ) : null}
  </div>
);

// A home video answered "It's ours, keep it as is". Its own name, its own genre, a frame
// from the video for a picture, no catalogue facts.
const HomeVideoTile = ({ title }: { title: MockTitle }) => (
  <div>
    <div className="bg-static-900 relative flex aspect-[2/3] w-full items-center justify-center overflow-hidden rounded-md">
      <div className="w-full">
        <PosterArt small title={{ ...title, layout: 0 }} />
      </div>
      <div
        aria-hidden
        className="absolute inset-x-0 top-1/2 aspect-video -translate-y-1/2"
        style={{
          background:
            "radial-gradient(60% 70% at 40% 45%, color-mix(in srgb, var(--color-caution) 18%, transparent), transparent 70%), linear-gradient(180deg, color-mix(in srgb, var(--color-tune) 14%, var(--color-static-900)), var(--color-static-950))",
        }}
      />
    </div>
    <div className="mt-2">
      <Text as="p" numberOfLines={1} strong textRole="label">
        {title.name}
      </Text>
      <Text as="p" textRole="compact" tone="secondary">
        {title.year} · Home video
      </Text>
    </div>
  </div>
);

const LibraryWall = ({
  state,
  role,
  tab: initialTab,
  phone,
  onReview,
}: {
  state: WallState;
  role: Role;
  tab: Tab;
  phone: boolean;
  onReview: () => void;
}) => {
  const [tab, setTab] = useState<Tab>(initialTab);
  const [sort, setSort] = useState<"az" | "recent">(state === "home-video" ? "recent" : "az");
  const [genre, setGenre] = useState("all");
  const [query, setQuery] = useState(state === "search-empty" ? "zebra 1977" : "");
  const admin = role === "admin";
  const loading = state === "loading";

  const shown = TITLES.filter((t) => tab === "all" || (tab === "movies" ? t.kind === "movie" : t.kind === "show"))
    .filter((t) => genre === "all" || t.genres.includes(genre))
    .sort((a, b) => (sort === "az" ? a.name.replace(/^(The|An?) /, "").localeCompare(b.name.replace(/^(The|An?) /, "")) : a.added - b.added));
  const empty = query !== "" && state === "search-empty";

  return (
    <div className="flex flex-col">
      <PageHeader
        actions={
          admin && !phone ? (
            <Action icon="settings" variant="ghost">
              Library settings
            </Action>
          ) : undefined
        }
        status={
          admin ? (
            <div className="flex items-center justify-between gap-2">
              <StatusLine onReview={onReview} state={state} />
              {phone ? (
                <Action icon="settings" layout="icon" variant="ghost">
                  Library settings
                </Action>
              ) : null}
            </div>
          ) : undefined
        }
        title="Library"
      />
      <div className="flex flex-col gap-4 px-6 pt-2 pb-10">
        <div className="border-static-700 flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between lg:gap-6 lg:border-b">
          <UnderlineTabs
            bare={!phone}
            label="What to show"
            onChange={setTab}
            options={[
              { value: "all", label: "All", count: loading || phone ? undefined : fmt(COUNTS.all) },
              { value: "movies", label: "Movies", count: loading || phone ? undefined : fmt(COUNTS.movies) },
              { value: "shows", label: "Shows", count: loading || phone ? undefined : fmt(COUNTS.shows) },
              { value: "collections", label: "Collections" },
            ]}
            value={tab}
          />
          {tab === "collections" ? null : (
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center lg:pb-1.5">
              <div className="sm:w-64">
                <Field
                  aria-label="Search the library"
                  density="pointer"
                  onChangeText={setQuery}
                  placeholder="Search titles, years, genres"
                  value={query}
                />
              </div>
              <div className="grid grid-cols-[1fr_1fr_auto] items-center gap-2 sm:flex">
                <div className="sm:w-40">
                  <Picker
                    label="Sort"
                    onChange={setSort}
                    options={[
                      { value: "az", label: "A–Z" },
                      { value: "recent", label: "Recently added" },
                    ]}
                    value={sort}
                  />
                </div>
                <div className="sm:w-44">
                  <Picker
                    label="Genre"
                    onChange={setGenre}
                    options={[
                      { value: "all", label: "All genres" },
                      ...(state === "home-video" ? [{ value: "Home videos", label: "Home videos · 2" }] : []),
                      ...GENRES.map(([g, n]) => ({ value: g, label: `${g} · ${fmt(n)}` })),
                    ]}
                    value={genre}
                  />
                </div>
                {phone && !loading ? (
                  <span className="ml-auto shrink-0 whitespace-nowrap">
                    <Text as="p" textRole="compact" tone="secondary">
                      {fmt(tab === "shows" ? COUNTS.shows : tab === "movies" ? COUNTS.movies : COUNTS.all)} titles
                    </Text>
                  </span>
                ) : null}
              </div>
            </div>
          )}
        </div>

        {tab === "collections" ? (
          <CollectionsEmpty role={role} />
        ) : empty ? (
          <SearchEmpty onClear={() => setQuery("")} query={query} />
        ) : (
          <div className="flex gap-2 sm:gap-4">
            <div className="min-w-0 flex-1">
              {state === "home-video" ? (
                <div className="mb-6">
                  <div className={GRID}>
                    {HOME_VIDEOS.map((h) => (
                      <HomeVideoTile key={h.id} title={h} />
                    ))}
                  </div>
                </div>
              ) : null}
              {loading ? <LoadingWall /> : <Wall state={state} titles={shown} />}
            </div>
            {loading || phone ? null : <JumpRail current={sort === "az" ? "A" : "This week"} sort={sort} />}
          </div>
        )}
      </div>
    </div>
  );
};

export type { Role, Tab, WallState };
export { LibraryWall };
