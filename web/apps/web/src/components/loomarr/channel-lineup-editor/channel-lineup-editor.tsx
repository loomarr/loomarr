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
import type { LineupEntryDTO, SearchCandidate } from "@loomarr/api";
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

// One row: drag handle, name/year, a "Pending" badge when the key isn't in the library
// yet (it'll play once the title lands — never an error state), and remove. Not
// exported from the barrel — same "internal row" shape as RefineReview's Row or
// ProposalReview's ItemRow — so it needs no story of its own.
const SortableLineupRow = ({
  entry,
  pending,
  disabled,
  onRemove,
}: {
  entry: LineupEntryDTO;
  pending: boolean;
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
          {pending && <Badge variant="tune">Pending</Badge>}
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
// "Pending" (not yet in the library — the backend renders it as a pending slot until
// the title lands, rather than an error) is tracked as LOCAL UI state, not re-derived
// from `lineup` on every render: LineupEntryDTO is deliberately {key, name, year,
// genres} only (the backend's own read-back never carries an availability flag — that
// richer per-entry metadata lives server-side, matched by key), so the only moment
// availability truth is ever visible client-side is the search result that added a
// title. A pick added `!inLibrary` is remembered as pending here; removing it (or the
// page loading fresh, where this state starts empty) clears that memory rather than
// guessing at entries the editor was never told the status of.
const ChannelLineupEditor = ({ channelId, lineup, className }: ChannelLineupEditorProps) => {
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");
  const [pendingKeys, setPendingKeys] = useState<Set<string>>(new Set());

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
                  pending={pendingKeys.has(entry.key)}
                  disabled={isPending}
                  onRemove={() => {
                    remove(entry.key);
                    setPendingKeys((prev) => {
                      if (!prev.has(entry.key)) return prev;
                      const next = new Set(prev);
                      next.delete(entry.key);
                      return next;
                    });
                  }}
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
              const committed = add({ key: result.id, name: candidate.name, year: candidate.year });
              // Pending is the "not in the library yet" fact FROM THE SEARCH RESULT
              // that added it — the only moment that truth is visible client-side, since
              // LineupEntryDTO's read-back never carries it (see the component header).
              if (committed && !candidate.inLibrary) {
                setPendingKeys((prev) => new Set(prev).add(result.id));
              }
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
