import { useState } from "react";
import { Button, Caption, Card, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import type { DiscoverPanelProps } from "./discover-panel.type";

// DiscoverPanel — search archive.org for clips to add, without downloading anything (§10, V33).
//
// The point of the surface is DECIDING. An operator types what they are after, reads a handful
// of titles, and picks the ones worth fetching; the download itself is the ingest panel's job.
// That is why results carry no duration, tags or thumbnail — nothing has been fetched, so the
// API has only what the source's search index knows, and showing empty fields would imply
// otherwise.
//
// ⚠ **No per-result licence badge, deliberately (build plan §6.3).** archive.org declares a
// licence on ~8% of items and yt-dlp reports none at all, so a chip on every row would read
// "unknown" almost always — which implies a per-item check that never happened. The caveat is
// stated once, about the search, in the source's own words.
const DiscoverPanel = ({
  items,
  total,
  licenceNote,
  query,
  onQueryChange,
  onSearch,
  onAdd,
  searching,
  searched,
  className,
}: DiscoverPanelProps) => {
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set());

  const toggle = (id: string) =>
    setPicked((prev) => {
      const next = new Set(prev);
      if (!next.delete(id)) next.add(id);
      return next;
    });

  return (
    <Card className={className}>
      <div className="flex flex-col gap-3 p-4">
        <div>
          <h2 className="font-semibold text-lg">Find clips</h2>
          <p className="mt-1 text-muted-foreground text-sm">
            Search archive.org for commercials and bumpers. Nothing downloads until you add it.
          </p>
        </div>

        {/* ⚠ Capped like the catalog's filter box. Unbounded `flex-1` made a search field
            stretch the full card width (~1050px at 1440), which reads as a layout accident
            rather than a generous input. */}
        <div className="flex items-end gap-2">
          <div className="min-w-0 max-w-md flex-1">
            <Label htmlFor="discover-q">Search archive.org</Label>
            <Input
              id="discover-q"
              value={query}
              placeholder="1980s cereal commercial"
              onChange={(e) => onQueryChange(e.target.value)}
              // Enter searches, because a search box that ignores Enter is a small
              // daily annoyance with no upside.
              onKeyDown={(e) => {
                if (e.key === "Enter" && query.trim() !== "") onSearch();
              }}
            />
          </div>
          <Button size="sm" disabled={searching || query.trim() === ""} onClick={onSearch}>
            {searching ? "Searching…" : "Search"}
          </Button>
        </div>

        {/* ⚠ Only after a search. Rendering "no results" before anyone has searched tells an
            operator their catalog is empty when they have not asked it anything yet. */}
        {searched && items.length === 0 && !searching && (
          <p className="text-muted-foreground text-sm">Nothing matched. Try fewer or different words.</p>
        )}

        {items.length > 0 && (
          <>
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <Caption>
                {/* The SOURCE's count, not the page's — see the type's comment. */}
                {total > items.length
                  ? `${items.length} of ${total} matches`
                  : `${total} ${total === 1 ? "match" : "matches"}`}
              </Caption>
              {licenceNote && <Caption className="max-w-prose">{licenceNote}</Caption>}
            </div>

            <ul className="flex flex-col gap-1.5">
              {items.map((it) => (
                <li key={it.id} className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id={`pick-${it.id}`}
                    checked={picked.has(it.id)}
                    onChange={() => toggle(it.id)}
                    className="size-4 shrink-0 accent-signal"
                  />
                  <label htmlFor={`pick-${it.id}`} className="min-w-0 flex-1 cursor-pointer truncate text-sm">
                    {/* Falls back to the id: an item with no title is still addable, and
                        showing a blank row would make it look broken. */}
                    {it.title || it.id}
                  </label>
                  {/* Absent when the source declares no year, which is common — rendered as
                      nothing rather than as the year 0. */}
                  {it.year ? <Caption className="shrink-0 tabular-nums">{it.year}</Caption> : null}
                  <a
                    href={it.url}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="shrink-0 text-signal text-xs underline-offset-2 hover:underline"
                  >
                    View
                  </a>
                </li>
              ))}
            </ul>

            {/* ⚠ Only when this image can download. Without ingest tooling the results are
                still worth showing — an operator can fetch them by hand — so the list stays
                and only the action goes, rather than hiding the whole search behind a gate
                that has nothing to do with searching. */}
            {onAdd && (
              <div className={cn("flex items-center justify-end gap-2")}>
                <Button
                  size="sm"
                  disabled={picked.size === 0}
                  onClick={() => {
                    onAdd([...picked]);
                    setPicked(new Set());
                  }}
                >
                  {picked.size === 0
                    ? "Add selected"
                    : `Add ${picked.size} clip${picked.size === 1 ? "" : "s"}`}
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </Card>
  );
};

export { DiscoverPanel };
