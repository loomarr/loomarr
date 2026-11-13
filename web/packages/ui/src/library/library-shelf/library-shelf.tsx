import { Action, Badge, Field, StatusDot, Surface, Tabs, Text, useWindowWidth } from "@loomarr/design-system";
import { type ComponentProps, useState } from "react";
import { Pressable } from "react-native";

import { StatePanel } from "../../state-panel";
import { LibraryGrid } from "../library-grid";
import { LibraryPoster, Line, PAGE_GUTTER, Stack, Twirl } from "../library-poster";
import { detailsWaiting, folderName, heldTitle, scanHeadline } from "../library-words";
import type { LibraryFilter, LibraryShelfProps, LibraryShelfTitle } from "./library-shelf.type";

// LIBRARY (#2151 decision 22, design/library variant D): one line about the library, the tabs and a
// search, then the poster grid. Presentational: the app owns the requests and draws the artwork.
// Plain words per the mock's word sheet: "Getting ready", never a facts state.

// The grid is 6 across on a desktop, 4 on a tablet and 3 on a phone, by the window as in the mock.
const columnsFor = (width: number) => (width >= 1024 ? 6 : width >= 768 ? 4 : 3);

// The one line under the heading: what's going on, the one action it needs, and the way to Settings.
const StatusLine = ({
  status,
  checking,
  onCheckAgain,
  onRescan,
  onOpenSettings,
  timeZone,
}: Pick<
  LibraryShelfProps,
  "status" | "checking" | "onCheckAgain" | "onRescan" | "onOpenSettings" | "timeZone"
>) => {
  const scan = scanHeadline(status.scan, status.needsYou || status.health === "folder_empty");
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
  return (
    <Line columnGap="control" paddingVertical="inline" rowGap="inline">
      <Line>
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
      {held ? (
        <Action onPress={onRescan} tone="primary">
          Check again
        </Action>
      ) : offline ? (
        <Action disabled={checking} onPress={onCheckAgain} tone="primary">
          {checking ? "Checking…" : "Check again"}
        </Action>
      ) : null}
      <Stack marginLeft="auto">
        <Action onPress={onOpenSettings} variant="ghost">
          Library settings
        </Action>
      </Stack>
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

const subtitle = (title: LibraryShelfTitle) => {
  const kind = title.kind === "series" ? "Show" : "Movie";
  return title.year ? `${title.year} · ${kind}` : kind;
};

const Tile = ({ title, onOpen }: { title: LibraryShelfTitle; onOpen: () => void }) => {
  const [focused, setFocused] = useState(false);
  return (
    // ⚠ Named by what it shows (the badge or dot, the title, its year and kind), not a label of its
    // own: a label that leaves out visible words fails WCAG 2.5.3, and voice control users say
    // what they see.
    <Pressable
      accessibilityRole="button"
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onPress={onOpen}
      style={{ minWidth: 0, width: "100%" }}
    >
      <Stack gap="inline">
        <Surface
          backgroundColor="transparent"
          borderColor={focused ? "actionFocus" : "transparent"}
          borderRadius="control"
          borderWidth={2}
        >
          <LibraryPoster art={title.poster} dim={!title.ready} name={title.name}>
            {title.ready ? null : (
              <Line flexWrap="nowrap" left={8} position="absolute" top={8}>
                <Badge tone="neutral">Getting ready</Badge>
                <Twirl />
              </Line>
            )}
            {title.ready && title.attention ? (
              // A dot, not a badge: a three-across phone grid has no room for a word on the poster.
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
        <Stack flexShrink={1} gap={2} minHeight={56} minWidth={0}>
          {/* Left-aligned: on the web the tile is a <button>, which centres its text. */}
          <Text numberOfLines={2} textAlign="left" textRole="label">
            {title.name}
          </Text>
          <Text textAlign="left" textRole="compact" tone="secondary">
            {subtitle(title)}
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

// The grid as rows, each filled out with empty cells so the last row's posters keep their width.
// LibraryGrid mounts only the rows near the screen.
const Grid = ({
  titles,
  onOpenTitle,
  onEndReached,
}: Pick<LibraryShelfProps, "titles" | "onOpenTitle" | "onEndReached">) => {
  const width = useWindowWidth();
  const columns = columnsFor(width);
  const rows: LibraryShelfTitle[][] = [];
  for (let i = 0; i < titles.length; i += columns) rows.push(titles.slice(i, i + columns));
  const slots = Array.from({ length: columns }, (_, i) => `slot-${i}`);
  return (
    <LibraryGrid
      estimateRowHeight={estimateRowHeight(width, columns)}
      label="Titles"
      onEndReached={onEndReached}
      renderRow={(row) => (
        // A row is layout only; the titles are the list's items.
        <Line alignItems="flex-start" flexWrap="nowrap" gap="control" paddingBottom="section" role="none">
          {row.map((title) => (
            <Stack flexBasis={0} flexGrow={1} flexShrink={1} key={title.id} minWidth={0} role="listitem">
              <Tile onOpen={() => onOpenTitle(title.id)} title={title} />
            </Stack>
          ))}
          {slots.slice(row.length).map((slot) => (
            <Stack aria-hidden flexBasis={0} flexGrow={1} flexShrink={1} key={slot} />
          ))}
        </Line>
      )}
      rowKey={(row, index) => row[0]?.id ?? `row-${index}`}
      rows={rows}
    />
  );
};

const FILTERS: { label: string; value: LibraryFilter }[] = [
  { label: "All", value: "all" },
  { label: "Movies", value: "movie" },
  { label: "Shows", value: "series" },
  { label: "Collections", value: "collections" },
];

const LibraryShelf = ({
  status,
  checking,
  onCheckAgain,
  onRescan,
  onOpenSettings,
  filter,
  onFilterChange,
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
  const fresh = status.health === "needs_folders";
  const onCollections = filter === "collections";
  const showTitles = showGrid && !onCollections;
  // From a tablet up the search sits beside the tabs; on a phone it takes its own full-width line.
  const wide = useWindowWidth() >= 768;
  return (
    <Stack
      alignSelf="center"
      gap="section"
      maxWidth={1152 + 2 * PAGE_GUTTER.wide}
      padding={wide ? PAGE_GUTTER.wide : PAGE_GUTTER.phone}
      width="100%"
    >
      <Stack gap="control">
        <Text as="h1" textRole="display">
          Library
        </Text>
        <StatusLine
          checking={checking}
          onCheckAgain={onCheckAgain}
          onOpenSettings={onOpenSettings}
          onRescan={onRescan}
          status={status}
          timeZone={timeZone}
        />
      </Stack>
      {fresh ? (
        <DeadAir onOpenSettings={onOpenSettings} />
      ) : (
        <>
          {needs}
          <Line columnGap="control" justifyContent="space-between" rowGap="control">
            <Tabs label="What to show" onValueChange={onFilterChange} options={FILTERS} value={filter} />
            {showTitles ? (
              <Stack width={wide ? 288 : "100%"}>
                <Field
                  accessibilityLabel="Search your library"
                  onChangeText={onSearchChange}
                  placeholder="Search your library"
                  value={search}
                />
              </Stack>
            ) : null}
          </Line>
          {onCollections ? collections : null}
          {!showTitles || loading ? null : titles.length === 0 && search.trim() !== "" ? (
            <Text textRole="body" tone="secondary">
              {`Nothing here matches "${search.trim()}".`}
            </Text>
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
