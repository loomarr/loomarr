import { type SearchCandidate, searchApi } from "@loomarr/api";
import { Plus, X } from "lucide-react";
import { useState } from "react";
import { Button, Label } from "@/components/ui";
import { cn } from "@/lib";
import { FieldHelp } from "../../feedback";
import { SearchCommand } from "../../shell";
import type { ChannelSeriesScopeProps } from "./channel-series-scope.type";

// ChannelSeriesScope — `policy.scope.series`, the "only these shows" narrowing (§2).
//
// ORPHANED until now, and the audit was right that a text box would be "a door in name only":
// the field is `[]provision.Key` — RESOLVED ids, never names (schedule/policy.go:177) — so a
// typed title could not become a valid entry. It needs a search-backed picker, which is why it
// was filed as design work. In practice the design already exists: the lineup editor solved
// the same problem, so this reuses its `keyOf` derivation and the shared SearchCommand.

// keyOf, narrowed to the SERIES branch of the lineup editor's derivation. scope.series
// restricts to series, so a movie candidate can never produce a valid entry here.
// A series with no tvdbId yields "" and is filtered out rather than offered: the backend's
// provision.ParseKey rejects a `name:` fallback key, so offering it would let someone pick a
// title, then have the save 422.
const seriesKeyOf = (c: SearchCandidate): string => {
  if (c.mediaType !== "series") return "";
  if (c.tvdbId) return `series:tvdb:${c.tvdbId}`;
  if (c.tmdbId) return `series:tmdb:${c.tmdbId}`;
  return "";
};

// A stored key rendered for a human. The keys are `series:tvdb:1234` — accurate and unreadable
// — and the policy stores no display name, so the id is the honest thing to show rather than a
// guessed title. Kept compact: the source and the number are what identify it.
const labelOfKey = (key: string): string => {
  const parts = key.split(":");
  if (parts.length === 3) return `${parts[1]} ${parts[2]}`;
  return key;
};

const ChannelSeriesScope = ({ policy, onChange, className }: ChannelSeriesScopeProps) => {
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");

  const series = policy.scope?.series ?? [];

  const search = searchApi.useSearch(
    { q: query, scope: "all", limit: 8 },
    { query: { enabled: adding && query.trim().length > 1 } },
  );
  const candidates = search.data?.status === 200 ? (search.data.data.candidates ?? []) : [];

  const commit = (next: string[]) => {
    // An emptied list must send [] (not undefined): `series` is omitempty, so dropping the key
    // would leave the previous restriction in place and the field would appear to clear while
    // still filtering. Same trap as runtimeMax, one container up.
    onChange({ ...policy, scope: { ...policy.scope, series: next } });
  };

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="flex items-center gap-1.5">
        <Label>Only these shows</Label>
        <FieldHelp label="Only these shows">
          Restrict the channel to specific series. Leave empty for no restriction — the lineup decides on its
          own.
        </FieldHelp>
      </div>

      {series.length > 0 && (
        <ul className="flex flex-wrap gap-2">
          {series.map((key) => (
            <li
              key={key}
              className="flex items-center gap-1.5 rounded-md border border-border bg-muted/40 px-2 py-1 text-sm"
            >
              <span className="font-mono text-muted-foreground text-xs">{labelOfKey(key)}</span>
              <button
                type="button"
                aria-label={`Remove ${labelOfKey(key)}`}
                className="text-muted-foreground hover:text-foreground"
                onClick={() => commit(series.filter((k) => k !== key))}
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
            loading={search.isFetching}
            // Movies, already-picked series, and series with no resolvable id are left out
            // rather than shown-then-rejected — the same prevention the lineup editor uses.
            results={candidates
              .filter((c) => {
                const k = seriesKeyOf(c);
                return k !== "" && !series.includes(k);
              })
              .map((c) => ({
                id: seriesKeyOf(c),
                scope: c.inLibrary ? "library" : "tmdb",
                name: c.name,
                ...(c.year ? { meta: String(c.year) } : {}),
                inLibrary: c.inLibrary,
              }))}
            onSelect={(result) => {
              commit([...series, result.id]);
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
      ) : (
        <Button variant="outline" size="sm" className="self-start" onClick={() => setAdding(true)}>
          <Plus aria-hidden />
          Add a show…
        </Button>
      )}
    </div>
  );
};

export { ChannelSeriesScope };
