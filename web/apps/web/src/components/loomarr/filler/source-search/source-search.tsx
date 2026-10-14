import type { DiscoveredClip } from "@loomarr/api/models/discoveredClip";
import type { DiscoveredClipStats } from "@loomarr/api/models/discoveredClipStats";
import { formatClipDuration, formatDuration, pluralize } from "@loomarr/core/format";
import { Check, Loader2, Play, Search } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { SourceSearchProps } from "./source-search.type";

// SourceSearch — the Sources tab's per-source search expander (§10, V35).
//
// ⚠ **Searching downloads NOTHING.** A listing is one Solr question plus a metadata call per
// row; the only path that fetches bytes is `Queue download`. The footnote says so, because that
// distinction is invisible from the rows themselves and an operator browsing a collection must
// not fear triggering a multi-gigabyte walk.
//
// ⚠ These rows are deliberately THINNER than the retired Discover cards: no description, and
// **no licence badge**. Build plan §6.3 measured ~92% of archive.org items as declaring no
// licence at all, so a badge would be empty on almost every row — and an absent licence means
// UNKNOWN, never "public domain", which a chip cannot say.

// Keep a long provider result set calm inside the source workspace. The server still owns the
// bounded 25-result page; this only controls how much of that page is revealed at once.
const RESULT_STEP = 8;

// Duration for a search row, composed rather than written a fourth time.
//
// ⚠ Two formatters because discovery spans two scales: real ad clips are seconds
// (`formatClipDuration` → "31s") while compilations run to hours, where the same function would
// render "183m 52s". Above an hour `formatDuration` reads "3h 4m", and the lost seconds do not
// matter on something nobody schedules into a break.
const durationLabel = (ms: number): string => (ms >= 3_600_000 ? formatDuration(ms) : formatClipDuration(ms));

// ⚠ The `date · duration · quality` line, and every part of it is OPTIONAL. Archive.org has not
// probed every item, so a missing value means UNKNOWN — rendering "0:00" or "0p" would claim a
// clip is empty or has no picture, which are both assertions the data does not support. An
// unknown part is omitted; a row that knows nothing shows no line at all.
//
// ⚠ Duration and quality come from `stats`, which arrives LATE (one upstream call per row), so
// this deliberately reads the late value in preference to the clip's own — a search never sets
// them, and a future one that did should not silently win over the measured answer.
const metaParts = (clip: DiscoveredClip, stat?: DiscoveredClipStats): string[] => {
  const parts: string[] = [];
  if (clip.date) parts.push(clip.date.slice(0, 4)); // the year alone; the full timestamp is noise
  const ms = stat?.durationMs ?? clip.durationMs;
  const height = stat?.height ?? clip.height;
  if (ms) parts.push(durationLabel(ms));
  if (height) parts.push(`${height}p`);
  return parts;
};

const SourceSearch = ({
  results,
  total,
  stats,
  onVisible,
  loadingStats,
  query,
  onQueryChange,
  onSearch,
  onPreview,
  onQueue,
  queueStatus,
  queueing,
  searching,
  error,
  className,
}: SourceSearchProps) => {
  const loadingSet = new Set(loadingStats ?? []);
  const resultKey = results.map((result) => result.id).join(",");
  const [reveal, setReveal] = useState({ key: resultKey, count: RESULT_STEP });
  const visibleCount = reveal.key === resultKey ? reveal.count : RESULT_STEP;
  const visibleResults = results.slice(0, visibleCount);
  const remaining = results.length - visibleResults.length;

  // Report which rows are on screen so the caller can fetch their stats.
  //
  // ⚠ An IntersectionObserver rather than "all of them": each id is a real ~1.8s upstream
  // request, so asking for a whole page up front is the 22.6s stall this design exists to
  // avoid. Rows below the fold cost nothing until scrolled to.
  //
  // ⚠ Falls back to reporting EVERY row where IntersectionObserver is unavailable (jsdom, and
  // any browser old enough to lack it). That is the correct degradation — the panel works and
  // is merely less frugal — and it is why the caller must de-duplicate rather than trusting
  // this to fire once per id.
  const listRef = useRef<HTMLUListElement>(null);
  // ⚠ A STRING of the ids, and the effect depends on it alone. `results` is a fresh array on
  // every render, so depending on it re-creates the observer continuously — and listing both
  // (which a first cut did, with a comment claiming otherwise) makes the string pointless,
  // because the array still changes every time. Biome caught the contradiction.
  const ids = visibleResults.map((r) => r.id).join(",");
  useEffect(() => {
    const rowIds = ids ? ids.split(",") : [];
    if (!onVisible || rowIds.length === 0) return;
    const root = listRef.current;
    if (!root || typeof IntersectionObserver === "undefined") {
      onVisible(rowIds);
      return;
    }
    const seen = new Set<string>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const id = (entry.target as HTMLElement).dataset.clipId;
          if (entry.isIntersecting && id) seen.add(id);
        }
        if (seen.size > 0) onVisible([...seen]);
      },
      // A margin, so a row's stats are in flight before it reaches the viewport — the request
      // takes ~1.8s, which is long enough that waiting for the row to actually appear would
      // show "—" first and fill in visibly late.
      { root: null, rootMargin: "200px" },
    );
    for (const li of root.querySelectorAll("[data-clip-id]")) observer.observe(li);
    return () => observer.disconnect();
  }, [ids, onVisible]);

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      {/* A real form, so Enter submits — this is a search box, and needing the mouse to run it
          is the kind of papercut that makes a surface feel broken. */}
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          onSearch();
        }}
      >
        <div className="min-w-0 flex-1">
          <Input
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Search this source…"
            // The visible label lives in the tab heading above, so the control carries its own
            // accessible name rather than relying on placeholder text (which is not a label).
            aria-label="Search this source"
          />
        </div>
        <Button type="submit" disabled={searching || query.trim().length < 2}>
          <Search className="size-4" aria-hidden />
          {searching ? "Searching…" : "Search"}
        </Button>
      </form>

      {error && <p className="text-onair-300 text-sm">{error}</p>}

      {results.length > 0 && (
        <>
          <Caption>
            {total != null && total > visibleResults.length
              ? `Showing ${visibleResults.length} of ${total} matches`
              : pluralize(visibleResults.length, "match", "matches")}
          </Caption>

          <ul ref={listRef} className="flex flex-col gap-2">
            {visibleResults.map((clip) => {
              const acquisition = queueStatus?.[clip.id];
              const parts = metaParts(clip, stats?.[clip.id]);
              return (
                <li
                  key={clip.id}
                  data-clip-id={clip.id}
                  className="flex flex-wrap items-center gap-3 rounded-lg border border-border px-3 py-2"
                >
                  {/* Archive's own thumbnail service — a stable URL, no API call. Rendered only
                      when present, following the clip card: a grid of identical grey
                      placeholders reads as a broken page rather than an absent nicety. */}
                  {clip.thumbnailUrl && (
                    <img
                      src={clip.thumbnailUrl}
                      // Empty alt: the title is the very next element, so describing the frame
                      // would make a screen reader announce the same item twice.
                      alt=""
                      className="h-10 w-16 shrink-0 rounded bg-static-800 object-cover"
                      loading="lazy"
                    />
                  )}

                  <div className="min-w-0 flex-1">
                    {/* The item's own page, so an operator can look before committing. Opens
                        away from Loomarr, so it says so for anyone not watching the status bar. */}
                    <a
                      href={clip.url}
                      target="_blank"
                      rel="noreferrer"
                      className="block truncate text-sm hover:underline"
                    >
                      {clip.title || clip.id}
                    </a>
                    {/* ⚠ "checking…" only while a request is genuinely in flight. Once it has
                        answered, an unknown duration renders as nothing at all — a permanent
                        spinner would promise a value that is never coming, and "0:00" would
                        claim the clip is empty. */}
                    {(parts.length > 0 || loadingSet.has(clip.id)) && (
                      <Caption className="mt-0.5 block">
                        {[...parts, ...(loadingSet.has(clip.id) ? ["checking…"] : [])].join(" · ")}
                      </Caption>
                    )}
                  </div>

                  <div className="ml-auto flex shrink-0 items-center gap-1">
                    {onPreview && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => onPreview(clip)}
                        aria-label={`Preview: ${clip.title || clip.id}`}
                      >
                        <Play aria-hidden />
                        Preview
                      </Button>
                    )}
                    {acquisition === "queued" || acquisition === "running" ? (
                      <span className="flex shrink-0 items-center gap-1 text-signal text-sm">
                        <Loader2 className="size-4 animate-spin" aria-hidden />
                        Downloading…
                      </span>
                    ) : acquisition === "success" ? (
                      <span className="flex shrink-0 items-center gap-1 text-signal text-sm">
                        <Check className="size-4" aria-hidden />
                        Added · being checked
                      </span>
                    ) : acquisition === "error" ? (
                      <div className="flex shrink-0 items-center gap-2">
                        <span className="text-onair-300 text-sm">Couldn’t add</span>
                        <Button variant="ghost" onClick={() => onQueue(clip)} disabled={queueing != null}>
                          {queueing === clip.id ? "Queueing…" : "Try again"}
                        </Button>
                      </div>
                    ) : (
                      <Button
                        variant="ghost"
                        onClick={() => onQueue(clip)}
                        disabled={queueing != null}
                        // The row's own link carries the title, so the button names what it acts on
                        // or a screen-reader user hears a list of identical "Queue download"s.
                        aria-label={`Queue download: ${clip.title || clip.id}`}
                      >
                        {queueing === clip.id ? "Queueing…" : "Queue download"}
                      </Button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>

          {remaining > 0 && (
            <Button
              type="button"
              variant="outline"
              onClick={() =>
                setReveal({
                  key: resultKey,
                  count: Math.min(visibleResults.length + RESULT_STEP, results.length),
                })
              }
            >
              {`Show ${Math.min(RESULT_STEP, remaining)} more`}
            </Button>
          )}
        </>
      )}

      {/* This is prose, not machine metadata, so it must not use the monospace Caption component. */}
      <p className="text-muted-foreground text-xs">
        Searching doesn’t download anything. Choose Queue download when you find a clip you want.
      </p>
    </div>
  );
};

export { SourceSearch };
