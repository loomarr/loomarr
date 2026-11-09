// PROTOTYPE — throwaway (Direction mocks, 2026-11-08, #2785)
// Home's admin additions on the channel wall (variant C), shown only with `?admin=1`: a "Needs you"
// set of counts, each a number and a link, hidden when every count is zero; and who is watching
// what right now. Three placements: A a strip of four counts above the wall with watching as one
// row under it; B one quiet line of counts under the wall's header with the names on the tiles
// themselves; C a column beside the wall holding both as lists. Motion is a 120ms colour fade on
// hover; no transforms. Services and Activity stay in Settings → Diagnostics (decided).
import { SectionHeader, StatusDot } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { useEffect } from "react";
import type { Tint } from "../shared";
import {
  ADMIN_LAYOUTS,
  type AdminSearch,
  NEEDS,
  type NeedItem,
  needLabel,
  type Viewer,
  WATCHING,
} from "./admin-data";
import { NumberBlock, watchTo } from "./parts";
import type { ProtoChannel } from "./stub-data";

const fade = "transition-colors duration-[120ms]";

const showing = (items: NeedItem[]) => items.filter((n) => n.count > 0);

// The quota reads as a warning: a dot and a word, never a filled chip.
const WarnWord = ({ children }: { children: string }) => (
  <span className="text-static-300 inline-flex items-center gap-1.5 text-xs">
    <StatusDot tone="warn" size="compact" label="" />
    {children}
  </span>
);

// ---------------------------------------------------------------------------------------------
// A — four counts in a strip above the wall.

const NeedsStrip = ({ items, phone }: { items: NeedItem[]; phone: boolean }) => {
  const shown = showing(items);
  if (shown.length === 0) return null;
  return (
    <section aria-labelledby="home-needs">
      <SectionHeader id="home-needs" title="Needs you" />
      <ul
        className={`m-0 grid list-none gap-3 p-0 ${phone ? "grid-cols-2" : ""}`}
        style={phone ? undefined : { gridTemplateColumns: `repeat(${shown.length}, minmax(0, 1fr))` }}
      >
        {shown.map((n) => (
          <li key={n.id}>
            <Link
              to={n.to}
              className={`border-static-700 bg-static-900 hover:bg-static-800 focus-visible:ring-ring flex h-full flex-col gap-1 rounded-lg border p-3.5 focus-visible:ring-2 focus-visible:outline-none ${fade}`}
            >
              <span className="text-2xl leading-none font-semibold tabular-nums">{n.value}</span>
              <span className="text-static-300 text-sm">{needLabel(n)}</span>
              {n.warn && !phone && <WarnWord>Running short</WarnWord>}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
};

// A — who is watching, as one row of names under the strip.
const WatchingRow = ({
  viewers,
  channels,
  tint,
}: {
  viewers: Viewer[];
  channels: ProtoChannel[];
  tint: Tint;
}) => (
  <section aria-labelledby="home-watching" className="flex flex-wrap items-center gap-x-4 gap-y-2">
    <h2 id="home-watching" className="text-static-400 m-0 text-sm font-medium">
      Watching now
    </h2>
    {viewers.length === 0 ? (
      <p className="text-static-400 m-0 text-sm">Nobody is watching right now.</p>
    ) : (
      viewers.map((v) => {
        const c = channels.find((ch) => ch.id === v.channelId);
        if (!c) return null;
        return (
          <Link
            key={v.id}
            {...watchTo(c.id)}
            className={`hover:bg-static-800 focus-visible:ring-ring flex items-center gap-2 rounded-md px-1.5 py-1 text-sm focus-visible:ring-2 focus-visible:outline-none ${fade}`}
          >
            <span className="font-semibold">{v.name}</span>
            <span className="text-static-400">on</span>
            <NumberBlock channel={c} tint={tint} />
            <span>{c.name}</span>
          </Link>
        );
      })
    )}
  </section>
);

// ---------------------------------------------------------------------------------------------
// B — one quiet line of counts; the names live on the tiles.

const NeedsLine = ({ items, phone }: { items: NeedItem[]; phone: boolean }) => {
  const shown = showing(items);
  if (shown.length === 0) return null;
  return (
    <nav
      aria-label="Needs you"
      className={`border-static-700 flex items-center gap-x-1 gap-y-1 rounded-lg border px-3 py-2 text-sm ${
        phone ? "flex-wrap" : ""
      }`}
    >
      <span className="text-static-400 mr-2 font-medium">Needs you</span>
      {shown.map((n, i) => (
        <span key={n.id} className="flex items-center gap-1">
          {i > 0 && (
            <span aria-hidden className="text-static-600">
              ·
            </span>
          )}
          <Link
            to={n.to}
            className={`hover:bg-static-800 focus-visible:ring-ring flex items-center gap-1.5 rounded-md px-1.5 py-0.5 focus-visible:ring-2 focus-visible:outline-none ${fade}`}
          >
            {n.warn && <StatusDot tone="warn" size="compact" label="Running short" />}
            <span className="font-semibold tabular-nums">{n.value}</span>
            <span className="text-static-300">{phone ? n.short : needLabel(n)}</span>
          </Link>
        </span>
      ))}
    </nav>
  );
};

// B — the people on a tile, in the tile's top-right corner.
const tileWatchers = (viewers: Viewer[], channelId: string) =>
  viewers.filter((v) => v.channelId === channelId).map((v) => v.name);

const WatchersOnTile = ({ names, compact }: { names: string[]; compact: boolean }) =>
  names.length === 0 ? null : (
    <span className="bg-static-950/75 text-static-0 ring-static-0/20 ml-auto flex items-center gap-1 rounded-full px-2 py-0.5 text-xs ring-1">
      {compact ? names.map((n) => n[0]).join(" ") : `${names.join(", ")} watching`}
    </span>
  );

const watchingMeta = (viewers: Viewer[]) =>
  viewers.length === 0
    ? "Nobody is watching right now"
    : `${viewers.length} ${viewers.length === 1 ? "person" : "people"} watching`;

// ---------------------------------------------------------------------------------------------
// C — a column beside the wall: the counts as a list, then who is watching.

const NeedsList = ({ items }: { items: NeedItem[] }) => {
  const shown = showing(items);
  if (shown.length === 0) return null;
  return (
    <section aria-labelledby="home-needs">
      <SectionHeader id="home-needs" title="Needs you" />
      <ul className="border-static-700 bg-static-900 m-0 flex list-none flex-col overflow-hidden rounded-lg border p-0">
        {shown.map((n, i) => (
          <li key={n.id} className={i > 0 ? "border-static-700 border-t" : undefined}>
            <Link
              to={n.to}
              className={`hover:bg-static-800 focus-visible:ring-ring flex items-center gap-3 px-4 py-3 focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset ${fade}`}
            >
              <span className="w-10 shrink-0 text-lg font-semibold tabular-nums">{n.value}</span>
              <span className="min-w-0 flex-1 text-sm">{needLabel(n)}</span>
              {n.warn && <WarnWord>Running short</WarnWord>}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
};

const WatchingList = ({
  viewers,
  channels,
  tint,
}: {
  viewers: Viewer[];
  channels: ProtoChannel[];
  tint: Tint;
}) => (
  <section aria-labelledby="home-watching">
    <SectionHeader
      id="home-watching"
      title="Watching now"
      meta={viewers.length > 0 ? `${viewers.length}` : undefined}
    />
    {viewers.length === 0 ? (
      <p className="border-static-700 text-static-400 m-0 rounded-lg border border-dashed px-4 py-5 text-sm">
        Nobody is watching right now.
      </p>
    ) : (
      <ul className="border-static-700 bg-static-900 m-0 flex list-none flex-col overflow-hidden rounded-lg border p-0">
        {viewers.map((v, i) => {
          const c = channels.find((ch) => ch.id === v.channelId);
          if (!c) return null;
          return (
            <li key={v.id} className={i > 0 ? "border-static-700 border-t" : undefined}>
              <Link
                {...watchTo(c.id)}
                className={`hover:bg-static-800 focus-visible:ring-ring flex items-center gap-3 px-4 py-3 focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset ${fade}`}
              >
                <span className="bg-static-800 text-foreground flex size-8 shrink-0 items-center justify-center rounded-full text-sm">
                  {v.name[0]}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="m-0 flex items-center gap-2 truncate text-sm">
                    <span className="font-semibold">{v.name}</span>
                    <NumberBlock channel={c} tint={tint} />
                    <span className="truncate">{c.name}</span>
                  </p>
                  <p className="text-static-400 m-0 truncate text-xs">
                    {c.series ?? c.title} · {v.device}
                  </p>
                </div>
              </Link>
            </li>
          );
        })}
      </ul>
    )}
  </section>
);

// ---------------------------------------------------------------------------------------------
// The second white pill, under the main switcher, only with ?admin=1: L layout, N needs, W watching.

const AdminSwitcher = ({
  search,
  onChange,
}: {
  search: AdminSearch;
  onChange: (next: Partial<AdminSearch>) => void;
}) => {
  const cycle = <T,>(list: readonly T[], current: T) => list[(list.indexOf(current) + 1) % list.length] as T;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.closest("input, textarea, select, [contenteditable]")) return;
      if (e.key === "L")
        onChange({
          layout: cycle(
            ADMIN_LAYOUTS.map((l) => l.key),
            search.layout,
          ),
        });
      else if (e.key === "N") onChange({ needs: cycle(NEEDS, search.needs) });
      else if (e.key === "W") onChange({ watching: cycle(WATCHING, search.watching) });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  if (import.meta.env.PROD) return null;
  const select = "rounded border border-black/20 bg-white px-1 py-0.5";
  return (
    <div className="fixed top-12 left-1/2 z-[100] flex -translate-x-1/2 items-center gap-2 rounded-full bg-white px-3 py-1.5 font-sans text-xs text-black shadow-2xl ring-1 ring-black/20">
      <span className="font-semibold">Admin</span>
      <select
        aria-label="Admin layout"
        className={select}
        value={search.layout}
        onChange={(e) => onChange({ layout: e.target.value as AdminSearch["layout"] })}
      >
        {ADMIN_LAYOUTS.map((l) => (
          <option key={l.key} value={l.key}>
            {l.key} — {l.name}
          </option>
        ))}
      </select>
      <label className="flex items-center gap-1">
        Needs you
        <select
          className={select}
          value={search.needs}
          onChange={(e) => onChange({ needs: e.target.value as AdminSearch["needs"] })}
        >
          {NEEDS.map((n) => (
            <option key={n}>{n}</option>
          ))}
        </select>
      </label>
      <label className="flex items-center gap-1">
        Watching
        <select
          className={select}
          value={search.watching}
          onChange={(e) => onChange({ watching: e.target.value as AdminSearch["watching"] })}
        >
          {WATCHING.map((w) => (
            <option key={w}>{w}</option>
          ))}
        </select>
      </label>
      <span className="text-black/50">L layout · N needs · W watching</span>
    </div>
  );
};

export {
  AdminSwitcher,
  NeedsLine,
  NeedsList,
  NeedsStrip,
  tileWatchers,
  WatchersOnTile,
  WatchingList,
  WatchingRow,
  watchingMeta,
};
