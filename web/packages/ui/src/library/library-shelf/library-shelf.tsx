import {
  Action,
  CompactSelect,
  Field,
  ProgressTrack,
  Skeleton,
  StatusDot,
  Surface,
  Tabs,
  Text,
  TextLink,
  useReducedMotionPreference,
  useWindowWidth,
} from "@loomarr/design-system";
import { type ComponentProps, type ReactNode, useState } from "react";
import { Pressable, View } from "react-native";

import { StatePanel } from "../../state-panel";
import { LibraryGrid } from "../library-grid";
import { LibraryPoster, Line, PAGE_GUTTER, Stack, Twirl } from "../library-poster";
import { detailsWaiting, folderName, heldTitle, scanHeadline } from "../library-words";
import type {
  LibraryShelfAdmin,
  LibraryShelfCounts,
  LibraryShelfProps,
  LibraryShelfTitle,
  LibrarySort,
} from "./library-shelf.type";

// THE WALL (#4476, design/library 2026-11-21): the posters first. An admin's one line about the
// library sits above them, with Needs you as a count and Review; then the tabs with their counts,
// the search and the order, and the 2:3 poster grid. A member gets the same wall with no line and
// no settings. Presentational: the app owns the requests and draws the artwork.
// Plain words per the mock's word sheet: "Getting ready", never a facts state.

// The grid is 6 across on a desktop, 4 on a tablet and 3 on a phone, by the window as in the mock.
const columnsFor = (width: number) => (width >= 1024 ? 6 : width >= 768 ? 4 : 3);

const count = (n: number) => n.toLocaleString("en-US");
const titlesWord = (n: number) => (n === 1 ? "1 title" : `${count(n)} titles`);

// "6 need you · Review": the count and the way to Needs you's own page, which has no Review.
const NeedsYouLine = ({ needsYou, onOpenNeeds }: { needsYou: number; onOpenNeeds?: () => void }) => (
  <Line flexWrap="nowrap">
    <StatusDot label="Needs you" tone="warn" />
    <Text textRole="label">{`${count(needsYou)} need you`}</Text>
    {onOpenNeeds ? <TextLink onPress={onOpenNeeds}>Review</TextLink> : null}
  </Line>
);

// The one line under the heading: what's going on, the one action it needs, and Needs you.
const StatusLine = ({
  admin: { status, checking, onCheckAgain, onRescan, onOpenNeeds },
  timeZone,
}: {
  admin: LibraryShelfAdmin;
  timeZone?: string;
}) => {
  // The wall says "Up to date" once nothing is being added; the long "All caught up" belongs to
  // Settings, where the whole import is told.
  const scan = scanHeadline(status.scan, true);
  const held = status.held[0];
  const offline = status.health === "folder_offline" ? status.offline[0] : undefined;
  const waiting = detailsWaiting(status.details, status.scan, timeZone);
  // What needs the maintainer comes first; the import's own words show only when nothing does.
  // Titles stuck waiting for details come after the folders: Settings says why.
  const alert: [ComponentProps<typeof StatusDot>["tone"], string] | undefined =
    status.health === "needs_folders"
      ? ["off", "Tell Loomarr where your media is"]
      : held
        ? ["warn", heldTitle(held)]
        : offline
          ? ["warn", `Can't reach your ${folderName(offline)} folder`]
          : waiting
            ? ["warn", waiting.line]
            : undefined;
  const importing = alert === undefined;
  const [tone, text] = alert ?? [scan.tone, scan.text];
  // An import still running for the other folders stays on the line after a held removal or an
  // offline folder: "Can't reach your Movies folder · Adding your media · Step 2 of 3 · 34%".
  const alongside =
    alert !== undefined && status.health !== "needs_folders" && scan.shortLine
      ? `${scan.text} · ${scan.shortLine}`
      : "";
  // An import that is adding titles draws its bar under its words: the preparation hue while it
  // runs, the neutral foreground while it waits for someone watching.
  const bar = importing && scan.running;
  return (
    <Line columnGap="section" paddingVertical="inline" rowGap="inline">
      <Stack gap={6}>
        <Line flexWrap="nowrap">
          {/* A step moving twirls; paused, done or off keeps the still dot. */}
          {importing && scan.moving ? <Twirl /> : <StatusDot label={text} tone={tone} />}
          <Text textRole="label">
            {/* "Adding your media · Step 1 of 3 · 21% · about 9 min left". */}
            {importing && scan.shortLine
              ? `${text} · ${scan.shortLine}`
              : alongside
                ? `${text} · ${alongside}`
                : text}
          </Text>
        </Line>
        {bar ? (
          <ProgressTrack
            maxWidth={240}
            percent={scan.percent}
            tone={status.scan.paused ? "elapsed" : "tune"}
            width="100%"
          />
        ) : null}
      </Stack>
      {status.needsYou > 0 ? <NeedsYouLine needsYou={status.needsYou} onOpenNeeds={onOpenNeeds} /> : null}
      {held ? (
        <Action onPress={onRescan} tone="primary">
          Check again
        </Action>
      ) : offline ? (
        <Action disabled={checking} onPress={onCheckAgain} tone="primary">
          {checking ? "Checking…" : "Check again"}
        </Action>
      ) : null}
    </Line>
  );
};

// No folder chosen yet: the shelf is dead air until Loomarr knows where the media is.
const DeadAir = ({ onOpenSettings }: { onOpenSettings: () => void }) => (
  <StatePanel
    action={{ label: "Tell Loomarr where your media is", onPress: onOpenSettings }}
    description="Point it at your movie and show folders and your shelf fills up on its own."
    kind="empty"
    title="Dead air"
  />
);

// Movies or Shows with nothing in it: the empty panel, worded for the tab, back to everything.
const EmptyTab = ({ kind, onShowEverything }: { kind: "movie" | "series"; onShowEverything: () => void }) => {
  const what = kind === "movie" ? "movie" : "show";
  return (
    <StatePanel
      action={{ label: "Show everything", onPress: onShowEverything }}
      description={`When a ${what} lands in one of your folders, it shows up here.`}
      kind="empty"
      title={`No ${what}s yet`}
    />
  );
};

// A search that found nothing: what was asked, what the search reads, and the way back. The search
// matches names only, so it says so; years and genres join it with #4476 A2b.
const NoMatch = ({ search, total, onClear }: { search: string; total?: number; onClear: () => void }) => {
  const wide = useWindowWidth() >= 768;
  return (
    <Stack
      alignItems={wide ? "center" : "flex-start"}
      gap="control"
      paddingVertical={wide ? 64 : "section"}
      role="status"
    >
      <Text as="h2" textAlign={wide ? "center" : "left"} textRole="headline">
        {`Nothing here matches “${search}”`}
      </Text>
      <Text textAlign={wide ? "center" : "left"} textRole="body" tone="secondary">
        {total === undefined
          ? "Search looks at the names of your titles."
          : `Search looks at the names of all ${titlesWord(total)}.`}
      </Text>
      <Line>
        <Action onPress={onClear} variant="outline">
          Clear search
        </Action>
      </Line>
    </Stack>
  );
};

const subtitle = (title: LibraryShelfTitle) => {
  const kind = title.kind === "series" ? "Show" : "Movie";
  return title.year ? `${title.year} · ${kind}` : kind;
};

// A poster grows a little under the pointer, so a household sees it opens (the maintainer's call,
// 2026-11-21: an exception to #2789's motion rule, for the Library's posters only). It grows over
// the base interaction time and not at all with reduced motion.
const HOVER_SCALE = 1.04;
const HOVER_MS = 200;

const Tile = ({ title, onOpen }: { title: LibraryShelfTitle; onOpen: () => void }) => {
  const [focused, setFocused] = useState(false);
  const [hovered, setHovered] = useState(false);
  const reduced = useReducedMotionPreference();
  const grow =
    reduced === false
      ? {
          transform: [{ scale: hovered ? HOVER_SCALE : 1 }],
          transitionDuration: `${HOVER_MS}ms`,
          transitionProperty: "transform",
          transitionTimingFunction: "ease-out",
        }
      : {};
  return (
    // ⚠ Named by what it shows (the dot, the title, its year and kind or Getting ready), not a
    // label of its own: a label that leaves out visible words fails WCAG 2.5.3, and voice control
    // users say what they see.
    <Pressable
      accessibilityRole="button"
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onOpen}
      style={{ minWidth: 0, width: "100%" }}
    >
      <Stack gap="inline">
        <View style={grow as object}>
          <Surface
            backgroundColor="transparent"
            borderColor={focused ? "actionFocus" : "transparent"}
            borderRadius="control"
            borderWidth={2}
          >
            {/* A title getting ready is dimmed, and its caption says so: a word on the poster
                doesn't fit a three-across phone grid. */}
            <LibraryPoster art={title.poster} dim={!title.ready} name={title.name}>
              {title.ready && title.attention ? (
                // A dot, not a badge, for the same reason.
                <Surface
                  backgroundColor="surfaceCanvas"
                  borderRadius="round"
                  borderWidth={0}
                  padding={6}
                  position="absolute"
                  right={8}
                  top={8}
                >
                  <StatusDot label="Needs a look" tone="warn" />
                </Surface>
              ) : null}
            </LibraryPoster>
          </Surface>
        </View>
        <Stack flexShrink={1} gap={2} minHeight={56} minWidth={0}>
          {/* Left-aligned: on the web the tile is a <button>, which centres its text. */}
          <Text numberOfLines={2} textAlign="left" textRole="label">
            {title.name}
          </Text>
          <Text textAlign="left" textRole="compact" tone="secondary">
            {title.ready ? subtitle(title) : "Getting ready"}
          </Text>
        </Stack>
      </Stack>
    </Pressable>
  );
};

// A row's first guess before it is measured: a poster's 2:3 box (the content width less the
// control gaps, per column) over the inline gap, the caption's 56 px floor and the section gap.
const estimateRowHeight = (width: number, columns: number) => {
  const gutter = width >= 768 ? PAGE_GUTTER.wide : PAGE_GUTTER.phone;
  const content = Math.min(1152, width - 2 * gutter);
  const poster = (content - (columns - 1) * 12) / columns;
  return Math.round(poster * 1.5 + 8 + 56 + 24);
};

// One row of cells, each filled out with empty cells so the last row's posters keep their width.
const Row = ({ columns, children }: { columns: number; children: ReactNode[] }) => (
  // A row is layout only; the titles are the list's items.
  <Line alignItems="flex-start" flexWrap="nowrap" gap="control" paddingBottom="section" role="none">
    {children}
    {Array.from({ length: columns - children.length }, (_, i) => `slot-${i}`).map((slot) => (
      <Stack aria-hidden flexBasis={0} flexGrow={1} flexShrink={1} key={slot} />
    ))}
  </Line>
);

// The grid as rows. LibraryGrid mounts only the rows near the screen.
// TODO(#4476 A2b): the desktop letter rail sits beside this grid once the list can jump to a letter.
const Grid = ({
  titles,
  onOpenTitle,
  onEndReached,
}: Pick<LibraryShelfProps, "titles" | "onOpenTitle" | "onEndReached">) => {
  const width = useWindowWidth();
  const columns = columnsFor(width);
  const rows: LibraryShelfTitle[][] = [];
  for (let i = 0; i < titles.length; i += columns) rows.push(titles.slice(i, i + columns));
  return (
    <LibraryGrid
      estimateRowHeight={estimateRowHeight(width, columns)}
      label="Titles"
      onEndReached={onEndReached}
      renderRow={(row) => (
        <Row columns={columns}>
          {row.map((title) => (
            <Stack flexBasis={0} flexGrow={1} flexShrink={1} key={title.id} minWidth={0} role="listitem">
              <Tile onOpen={() => onOpenTitle(title.id)} title={title} />
            </Stack>
          ))}
        </Row>
      )}
      rowKey={(row, index) => row[0]?.id ?? `row-${index}`}
      rows={rows}
    />
  );
};

// The first page on its way: two rows of poster-shaped placeholders with their captions' lines.
const LOADING_ROWS = 2;
const Loading = () => {
  const columns = columnsFor(useWindowWidth());
  const cells = Array.from({ length: columns }, (_, i) => `cell-${i}`);
  return (
    <Stack aria-busy aria-label="Loading your titles" role="status">
      {Array.from({ length: LOADING_ROWS }, (_, r) => `row-${r}`).map((row) => (
        <Row columns={columns} key={row}>
          {cells.map((cell) => (
            <Stack flexBasis={0} flexGrow={1} flexShrink={1} gap="inline" key={cell} minWidth={0}>
              <Stack aspectRatio={2 / 3} width="100%">
                <Skeleton height="100%" shape="media" width="100%" />
              </Stack>
              <Skeleton width="80%" />
              <Skeleton width="50%" />
            </Stack>
          ))}
        </Row>
      ))}
    </Stack>
  );
};

const SORTS: { label: string; value: LibrarySort }[] = [
  { label: "A–Z", value: "name" },
  { label: "Recently added", value: "recent" },
];

const filterOptions = (counts: LibraryShelfCounts | undefined, withCounts: boolean) => {
  const counted = (label: string, value: "all" | "movie" | "series") =>
    withCounts && counts
      ? { label, value, count: count(counts[value]), countLabel: titlesWord(counts[value]) }
      : { label, value };
  return [
    counted("All", "all"),
    counted("Movies", "movie"),
    counted("Shows", "series"),
    { label: "Collections", value: "collections" as const },
  ];
};

// The tabs, then the search and the order. From a tablet up they share a line and the tabs carry
// their counts; on a phone the search takes its own line and the count sits beside the order.
const Controls = ({
  filter,
  onFilterChange,
  counts,
  sort,
  onSortChange,
  search,
  onSearchChange,
  showSearch,
}: Pick<
  LibraryShelfProps,
  "filter" | "onFilterChange" | "counts" | "sort" | "onSortChange" | "search" | "onSearchChange"
> & { showSearch: boolean }) => {
  const wide = useWindowWidth() >= 768;
  const tabs = (
    <Tabs
      appearance="underline"
      label="What to show"
      onValueChange={onFilterChange}
      options={filterOptions(counts, wide)}
      value={filter}
    />
  );
  if (!showSearch) return tabs;
  const searchField = (
    <Field
      accessibilityLabel="Search titles"
      onChangeText={onSearchChange}
      placeholder="Search titles"
      value={search}
    />
  );
  const order = (
    <CompactSelect label="Order" onValueChange={onSortChange} options={SORTS} value={sort} width={168} />
  );
  // TODO(#4476 A2b): the genre filter sits after the order once the list can filter by genre.
  if (wide) {
    return (
      <Line columnGap="control" flexWrap="nowrap" justifyContent="space-between">
        {tabs}
        <Line flexShrink={1} flexWrap="nowrap" gap="control">
          <Stack width={288}>{searchField}</Stack>
          {order}
        </Line>
      </Line>
    );
  }
  const shown = counts && filter !== "collections" ? counts[filter] : undefined;
  return (
    <Stack gap="control">
      {tabs}
      {searchField}
      <Line flexWrap="nowrap" gap="control">
        {order}
        {shown === undefined ? null : (
          <Stack marginLeft="auto">
            <Text textRole="compact" tone="secondary">
              {titlesWord(shown)}
            </Text>
          </Stack>
        )}
      </Line>
    </Stack>
  );
};

const LibraryShelf = ({
  admin,
  filter,
  onFilterChange,
  counts,
  sort,
  onSortChange,
  search,
  onSearchChange,
  titles,
  loading,
  onOpenTitle,
  onEndReached,
  needs,
  showTitles: showGrid = true,
  collections,
  timeZone,
}: LibraryShelfProps) => {
  const fresh = admin?.status.health === "needs_folders";
  const onCollections = filter === "collections";
  const showTitles = showGrid && !onCollections;
  const width = useWindowWidth();
  const wide = width >= 768;
  const searched = search.trim();
  return (
    <Stack
      alignSelf="center"
      gap="section"
      maxWidth={1152 + 2 * PAGE_GUTTER.wide}
      padding={wide ? PAGE_GUTTER.wide : PAGE_GUTTER.phone}
      width="100%"
    >
      <Stack gap="inline">
        <Line flexWrap="nowrap" justifyContent="space-between">
          <Text as="h1" textRole="display">
            Library
          </Text>
          {admin ? (
            <Action
              icon="settings"
              layout={wide ? "inline" : "icon"}
              onPress={admin.onOpenSettings}
              variant="ghost"
            >
              Library settings
            </Action>
          ) : null}
        </Line>
        {admin ? <StatusLine admin={admin} timeZone={timeZone} /> : null}
      </Stack>
      {fresh && admin ? (
        <DeadAir onOpenSettings={admin.onOpenSettings} />
      ) : (
        <>
          <Controls
            counts={counts}
            filter={filter}
            onFilterChange={onFilterChange}
            onSearchChange={onSearchChange}
            onSortChange={onSortChange}
            search={search}
            showSearch={showTitles}
            sort={sort}
          />
          {needs}
          {onCollections ? collections : null}
          {!showTitles ? null : loading ? (
            <Loading />
          ) : titles.length === 0 && searched !== "" ? (
            <NoMatch onClear={() => onSearchChange("")} search={searched} total={counts?.[filter]} />
          ) : titles.length === 0 && (filter === "movie" || filter === "series") ? (
            <EmptyTab kind={filter} onShowEverything={() => onFilterChange("all")} />
          ) : (
            <Grid onEndReached={onEndReached} onOpenTitle={onOpenTitle} titles={titles} />
          )}
        </>
      )}
    </Stack>
  );
};

export { LibraryShelf };
