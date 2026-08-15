import {
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import * as searchApi from "@loomarr/api/endpoints/search";
import type { LineupEntryDTO } from "@loomarr/api/models/lineupEntryDTO";
import type { LineupEntryDTOState } from "@loomarr/api/models/lineupEntryDTOState";
import type { SearchCandidate } from "@loomarr/api/models/searchCandidate";
import { unwrap } from "@loomarr/api/unwrap";
import { GripVertical, Plus, X } from "lucide-react";
import { useState } from "react";
import { useChannelLineup } from "@/channels/use-channel-lineup";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { SearchCommand } from "../../shell";
import type { ChannelLineupEditorProps } from "./channel-lineup-editor.type";

// keyOf — the SAME provisioning-key derivation RefineReview uses (its `keyOf`, over
// ProposalItem), just over a SearchCandidate: series+tvdbId → series:tvdb:{id}, else
// {mediaType}:tmdb:{id}. One title has one key no matter which endpoint described it, so
// an add here lands on the identical identity the refine diff and the backend both use.
const keyOf = (c: SearchCandidate): string => {
  if (c.mediaType === "series" && c.tvdbId) return `series:tvdb:${c.tvdbId}`;
  if (c.tmdbId) return `${c.mediaType}:tmdb:${c.tmdbId}`;
  return `name:${c.name.trim().toLowerCase()}:${c.year ?? ""}`;
};

// The badge for a lineup entry's acquisition state (§7). `available` is the normal case and
// wears no badge — a badge only calls out a title that isn't playing yet: `pending` (added,
// nothing requested it), `acquiring` (on its way), or `unavailable` (gave up). `unavailable`
// is the one problem state, so it wears the `onair` (attention) colour; pending/acquiring are
// calm "not yet" states, never errors. For `acquiring`, the coarse acquisition status from the
// queue poll (§18.1 — "Downloading" / "Partly available" via Seerr, or the arr's status) is
// shown as the chip text when known, so the row says what stage it's at without a progress bar;
// it falls back to "Acquiring…" before any status has landed. Server-derived (durable across
// reloads); an optimistic add seeds state from the search result's inLibrary.
const LineupStateBadge = ({ state, status }: { state?: LineupEntryDTOState; status?: string }) => {
  switch (state) {
    case "pending":
      return <Badge variant="tune">Pending</Badge>;
    case "acquiring":
      return <Badge variant="tune">{status ? `${status}…` : "Acquiring…"}</Badge>;
    case "unavailable":
      return <Badge variant="onair">Unavailable</Badge>;
    default:
      return null; // available (or unresolved) → no badge
  }
};

// One row: drag handle, name/year, a state badge when the title isn't playing yet (see
// LineupStateBadge — never an error state except a genuine "gave up"), and remove. Not
// exported from the barrel — same "internal row" shape as RefineReview's Row or
// ProposalReview's ItemRow — so it needs no story of its own.
const SortableLineupRow = ({
  entry,
  disabled,
  onRemove,
  onSeasonChange,
}: {
  entry: LineupEntryDTO;
  disabled: boolean;
  onRemove: () => void;
  // Commit a season-window edit for THIS entry (series only). A blank/0 field clears that
  // bound — "no constraint", never season 0 (§9). Omitted for a movie row.
  onSeasonChange?: (patch: { seasonMin?: number; seasonMax?: number }) => void;
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: entry.key,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  // Only a SERIES carries a season window (§9 series expansion) — a movie is one title.
  const isSeries = entry.key.startsWith("series:");
  const seasonInput = (bound: "seasonMin" | "seasonMax", value: number | undefined, placeholder: string) => (
    <input
      type="number"
      min={1}
      disabled={disabled}
      defaultValue={value || ""}
      placeholder={placeholder}
      aria-label={`${entry.name} ${bound === "seasonMin" ? "first" : "last"} season`}
      className="w-16 rounded border border-border bg-background px-2 py-1 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
      onBlur={(e) => {
        const next = e.target.value === "" ? undefined : Number(e.target.value);
        if (next === (value || undefined)) return;
        onSeasonChange?.({ [bound]: next });
      }}
    />
  );

  return (
    <li
      ref={setNodeRef}
      style={style}
      className={cn(
        "flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2.5",
        isDragging && "z-10 opacity-70 shadow-md",
      )}
    >
      <button
        type="button"
        {...attributes}
        {...listeners}
        disabled={disabled}
        aria-label={`Reorder ${entry.name}`}
        className="flex size-7 shrink-0 cursor-grab items-center justify-center rounded text-static-400 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing disabled:cursor-not-allowed disabled:opacity-50"
      >
        <GripVertical className="size-4" aria-hidden />
      </button>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate font-medium text-sm">{entry.name}</span>
          {entry.year ? <span className="font-mono text-static-400 text-xs">{entry.year}</span> : null}
          <LineupStateBadge state={entry.state} status={entry.downloadStatus} />
        </div>
        {isSeries && onSeasonChange && (
          <div className="mt-1.5 flex items-center gap-1.5">
            <span className="text-muted-foreground text-xs">Seasons</span>
            {seasonInput("seasonMin", entry.seasonMin, "1")}
            <span className="text-muted-foreground text-xs" aria-hidden>
              –
            </span>
            {seasonInput("seasonMax", entry.seasonMax, "latest")}
          </div>
        )}
      </div>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="ghost"
              size="icon"
              className="size-7 shrink-0"
              disabled={disabled}
              aria-label={`Remove ${entry.name}`}
              onClick={onRemove}
            />
          }
        >
          <X aria-hidden />
        </TooltipTrigger>
        <TooltipContent>Remove from lineup</TooltipContent>
      </Tooltip>
    </li>
  );
};

// ChannelLineupEditor — the manual lineup editor on a channel's detail page (phase 3 of
// editable channels): add via search, remove, reorder by drag. This is the direct path
// alongside "Refine with AI" — no diff to review, no rebuild step. Every op (add/
// remove/reorder) commits IMMEDIATELY via useChannelLineup, which sends the backend's
// whole-list replace and reconciles server-side; the channel page's own SSE
// subscription + query invalidate is what brings this back in line with the server, the
// same as every other edit on that page.
//
// Each entry's acquisition state (pending / acquiring / unavailable / available) rides on
// the entry itself — LineupEntryDTO.state, resolved server-side from the provision Record
// (§7 GET) — so the badge is DURABLE across reloads, not guessed from client-only memory.
// An optimistic add seeds that state from the search result's `inLibrary` for instant
// feedback; the refetch the add triggers replaces it with the server's authoritative value.
// LineupSort is how the list is DISPLAYED — never how it plays.
//
// "Play order" is the stored order, which under a sequential channel IS the play order and is
// what drag-to-reorder edits. The other views are read-only lenses over the same list: a channel
// that has been auto-curated for weeks accumulates its newest titles at the BOTTOM, so the
// default view shows the original lineup and buries everything that has been added since —
// reported as "why do I still see the same old movies".
type LineupSort = "order" | "newest" | "year";

const ChannelLineupEditor = ({ channelId, lineup, className }: ChannelLineupEditorProps) => {
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<LineupSort>("order");

  const { entries, isPending, add, remove, reorder, updateEntry } = useChannelLineup(channelId, lineup);

  const search = searchApi.useSearch(
    { q: query, scope: "all", limit: 8 },
    { query: { enabled: adding && query.trim().length > 1 } },
  );
  const candidates = unwrap(search.data, (b) => b.candidates) ?? [];

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  // Commit ONLY on drop, not on every move over — dragging itself is local reorder
  // preview handled by dnd-kit/SortableContext; the PATCH fires once, here.
  const onDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const from = entries.findIndex((e) => e.key === active.id);
    const to = entries.findIndex((e) => e.key === over.id);
    if (from === -1 || to === -1) return;
    reorder(from, to);
  };

  const existingKeys = new Set(entries.map((e) => e.key));

  // The DISPLAY list. `order` hands back the stored array untouched so drag-to-reorder keeps
  // operating on real indices; the other lenses copy before sorting, because mutating `entries`
  // would corrupt the order the PATCH sends back.
  //
  // "Newest" is the stored order reversed rather than a timestamp sort: a lineup entry carries
  // no added-at (auto-curate unions onto the end), so append position IS the recency signal —
  // and reversing it is exact rather than approximate.
  const shown =
    sort === "newest"
      ? [...entries].reverse()
      : sort === "year"
        ? [...entries].sort((a, b) => (b.year ?? 0) - (a.year ?? 0))
        : entries;

  // Dragging is only coherent while the list shows its real order — a drop in a sorted view
  // would move the item to a position the user cannot see. Rather than silently misplace it,
  // reordering is disabled outside "Play order" and the control says why.
  const dragEnabled = sort === "order";

  const closeAdd = () => {
    setAdding(false);
    setQuery("");
  };

  return (
    <section className={cn("flex flex-col gap-3 rounded-lg border border-border p-4", className)}>
      <div>
        <h2 className="font-semibold text-lg">Lineup</h2>
        <p className="text-muted-foreground text-sm">
          Add, remove, and reorder the titles this channel plays. Changes save immediately.
        </p>
      </div>

      {/* Sort is a VIEW, not an edit — nothing here writes. Newest-first exists because
          auto-curate appends, so the default play order shows the oldest picks first and hides
          everything added since. */}
      {entries.length > 1 && (
        <div className="flex flex-wrap items-center gap-1.75">
          {/* tracking-[0.06em] overrides `shout`'s tracking-wide (0.025em) — the same toolbar
              label voice as the guide's "Start". */}
          <Caption shout className="tracking-[0.06em]">
            Show
          </Caption>
          {(
            [
              ["order", "Play order"],
              ["newest", "Newest first"],
              ["year", "Year"],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              onClick={() => setSort(value)}
              aria-pressed={sort === value}
              className={cn(
                "cursor-pointer rounded-md border px-2.5 py-0.5 font-mono text-2xs transition-colors",
                sort === value
                  ? "border-signal bg-signal-tint-15 text-signal"
                  : "border-border text-static-400 hover:border-static-400",
              )}
            >
              {label}
            </button>
          ))}
          {!dragEnabled && <span className="text-2xs text-static-400">Switch to Play order to reorder</span>}
        </div>
      )}

      {entries.length === 0 ? (
        <p className="rounded-md border border-border border-dashed px-3 py-6 text-center text-muted-foreground text-sm">
          Nothing in the lineup yet. Add a title below.
        </p>
      ) : (
        <DndContext sensors={sensors} onDragEnd={onDragEnd}>
          <SortableContext items={shown.map((e) => e.key)} strategy={verticalListSortingStrategy}>
            {/* Cap the lineup at a comfortable height and scroll INSIDE once it's long — a
                channel with many titles otherwise runs the page off. max-height means a short
                lineup isn't boxed (the scroll only appears when it overflows). scroll-thin
                (styles.css) is the same token-keyed slim scrollbar the cycle preview uses.
                dnd-kit auto-scrolls this container during a drag toward its edges. */}
            <ul className="scroll-thin flex max-h-128 flex-col gap-2 overflow-y-auto">
              {shown.map((entry) => (
                <SortableLineupRow
                  key={entry.key}
                  entry={entry}
                  disabled={isPending || !dragEnabled}
                  onRemove={() => remove(entry.key)}
                  onSeasonChange={(patch) => updateEntry(entry.key, patch)}
                />
              ))}
            </ul>
          </SortableContext>
        </DndContext>
      )}

      {adding ? (
        <div className="flex flex-col gap-2">
          <SearchCommand
            query={query}
            onQueryChange={setQuery}
            loading={search.isFetching}
            // Escape does what Cancel does — the same `closeAdd`, so the two paths cannot
            // drift. Opt-in because the ⌘K palette binds Escape window-level (see onEscape).
            onEscape={closeAdd}
            // Two kinds of candidate are left out entirely rather than shown-then-rejected:
            // (1) one already on the lineup (the backend 422s on a duplicate key), and
            // (2) one with no usable id, whose key falls back to `name:…` — the backend's
            // provision.ParseKey rejects that shape (a title with no tmdb/tvdb id can't be
            // provisioned or resolved anyway), so offering it would let a user pick it, see
            // the optimistic add appear, then watch the whole list snap back on the 422.
            // SearchCommand has no per-item disabled affordance, so filtering is the clean
            // prevention: an unaddable title simply isn't offered.
            results={candidates
              .filter((c) => {
                const k = keyOf(c);
                return !k.startsWith("name:") && !existingKeys.has(k);
              })
              .map((c) => ({
                id: keyOf(c),
                scope: c.inLibrary ? "library" : "tmdb",
                name: c.name,
                ...(c.year ? { meta: String(c.year) } : {}),
                inLibrary: c.inLibrary,
              }))}
            onSelect={(result) => {
              const candidate = candidates.find((c) => keyOf(c) === result.id);
              if (!candidate) return;
              // Seed the optimistic `state` from the candidate's library membership: a pick
              // already in the library reads `available` immediately, one that isn't reads
              // `pending`. The server re-derives the authoritative state (from the provision
              // Record) on the refetch this add triggers — so the badge is right instantly
              // AND durable across reloads, no client-only pending tracking.
              add({
                key: result.id,
                name: candidate.name,
                year: candidate.year,
                state: candidate.inLibrary ? "available" : "pending",
              });
              closeAdd();
            }}
          />
          <Button variant="ghost" size="sm" className="self-start" onClick={closeAdd}>
            Cancel
          </Button>
        </div>
      ) : (
        <Button variant="outline" size="sm" className="self-start" onClick={() => setAdding(true)}>
          <Plus aria-hidden />
          Add a title…
        </Button>
      )}
    </section>
  );
};

export { ChannelLineupEditor };
