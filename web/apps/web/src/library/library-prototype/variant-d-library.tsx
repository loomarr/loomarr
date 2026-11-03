// PROTOTYPE — throwaway (#2151 step 2). Variant D · the Library page: status line, "Needs you",
// filter tabs and the poster shelf. Posters lead; a dot appears only when something needs attention.
import { Action, StatusDot, Surface, Tabs, Text } from "@loomarr/design-system";
import { useEffect, useState } from "react";
import { type ItemRow, itemsFor, scanStatus } from "./prototype-data";
import type { ScanState, Screen } from "./prototype-state";
import { ScanHealth } from "./scan-health";
import { CollectionsShelf } from "./variant-d-collections";
import { NeedsCards, NeedsStrip } from "./variant-d-needs";
import { ColorBars, FRAME, Poster, SearchBox } from "./variant-d-parts";

type Kind = "all" | "movies" | "shows" | "collections";

const subtitle = (it: ItemRow): string => `${it.year} · ${it.kind === "Series" ? "Show" : "Movie"}`;

const Tile = ({ item, onOpen }: { item: ItemRow; onOpen: () => void }) => {
  const notReady = item.facts === "waiting" || item.facts === "reading";
  // A title that was being prepared clears to normal when it becomes ready.
  const [ready, setReady] = useState(!notReady);
  useEffect(() => {
    if (item.facts !== "reading") return;
    const t = window.setTimeout(() => setReady(true), 7000);
    return () => window.clearTimeout(t);
  }, [item.facts]);
  const gettingReady = notReady && !ready;
  const attention = !gettingReady && (item.problems?.length || item.facts === "failed");
  return (
    <button
      aria-label={item.title}
      className="group flex min-w-0 cursor-pointer flex-col gap-2 text-left outline-none"
      onClick={onOpen}
      type="button"
    >
      <Poster dim={gettingReady || item.offline} hue={item.hue} title={item.title}>
        {attention ? (
          <span className="absolute top-2 right-2 rounded-full bg-black/60 p-1">
            <StatusDot label="Needs a look" tone="warn" />
          </span>
        ) : null}
        <div
          className={`absolute inset-x-0 bottom-0 flex justify-center bg-black/55 px-2 py-2 motion-safe:transition-opacity motion-safe:duration-300 ${gettingReady ? "opacity-100" : "opacity-0"}`}
        >
          <Text textRole="compact">Getting ready</Text>
        </div>
      </Poster>
      <div className="flex min-h-[4.5rem] min-w-0 flex-col gap-0.5">
        <Text numberOfLines={2} textRole="label">
          {item.title}
        </Text>
        <Text textRole="compact" tone="secondary">
          {subtitle(item)}
        </Text>
      </div>
    </button>
  );
};

const FreshState = ({ go }: { go: (s: Screen) => void }) => (
  <Surface alignItems="center" gap="control" padding="section">
    <ColorBars />
    <Text textRole="title">Dead air — tell Loomarr where your media is.</Text>
    <Text textRole="body" tone="secondary">
      Point it at your movie and show folders and your shelf fills up on its own.
    </Text>
    <div>
      <Action onPress={() => go("settings")} tone="primary">
        Tell Loomarr where your media is
      </Action>
    </div>
  </Surface>
);

const LibraryPage = ({
  screen,
  scan,
  go,
  kind,
  setKind,
  q,
  setQ,
}: {
  screen: Screen;
  scan: ScanState;
  go: (s: Screen) => void;
  kind: Kind;
  setKind: (k: Kind) => void;
  q: string;
  setQ: (q: string) => void;
}) => {
  const status = scanStatus(scan);
  const all = itemsFor(scan);
  const tab: Kind = screen === "collections" ? "collections" : kind;
  const needle = q.trim().toLowerCase();
  const shown = all
    .filter((i) => (kind === "movies" ? i.kind === "Movie" : kind === "shows" ? i.kind === "Series" : true))
    .filter((i) => i.title.toLowerCase().includes(needle));

  return (
    <div className={FRAME}>
      <div className="flex flex-col gap-3">
        <Text textRole="display">Library</Text>
        <ScanHealth density="compact" onOpenSettings={() => go("settings")} status={status} />
      </div>

      {screen === "unmatched" ? <NeedsCards go={go} scan={scan} /> : null}
      {screen === "items" ? <NeedsStrip go={go} scan={scan} /> : null}

      <div
        className={`flex flex-col gap-3 md:flex-row md:items-center md:justify-between ${scan === "fresh" ? "hidden" : ""}`}
      >
        <Tabs
          label="What to show"
          onValueChange={(t: Kind) => {
            if (t === "collections") go("collections");
            else {
              setKind(t);
              go("items");
            }
          }}
          options={[
            { label: "All", value: "all" },
            { label: "Movies", value: "movies" },
            { label: "Shows", value: "shows" },
            { label: "Collections", value: "collections" },
          ]}
          value={tab}
        />
        {screen === "items" ? <SearchBox onChange={setQ} value={q} /> : null}
      </div>

      {scan === "fresh" ? <FreshState go={go} /> : null}
      {screen === "collections" ? <CollectionsShelf go={go} /> : null}

      {scan !== "fresh" && screen === "items" ? (
        shown.length === 0 ? (
          <Text textRole="body" tone="secondary">
            Nothing here matches "{q}".
          </Text>
        ) : (
          <div className="grid grid-cols-3 gap-x-4 gap-y-6 md:grid-cols-4 lg:grid-cols-6">
            {shown.map((it) => (
              <Tile
                item={it}
                key={it.id}
                onOpen={() => go(it.kind === "Series" ? "item-series" : "item-movie")}
              />
            ))}
          </div>
        )
      ) : null}
    </div>
  );
};

export type { Kind };
export { LibraryPage };
