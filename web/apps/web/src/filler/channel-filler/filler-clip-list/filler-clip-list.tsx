import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatClipDuration } from "@loomarr/core/format";
import { Plus, X } from "lucide-react";
import { useState } from "react";
import { SearchCommand } from "@/components/loomarr/shell/search-command";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

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
  resolving,
  cap,
  disabled,
  excludeIds,
}: {
  label: string;
  hint: string;
  ids: string[];
  onChange: (next: string[] | undefined) => void;
  // Look up a clip's display fields by id (from a shared catalog fetch in the parent).
  resolve: (id: string) => ClipDTO | undefined;
  // Whether that lookup is still in flight. ⚠ Required to tell "not loaded yet" from "not in the
  // catalog any more" — `resolve` returns undefined for both, and rendering the second message
  // during the first state would accuse a perfectly good clip of having been deleted.
  resolving?: boolean;
  // cap is the most clips a single break can play (`filler.pod_max`). Passed only for the PIN
  // list, where exceeding it is the operator's decision to see; omitted elsewhere.
  cap?: number;
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

      {/* ⚠ **The clamp said nothing at all before this.** `filler.pod_max` caps how many clips one
          break plays; pin more than that and the extras are saved, honoured, and simply cannot all
          fit — with no count anywhere on the page. That is the same shape as the other V51f
          defects: a control that accepts input and quietly does less than it says.
          ⚠ Worded as "not all in ONE break" rather than "N will be dropped", because that is what
          actually happens: the pins are a priority pool the assembler draws from per break, so
          which ones appear varies by break rather than the list being truncated. */}
      {cap !== undefined && cap > 0 && ids.length > cap && (
        <p className="text-signal text-xs" data-testid="pod-max-clamp">
          A break plays at most {cap} {cap === 1 ? "clip" : "clips"}, so only some of these {ids.length}{" "}
          appear in any one break. Raise the limit in Filler settings to fit more.
        </p>
      )}

      {ids.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {ids.map((id) => {
            const clip = resolve(id);
            // ⚠ **A resolved-and-missing id is a DEAD override, not a clip with a long name.**
            // It used to render the raw 64-character content hash, and this file's own comment
            // called that "the honest signal that the override points at something no longer
            // airable" — honest to someone who already knew, unreadable to everyone else. The
            // assembler silently skips it, so nothing else says the pin does nothing.
            const gone = !clip && !resolving;
            return (
              <li
                key={id}
                className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2"
              >
                <div className="min-w-0 flex-1">
                  {gone ? (
                    <>
                      <span className="truncate font-medium text-onair-300 text-sm">
                        This clip is no longer in your catalog
                      </span>
                      {/* The hash still shown, small and secondary: it is the only handle an
                          operator has if they want to work out WHICH clip this was. */}
                      <span className="ml-2 truncate font-mono text-static-400 text-xs">
                        {id.slice(0, 12)}…
                      </span>
                    </>
                  ) : (
                    <>
                      <span className="truncate font-medium text-sm">{clip?.name ?? id}</span>
                      {clip ? (
                        <span className="ml-2 font-mono text-static-400 text-xs">
                          {formatClipDuration(clip.durationMs)}
                        </span>
                      ) : null}
                    </>
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
                        aria-label={
                          gone
                            ? "Remove this clip, which is no longer in your catalog"
                            : `Remove ${clip?.name ?? id}`
                        }
                        onClick={() => remove(id)}
                      />
                    }
                  >
                    <X aria-hidden />
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
              .filter((c) => !chosen.has(c.hash) && !blocked.has(c.hash))
              .map((c) => ({
                id: c.hash,
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
