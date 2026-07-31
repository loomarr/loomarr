import { type ClipDTO, fillerApi, unwrap } from "@loomarr/api";
import { formatClipDuration } from "@loomarr/core";
import { Plus, X } from "lucide-react";
import { useState } from "react";
import { SearchCommand } from "@/components/loomarr";
import { Button, Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui";
import { cn } from "@/lib";

// FillerClipList — one override list ("Always include" pins, or "Never include" excludes).
// Add a clip via the filler search (SearchCommand's `clips` scope, fed by useListFiller),
// remove with the X. Clips are identified by TunarrProgramID — the same id the selection
// stores and the assembler pins/excludes on. A resolved clip carries its name/duration for
// the row; an id already saved but not in the current search page still renders (as a bare
// id row) so an override is never invisible just because it's off the current result page.
//
// Presentational + controlled: the parent owns `ids` (draft.pinned / draft.excluded) and
// applies whatever add/remove hands back. `resolve` maps a saved id to its clip when known.
const FillerClipList = ({
  label,
  hint,
  ids,
  onChange,
  resolve,
  disabled,
  excludeIds,
}: {
  label: string;
  hint: string;
  ids: string[];
  onChange: (next: string[] | undefined) => void;
  // Look up a clip's display fields by id (from a shared catalog fetch in the parent).
  resolve: (id: string) => ClipDTO | undefined;
  disabled?: boolean;
  // Ids to leave OUT of the add-search results — the counterpart list's ids, so a clip
  // can't be pinned and excluded at once from the UI (the backend resolves the conflict as
  // exclude-wins, but offering both invites confusion).
  excludeIds: string[];
}) => {
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");

  const search = fillerApi.useListFiller(
    { q: query },
    { query: { enabled: adding && query.trim().length > 1 } },
  );
  const clips = unwrap(search.data, (b) => b.clips) ?? [];

  const chosen = new Set(ids);
  const blocked = new Set(excludeIds);

  const remove = (id: string) => {
    const next = ids.filter((x) => x !== id);
    onChange(next.length === 0 ? undefined : next);
  };
  const add = (id: string) => {
    if (chosen.has(id)) return;
    onChange([...ids, id]);
    setAdding(false);
    setQuery("");
  };

  return (
    <div className="flex flex-col gap-2">
      <div>
        <p className="font-medium text-sm">{label}</p>
        <p className="text-muted-foreground text-xs">{hint}</p>
      </div>

      {ids.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {ids.map((id) => {
            const clip = resolve(id);
            return (
              <li
                key={id}
                className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2"
              >
                <div className="min-w-0 flex-1">
                  <span className="truncate font-medium text-sm">{clip?.name ?? id}</span>
                  {clip ? (
                    <span className="ml-2 font-mono text-static-400 text-xs">
                      {formatClipDuration(clip.durationMs)}
                    </span>
                  ) : null}
                </div>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-7 shrink-0"
                      disabled={disabled}
                      aria-label={`Remove ${clip?.name ?? id}`}
                      onClick={() => remove(id)}
                    >
                      <X aria-hidden />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Remove</TooltipContent>
                </Tooltip>
              </li>
            );
          })}
        </ul>
      )}

      {adding ? (
        <div className="flex flex-col gap-2">
          <SearchCommand
            query={query}
            onQueryChange={setQuery}
            loading={search.isFetching}
            // Escape does what Cancel does. Opt-in because the ⌘K palette binds Escape at the
            // window level and a default binding would close both (see onEscape).
            onEscape={() => {
              setAdding(false);
              setQuery("");
            }}
            // Only clips not already in THIS list and not in the counterpart list are
            // offered — the same filter-don't-disable choice the lineup editor makes.
            results={clips
              .filter((c) => !chosen.has(c.path) && !blocked.has(c.path))
              .map((c) => ({
                id: c.path,
                scope: "clips" as const,
                name: c.name,
                meta: formatClipDuration(c.durationMs),
              }))}
            onSelect={(r) => add(r.id)}
          />
          <Button
            variant="ghost"
            size="sm"
            className="self-start"
            onClick={() => {
              setAdding(false);
              setQuery("");
            }}
          >
            Cancel
          </Button>
        </div>
      ) : (
        <Button
          variant="outline"
          size="sm"
          className={cn("self-start", disabled && "pointer-events-none opacity-50")}
          disabled={disabled}
          onClick={() => setAdding(true)}
        >
          <Plus aria-hidden />
          Add a clip…
        </Button>
      )}
    </div>
  );
};

export { FillerClipList };
