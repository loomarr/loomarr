import {
  Action,
  CompactSelect,
  Field,
  LetterRail,
  ProgressTrack,
  Skeleton,
  StatusDot,
  Surface,
  Tabs,
  Text,
  useReducedMotionPreference,
  useWindowWidth,
} from "@loomarr/design-system";
import {
  type ComponentProps,
  cloneElement,
  type ReactElement,
  type ReactNode,
  useCallback,
  useRef,
  useState,
} from "react";
import { Pressable, View } from "react-native";

import { StatePanel } from "../../state-panel";
import { ActiveFilters, FilterPanel, type FocusTarget, filterCount } from "../library-filters";
import { LibraryGrid } from "../library-grid";
import { LibraryPoster, Line, PAGE_GUTTER, Stack, Twirl } from "../library-poster";
import { detailsWaiting, folderName, heldTitle, scanHeadline } from "../library-words";
import type {
  LibraryShelfAdmin,
  LibraryShelfCounts,
  LibraryShelfLetter,
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
// The wall's desktop layout starts here: six posters across, with the letter rail beside them.
const DESKTOP = 1024;
const columnsFor = (width: number) => (width >= DESKTOP ? 6 : width >= 768 ? 4 : 3);

const count = (n: number) => n.toLocaleString("en-US");
const titlesWord = (n: number) => (n === 1 ? "1 title" : `${count(n)} titles`);

// "6 need you · Review": the count and the way to Needs you's own page, which has no Review.
const NeedsYouLine = ({ needsYou, onOpenNeeds }: { needsYou: number; onOpenNeeds?: () => void }) => {
  const wide = useWindowWidth() >= 768;
  return (
    <Line flexWrap="nowrap">
      <StatusDot label="Needs you" tone="warn" />
      <Text textRole="label">{needsYou === 1 ? "1 needs you" : `${count(needsYou)} need you`}</Text>
      {/* One look at every width; on a phone it takes a touch target. */}
      {onOpenNeeds ? (
        <Action density={wide ? "pointer" : "touch"} onPress={onOpenNeeds} variant="ghost">
          Review
        </Action>
      ) : null}
    </Line>
  );
};

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
          Check files again
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

// A member's wall with no titles at all: there is nothing for them to set up, so no action.
const EmptyLibrary = () => (
  <StatePanel
    description="Movies and shows appear here once they're added."
    kind="empty"
    title="Nothing in the Library yet"
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

// A genre with nothing in the tab (the genre is kept as the tab changes): back to every genre.
const EmptyGenre = ({
  genre,
  kind,
  onAllGenres,
}: {
  genre: string;
  kind: LibraryShelfProps["filter"];
  onAllGenres: () => void;
}) => {
  const what = kind === "movie" ? "movies" : kind === "series" ? "shows" : "titles";
  return (
    <StatePanel
      action={{ label: "Show all genres", onPress: onAllGenres }}
      description="Pick another genre, or show them all."
      kind="empty"
      title={`No ${what} in ${genre}`}
    />
  );
};

// Filters on and nothing left: the one way back is clearing them.
const EmptyFilters = ({ onClear }: { onClear: () => void }) => (
  <StatePanel
    action={{ label: "Clear filters", onPress: onClear }}
    description="Remove a filter, or clear them all."
    kind="empty"
    title="No titles match these filters"
  />
);

// A search that found nothing: what was asked, what the search reads, and the way back. With a
// genre or another filter on, the search is inside it, so leaving it is a way back too.
const NoMatch = ({
  search,
  total,
  onClear,
  onAllGenres,
  onClearFilters,
}: {
  search: string;
  total?: number;
  onClear: () => void;
  onAllGenres?: () => void;
  onClearFilters?: () => void;
}) => {
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
        {onClearFilters
          ? "Search looks at titles, years and genres, only in titles that match your filters."
          : total === undefined
            ? "Search looks at titles, years and genres."
            : `Search looks at titles, years and genres across all ${titlesWord(total)}.`}
      </Text>
      <Line>
        <Action onPress={onClear} variant="outline">
          Clear search
        </Action>
        {onAllGenres ? (
          <Action onPress={onAllGenres} variant="outline">
            Show all genres
          </Action>
        ) : null}
        {onClearFilters ? (
          <Action onPress={onClearFilters} variant="outline">
            Clear filters
          </Action>
        ) : null}
      </Line>
    </Stack>
  );
};

const subtitle = (title: LibraryShelfTitle) => {
  const kind = title.homeVideo
    ? "Home video"
    : title.unidentified
      ? "Not identified yet"
      : title.kind === "series"
        ? "Show"
        : "Movie";
  return title.year ? `${title.year} · ${kind}` : kind;
};

// A poster grows a little under the pointer, so a household sees it opens (the maintainer's call,
// 2026-11-21: an exception to #2789's motion rule, for the Library's posters only). It grows over
// the base interaction time and not at all with reduced motion.
const HOVER_SCALE = 1.04;
const HOVER_MS = 200;

// The link the card is drawn on keeps its own semantics and navigation; it only takes the card's
// layout, focus and hover. The focus look is the card's amber border, so the browser's own outline
// never shows. Forced colours repaint borders alike, focused or not, but turn a transparent
// outline visible, so a focused card draws one.
const linkCard = (
  link: ReactElement<Record<string, unknown>>,
  focused: boolean,
  handlers: Record<"onBlur" | "onFocus" | "onMouseEnter" | "onMouseLeave", () => void>,
  children: ReactNode,
) =>
  cloneElement(
    link,
    {
      ...handlers,
      style: {
        color: "inherit",
        display: "block",
        minWidth: 0,
        outline: focused ? "2px solid transparent" : "none",
        textDecoration: "none",
        width: "100%",
      },
    },
    children,
  );

const Tile = ({
  title,
  onOpen,
  link,
}: {
  title: LibraryShelfTitle;
  onOpen: () => void;
  link?: ReactElement<Record<string, unknown>>;
}) => {
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
  // ⚠ The card is named by what it shows (the dot, the title, its year and kind or Getting
  // ready), not a label of its own: a label that leaves out visible words fails WCAG 2.5.3, and
  // voice control users say what they see.
  const card = (
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
          <LibraryPoster art={title.poster} dim={!title.ready} id={title.id} name={title.name}>
            {title.ready && (title.attention || title.unidentified) ? (
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
        {/* Left-aligned: a button card centres its text. */}
        <Text numberOfLines={2} textAlign="left" textRole="label">
          {title.name}
        </Text>
        <Text textAlign="left" textRole="compact" tone="secondary">
          {title.ready ? subtitle(title) : "Getting ready"}
        </Text>
      </Stack>
    </Stack>
  );
  if (link) {
    return linkCard(
      link,
      focused,
      {
        onBlur: () => setFocused(false),
        onFocus: () => setFocused(true),
        onMouseEnter: () => setHovered(true),
        onMouseLeave: () => setHovered(false),
      },
      card,
    );
  }
  return (
    <Pressable
      accessibilityRole="button"
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={onOpen}
      style={{
        minWidth: 0,
        outlineColor: "transparent",
        outlineStyle: focused ? "solid" : ("none" as never),
        outlineWidth: focused ? 2 : 0,
        width: "100%",
      }}
    >
      {card}
    </Pressable>
  );
};

// A row's first guess before it is measured: a poster's 2:3 box (the content width less the
// control gaps, per column) over the inline gap, the caption's 56 px floor and the section gap.
const estimateRowHeight = (width: number, columns: number, rail: number) => {
  const gutter = width >= 768 ? PAGE_GUTTER.wide : PAGE_GUTTER.phone;
  const content = Math.min(1152, width - 2 * gutter) - rail;
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

// The rail's own width and the gap before it.
const RAIL_SPACE = 40;

// The letter the reader is at: the one just jumped to while its title is in the top row, else
// the last letter that starts at or before the top row's first title.
const letterAt = (
  letters: readonly LibraryShelfLetter[],
  first: number,
  columns: number,
  jump: LibraryShelfProps["jump"],
) => {
  if (jump && jump.index >= first && jump.index < first + columns) return jump.letter;
  let at = letters[0];
  for (const entry of letters) if (entry.offset <= first) at = entry;
  return at?.letter;
};

// The grid as rows, with the letter rail beside it on a desktop window. LibraryGrid mounts only the
// rows near the screen. The rail stays put while the wall scrolls past it.
const Grid = ({
  titles,
  onOpenTitle,
  titleLink,
  onEndReached,
  letters,
  onJump,
  jump,
  jumpingTo,
}: Pick<
  LibraryShelfProps,
  "titles" | "onOpenTitle" | "titleLink" | "onEndReached" | "letters" | "onJump" | "jump" | "jumpingTo"
>) => {
  const width = useWindowWidth();
  const columns = columnsFor(width);
  const [topRow, setTopRow] = useState(0);
  const onTopRowChange = useCallback((row: number) => setTopRow(row), []);
  const rail = width >= DESKTOP && letters && letters.length > 0 && onJump ? letters : undefined;
  const rows: LibraryShelfTitle[][] = [];
  for (let i = 0; i < titles.length; i += columns) rows.push(titles.slice(i, i + columns));
  const grid = (
    <LibraryGrid
      estimateRowHeight={estimateRowHeight(width, columns, rail ? RAIL_SPACE : 0)}
      label="Titles"
      onEndReached={onEndReached}
      onTopRowChange={onTopRowChange}
      renderRow={(row) => (
        <Row columns={columns}>
          {row.map((title) => (
            <Stack flexBasis={0} flexGrow={1} flexShrink={1} key={title.id} minWidth={0} role="listitem">
              <Tile link={titleLink?.(title.id)} onOpen={() => onOpenTitle(title.id)} title={title} />
            </Stack>
          ))}
        </Row>
      )}
      rowKey={(row, index) => row[0]?.id ?? `row-${index}`}
      rows={rows}
      scrollTo={jump ? { row: Math.floor(jump.index / columns), request: jump.request } : undefined}
    />
  );
  // The grid keeps its place in the tree with and without the rail, so it is never remounted.
  return (
    // The rail is first in the document, so Tab reaches it before the endless grid; the reversed
    // row draws it on the right.
    <Line alignItems="flex-start" flexDirection="row-reverse" flexWrap="nowrap" gap="control" role="none">
      {rail ? (
        <View style={{ alignSelf: "flex-start", position: "sticky", top: 8 } as object}>
          <LetterRail
            available={rail.map((entry) => entry.letter)}
            busy={jumpingTo}
            current={letterAt(rail, topRow * columns, columns, jump)}
            onSelect={(letter) => {
              const entry = rail.find((candidate) => candidate.letter === letter);
              if (entry) onJump?.(entry);
            }}
          />
        </View>
      ) : null}
      <Stack flexBasis={0} flexGrow={1} flexShrink={1} minWidth={0}>
        {grid}
      </Stack>
    </Line>
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

// The genre choice: every genre with its titles, under "All genres". A chosen genre the tab has
// none of is still listed, so the select shows what is filtering.
const ALL_GENRES = "";
const genreOptions = (genres: NonNullable<LibraryShelfProps["genres"]>, chosen: string | undefined) => {
  const options = [
    { label: "All genres", value: ALL_GENRES },
    ...genres.map(({ name, titles }) => ({ label: `${name} · ${count(titles)}`, value: name })),
  ];
  if (chosen && !genres.some((g) => g.name === chosen)) options.push({ label: chosen, value: chosen });
  return options;
};

// The server matches a genre in any case; the list's own spelling is the one shown.
const listedGenre = (genres: LibraryShelfProps["genres"], genre: string | undefined) =>
  genres?.find((g) => g.name.toLowerCase() === genre?.toLowerCase());

// The tabs, then the search, the order and the genre. From a tablet up they share a line and the
// tabs carry their counts; on a phone the search takes its own line and the count sits beside
// the order and the genre.
const Controls = ({
  filter,
  onFilterChange,
  counts,
  sort,
  onSortChange,
  genres,
  genre,
  onGenreChange,
  search,
  onSearchChange,
  showSearch,
  filtering,
  moreRef,
}: Pick<
  LibraryShelfProps,
  | "filter"
  | "onFilterChange"
  | "counts"
  | "sort"
  | "onSortChange"
  | "genres"
  | "genre"
  | "onGenreChange"
  | "search"
  | "onSearchChange"
  | "filtering"
> & { showSearch: boolean; moreRef: (node: unknown) => void }) => {
  const wide = useWindowWidth() >= 768;
  const [open, setOpen] = useState(false);
  const [moreFocused, setMoreFocused] = useState(false);
  // On a phone the controls are tapped, so they take the design system's touch targets.
  const density = wide ? "pointer" : "touch";
  const tabs = (
    <Tabs
      appearance="underline"
      density={density}
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
      density={density}
      onChangeText={onSearchChange}
      placeholder="Search titles, years, genres"
      value={search}
    />
  );
  const order = (
    <CompactSelect
      density={density}
      label="Order"
      onValueChange={onSortChange}
      options={SORTS}
      value={sort}
      width={wide ? 168 : undefined}
    />
  );
  const listed = listedGenre(genres, genre);
  const chosen = listed?.name ?? genre;
  const genreSelect = (
    <CompactSelect
      density={density}
      label="Genre"
      onValueChange={(next) => onGenreChange(next === ALL_GENRES ? undefined : next)}
      options={genreOptions(genres ?? [], chosen)}
      value={chosen ?? ALL_GENRES}
      width={wide ? 168 : undefined}
    />
  );
  const on = filtering ? filterCount(filtering) : 0;
  const more = filtering ? (
    <Action
      aria-expanded={open}
      onBlur={() => setMoreFocused(false)}
      onFocus={() => setMoreFocused(true)}
      // The amber border is the focus look; forced colours turn the transparent outline visible.
      style={{ outlineColor: "transparent", outlineStyle: "solid", outlineWidth: moreFocused ? 2 : 0 }}
      density={density}
      icon="filter"
      onPress={() => setOpen(!open)}
      ref={moreRef}
      variant="outline"
    >
      {on > 0 ? `More filters (${on})` : "More filters"}
    </Action>
  ) : null;
  const panel = filtering && open ? <FilterPanel filtering={filtering} /> : null;
  if (wide) {
    return (
      <Stack gap="control">
        <Line columnGap="control" justifyContent="space-between" rowGap="control">
          {tabs}
          <Line flexShrink={1} flexWrap="nowrap" gap="control">
            <Stack flexShrink={1} minWidth={0} width={288}>
              {searchField}
            </Stack>
            {order}
            {genreSelect}
            {more}
          </Line>
        </Line>
        {panel}
      </Stack>
    );
  }
  // With a genre chosen the count is that genre's. On a phone the order and the genre share the
  // line equally, so a chosen genre's name and count stay readable; the count sits under them.
  // With filters on, the line of what is on carries the list's own count instead.
  const filtered = on > 0 || Boolean(filtering && genre);
  const shown = filtered
    ? undefined
    : genre
      ? listed?.titles
      : counts && filter !== "collections"
        ? counts[filter]
        : undefined;
  return (
    <Stack gap="control">
      {tabs}
      {searchField}
      <Line flexWrap="nowrap" gap="control">
        <Stack flex={1} minWidth={0}>
          {order}
        </Stack>
        <Stack flex={1} minWidth={0}>
          {genreSelect}
        </Stack>
      </Line>
      {more}
      {panel}
      {shown === undefined ? null : (
        <Stack alignItems="flex-end">
          <Text textRole="compact" tone="secondary">
            {titlesWord(shown)}
          </Text>
        </Stack>
      )}
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
  genres,
  genre,
  onGenreChange,
  filtering,
  letters,
  onJump,
  jump,
  jumpingTo,
  search,
  onSearchChange,
  titles,
  loading,
  onOpenTitle,
  titleLink,
  onEndReached,
  collections,
  timeZone,
}: LibraryShelfProps) => {
  const fresh = admin?.status.health === "needs_folders";
  const onCollections = filter === "collections";
  const showTitles = !onCollections;
  const width = useWindowWidth();
  const wide = width >= 768;
  const searched = search.trim();
  const allGenres = () => onGenreChange(undefined);
  const chosenGenre = genre ? (listedGenre(genres, genre)?.name ?? genre) : undefined;
  // Filters past the genre are on, so an empty list is theirs and the way back clears them all.
  const filtered = filtering && filterCount(filtering) > 0 ? filtering : undefined;
  const empty = !loading && titles.length === 0;
  // Clearing takes away the control that was pressed; More filters stays, so focus goes there.
  const more = useRef<FocusTarget | null>(null);
  const focusMore = () => more.current?.focus();
  const clearFilters = filtered
    ? () => {
        focusMore();
        filtered.onClear();
      }
    : undefined;
  // The All tab with no titles. While an import is adding them, an admin's status line tells that
  // story, so the dead air panel waits until nothing is running.
  const emptyLibrary = !admin ? (
    <EmptyLibrary />
  ) : scanHeadline(admin.status.scan, true).running ? null : (
    <DeadAir onOpenSettings={admin.onOpenSettings} />
  );
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
            filtering={filtering}
            genre={genre}
            genres={genres}
            moreRef={(node) => {
              more.current = node as FocusTarget | null;
            }}
            onFilterChange={onFilterChange}
            onGenreChange={onGenreChange}
            onSearchChange={onSearchChange}
            onSortChange={onSortChange}
            search={search}
            showSearch={showTitles}
            sort={sort}
          />
          {showTitles && filtering ? (
            <ActiveFilters
              clearable={!empty}
              filtering={filtering}
              genre={chosenGenre}
              onGenreClear={allGenres}
              onLast={focusMore}
            />
          ) : null}
          {onCollections ? collections : null}
          {!showTitles ? null : loading ? (
            <Loading />
          ) : titles.length === 0 && searched !== "" ? (
            <NoMatch
              onAllGenres={chosenGenre && !filtered ? allGenres : undefined}
              onClear={() => onSearchChange("")}
              onClearFilters={clearFilters}
              search={searched}
              total={chosenGenre ? undefined : counts?.[filter]}
            />
          ) : titles.length === 0 && clearFilters ? (
            <EmptyFilters onClear={clearFilters} />
          ) : titles.length === 0 && chosenGenre ? (
            <EmptyGenre genre={chosenGenre} kind={filter} onAllGenres={allGenres} />
          ) : titles.length === 0 && (filter === "movie" || filter === "series") ? (
            <EmptyTab kind={filter} onShowEverything={() => onFilterChange("all")} />
          ) : titles.length === 0 && filter === "all" && searched === "" ? (
            emptyLibrary
          ) : (
            <Grid
              jump={jump}
              jumpingTo={jumpingTo}
              letters={letters}
              onEndReached={onEndReached}
              onJump={onJump}
              onOpenTitle={onOpenTitle}
              titleLink={titleLink}
              titles={titles}
            />
          )}
        </>
      )}
    </Stack>
  );
};

export { LibraryShelf };
