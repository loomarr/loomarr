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
import type { LineupEntryDTO, LineupEntryDTOState, SearchCandidate } from "@loomarr/api";
import { searchApi } from "@loomarr/api";
import { GripVertical, Plus, X } from "lucide-react";
import { useState } from "react";
import { useChannelLineup } from "@/channels";
import { Badge, Button, Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui";
import { cn } from "@/lib";
import { SearchCommand } from "../search-command";
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

// The badge for a lineup entry's acquisition state (§7). `available` is the normal case
// and wears no badge — a badge only calls out a title that isn't playing yet: `pending`
// (added, nothing requested it), `acquiring` (on its way), or `unavailable` (gave up).
// `unavailable` is the one problem state, so it wears the `onair` (attention) colour;
// pending/acquiring are calm "not yet" states, never errors. The state is server-derived
// (durable across reloads); an optimistic add seeds it from the search result's inLibrary.
const LineupStateBadge = ({ state }: { state?: LineupEntryDTOState }) => {
  switch (state) {
    case "pending":
      return <Badge variant="tune">Pending</Badge>;
    case "acquiring":
      return <Badge variant="tune">Acquiring…</Badge>;
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
}: {
  entry: LineupEntryDTO;
  disabled: boolean;
  onRemove: () => void;
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: entry.key,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

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
          <LineupStateBadge state={entry.state} />
        </div>
      </div>

      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="size-7 shrink-0"
            disabled={disabled}
            aria-label={`Remove ${entry.name}`}
            onClick={onRemove}
          >
            <X aria-hidden />
          </Button>
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
const ChannelLineupEditor = ({ channelId, lineup, className }: ChannelLineupEditorProps) => {
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");

  const { entries, isPending, add, remove, reorder } = useChannelLineup(channelId, lineup);

  const search = searchApi.useSearch(
    { q: query, scope: "all", limit: 8 },
    { query: { enabled: adding && query.trim().length > 1 } },
  );
  const candidates = search.data?.status === 200 ? (search.data.data.candidates ?? []) : [];

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

      {entries.length === 0 ? (
        <p className="rounded-md border border-border border-dashed px-3 py-6 text-center text-muted-foreground text-sm">
          Nothing in the lineup yet — add a title below.
        </p>
      ) : (
        <DndContext sensors={sensors} onDragEnd={onDragEnd}>
          <SortableContext items={entries.map((e) => e.key)} strategy={verticalListSortingStrategy}>
            <ul className="flex flex-col gap-2">
              {entries.map((entry) => (
                <SortableLineupRow
                  key={entry.key}
                  entry={entry}
                  disabled={isPending}
                  onRemove={() => remove(entry.key)}
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
