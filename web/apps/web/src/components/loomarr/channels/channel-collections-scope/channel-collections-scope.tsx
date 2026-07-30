import { ApiError, type LibraryCollection, libraryApi } from "@loomarr/api";
import { Plus, X } from "lucide-react";
import { useState } from "react";
import { Button, Label } from "@/components/ui";
import { cn } from "@/lib";
import { FieldHelp } from "../../feedback";
import { SearchCommand } from "../../shell";
import type { ChannelCollectionsScopeProps } from "./channel-collections-scope.type";

// ChannelCollectionsScope — `policy.scope.collections`, the "only what I shelved" narrowing
// (programming-design §2.2). The collections are the operator's own media-server BoxSets.
//
// ⚠ **This began as a checkbox list, and the real stack disproved the premise.** The design
// note said "a small closed set the operator curates by hand", so showing all of them looked
// obviously right. The maintainer's Emby returns **125** — Kometa and similar tools generate a
// franchise collection per movie series in bulk — which rendered as a wall of near-identical
// rows taller than every other control on the Programming page combined. Bounding it in a
// scroll box only contained the mess; 125 checkboxes is still 125 checkboxes to scan.
//
// So it is now the SAME type-ahead + chips shape as ChannelSeriesScope next to it: nothing
// listed until you type, picks become removable chips, and the field is two rows regardless of
// whether the library holds 5 collections or 500. One pattern to learn per page.
//
// ⚠ **Franchise collections are offered but DEPRIORITIZED, never hidden.** 112 of the 125 are
// auto-generated one-per-series ("Alien Collection", "Batman Collection"); the 13 that carry a
// decision are things like "80s Best" and "Oscars 2024". Franchises are still valid scopes and
// some operators curate them deliberately, so filtering them out server-side would remove a
// real capability on a NAME heuristic — and silently drop a shelf someone deliberately named
// that way. Instead they sort last and are labelled, so the curated lists lead.
//
// ⚠ **The accessible name is `aria-label`, NEVER an `sr-only` <legend>.** `sr-only` is
// `position:absolute`, and an absolutely-positioned <legend> escapes its fieldset's containing
// block: it laid out at y=1253, dragged `documentElement.scrollHeight` to 1254 against a 900px
// viewport, and made the WHOLE WINDOW scroll behind the fixed sidebar — content sliding past
// the end of the nav. The shell is `h-screen overflow-hidden` and every element in the chain
// measured exactly 900; one escaping absolute child defeated all of it.

// A franchise collection is Emby's auto-generated per-series grouping, which in practice is
// named "<Series> Collection". A heuristic, and deliberately only used for ORDERING and a
// label — never to drop anything, because being wrong then costs the operator nothing.
const isFranchise = (name: string): boolean => name.trim().endsWith("Collection");

const ChannelCollectionsScope = ({ policy, onChange, className }: ChannelCollectionsScopeProps) => {
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");

  const selected = policy.scope?.collections ?? [];
  const q = libraryApi.useListLibraryCollections();

  // 501 is "no library configured", which is a different thing from "no collections" and gets
  // no control at all — the Connections page is where that is fixed.
  //
  // ⚠ Read off the thrown ApiError, NOT `q.data.status`. A 501 is an error response, so it
  // never populates `data` — checking there leaves `unavailable` permanently false and the
  // component falls through to its EMPTY state, telling an operator with no media server to
  // "make a collection in your media server". Caught by a test, not by review; the same
  // detection ChannelIconField already uses for an unconfigured TMDB.
  const unavailable = q.error instanceof ApiError && q.error.status === 501;
  const collections: LibraryCollection[] = q.data?.status === 200 ? (q.data.data.collections ?? []) : [];

  const byId = new Map(collections.map((c) => [c.id, c]));
  // A chip falls back to the raw id: the policy stores ids, so a collection deleted in Emby
  // still renders as something removable rather than vanishing from a scope that still filters.
  const labelOf = (id: string): string => byId.get(id)?.name ?? id;

  const commit = (next: string[]) => {
    // An emptied list must send [] (not undefined): `collections` is omitempty, so dropping the
    // key would leave the previous restriction in place and the field would appear to clear
    // while still filtering. Same trap as scope.series and runtimeMax.
    onChange({ ...policy, scope: { ...policy.scope, collections: next } });
  };

  const term = query.trim().toLowerCase();
  const matches = collections
    .filter((c) => !selected.includes(c.id) && (term === "" || c.name.toLowerCase().includes(term)))
    // Curated lists first, then franchises; alphabetical within each. This is the whole point of
    // the classification — "80s Best" must not be buried under 112 franchise rows.
    .sort((a, b) => {
      const fa = isFranchise(a.name) ? 1 : 0;
      const fb = isFranchise(b.name) ? 1 : 0;
      return fa !== fb ? fa - fb : a.name.localeCompare(b.name);
    })
    // ⚠ Deliberately NOT capped at a handful. SearchCommand's own list is `max-h-80
    // overflow-y-auto` with keyboard-follow, so it scrolls — a small `.slice()` here would
    // silently hide matches the operator can see no way to reach, which is worse than a long
    // scrollable list. An empty query therefore browses the WHOLE library (curated first),
    // which is what makes this usable without knowing a name up front. The bound is high
    // enough to be a runaway guard rather than a product decision.
    .slice(0, 200);

  if (unavailable) return null;

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="flex items-center gap-1.5">
        <Label>Only these collections</Label>
        <FieldHelp label="Only these collections">
          Restrict the channel to titles in the collections you keep in your media server. Leave empty for no
          restriction. Changes to a collection are picked up the next time the channel rebuilds.
        </FieldHelp>
      </div>

      {q.isError ? (
        // A failure must NOT read as "you have none" — that would send the operator off to
        // create a collection they may already have.
        <p className="text-muted-foreground text-sm">
          Couldn't load your collections. Check the media server connection in Settings.
        </p>
      ) : (
        <fieldset className="flex flex-col gap-2" aria-label="Only these collections">
          {selected.length > 0 && (
            <ul className="flex flex-wrap gap-2">
              {selected.map((id) => (
                <li
                  key={id}
                  className="flex items-center gap-1.5 rounded-md border border-border bg-muted/40 px-2 py-1 text-sm"
                >
                  <span>{labelOf(id)}</span>
                  <button
                    type="button"
                    aria-label={`Remove ${labelOf(id)}`}
                    className="text-muted-foreground hover:text-foreground"
                    onClick={() => commit(selected.filter((c) => c !== id))}
                  >
                    <X className="size-3.5" aria-hidden />
                  </button>
                </li>
              ))}
            </ul>
          )}

          {adding ? (
            <div className="flex flex-col gap-2">
              <SearchCommand
                query={query}
                onQueryChange={setQuery}
                loading={q.isPending}
                // The shared default ("Search titles, channels, help…") describes the ⌘K
                // palette, none of which this searches.
                placeholder={`Search ${collections.length} collections…`}
                // ⚠ Escape closes the picker. SearchCommand does not bind it by default (the
                // ⌘K palette binds it window-level and would close twice), and the Cancel
                // button below is a POINTER affordance — a keyboard user presses Escape first
                // and, before this, nothing happened.
                onEscape={() => {
                  setQuery("");
                  setAdding(false);
                }}
                results={matches.map((c) => ({
                  id: c.id,
                  scope: "library" as const,
                  name: c.name,
                  // Says WHY a row is ranked where it is, so the ordering reads as deliberate
                  // rather than arbitrary.
                  ...(isFranchise(c.name) ? { meta: "franchise" } : {}),
                }))}
                onSelect={(r) => {
                  commit([...selected, r.id]);
                  setQuery("");
                  setAdding(false);
                }}
              />
              <Button
                variant="ghost"
                size="sm"
                className="self-start"
                onClick={() => {
                  setQuery("");
                  setAdding(false);
                }}
              >
                Cancel
              </Button>
            </div>
          ) : collections.length === 0 && !q.isPending ? (
            // A real answer, not a failure: a library is connected but nothing is shelved.
            <p className="text-muted-foreground text-sm">
              You have no collections yet. Make one in your media server and it will show up here.
            </p>
          ) : (
            <Button variant="outline" size="sm" className="self-start" onClick={() => setAdding(true)}>
              <Plus aria-hidden />
              Add a collection…
            </Button>
          )}
        </fieldset>
      )}
    </div>
  );
};

export { ChannelCollectionsScope };
