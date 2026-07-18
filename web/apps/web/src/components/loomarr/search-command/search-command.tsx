import { Search } from "lucide-react";
import { Fragment } from "react";
import { Badge } from "@/components/ui";
import { cn } from "@/lib";
import type { SearchCommandProps, SearchScope } from "./search-command.type";

// SearchCommand — the ⌘K palette (§3, §7.2) over library/TMDB/clips + channels + help.
// In-library results wear a `lock` flag so you see at a glance what's ready now vs.
// what would be an acquisition (§8). Presentational + controlled; the global ⌘K
// trigger and cmdk keyboard/listbox semantics wrap this in 13.4 (dependency-free here).
const SCOPE_LABEL: Record<SearchScope, string> = {
  channels: "Channels",
  library: "In your library",
  tmdb: "Discover (TMDB)",
  clips: "Filler clips",
  help: "Help",
};

const SCOPE_ORDER: SearchScope[] = ["channels", "library", "tmdb", "clips", "help"];

const SearchCommand = ({
  query,
  onQueryChange,
  results,
  onSelect,
  loading = false,
  className,
}: SearchCommandProps) => {
  const groups = SCOPE_ORDER.map((scope) => ({
    scope,
    items: results.filter((r) => r.scope === scope),
  })).filter((g) => g.items.length > 0);
  const empty = !loading && results.length === 0;
  return (
    <div
      className={cn(
        "flex w-full flex-col overflow-hidden rounded-lg border border-border bg-popover shadow-lg",
        className,
      )}
    >
      <div className="flex items-center gap-2 border-border border-b px-3">
        <Search className="size-4 shrink-0 text-static-400" aria-hidden />
        <input
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          placeholder="Search titles, channels, help…"
          aria-label="Search"
          className="h-11 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
        <kbd className="shrink-0 rounded border border-border bg-static-800 px-1.5 py-0.5 font-mono text-[10px] text-static-400">
          ESC
        </kbd>
      </div>

      <div className="max-h-80 overflow-y-auto p-1.5">
        {empty && (
          <p className="px-3 py-6 text-center text-muted-foreground text-sm">
            {query ? `No matches for “${query}”.` : "Type to search your library, TMDB, and channels."}
          </p>
        )}
        {groups.map((group) => (
          <Fragment key={group.scope}>
            <p className="px-2 pt-2 pb-1 font-mono text-[11px] text-static-400 uppercase tracking-wide">
              {SCOPE_LABEL[group.scope]}
            </p>
            {group.items.map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => onSelect?.(item)}
                className="flex w-full items-center justify-between gap-3 rounded-md px-2 py-2 text-left text-sm transition-colors hover:bg-static-800"
              >
                <span className="min-w-0 truncate">{item.name}</span>
                <span className="flex shrink-0 items-center gap-2">
                  {item.meta && <span className="font-mono text-static-400 text-xs">{item.meta}</span>}
                  {item.inLibrary && <Badge variant="lock">In library</Badge>}
                </span>
              </button>
            ))}
          </Fragment>
        ))}
      </div>
    </div>
  );
};

export { SearchCommand };
