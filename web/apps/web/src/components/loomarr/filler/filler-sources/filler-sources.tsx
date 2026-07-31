import { formatRelative } from "@loomarr/core";
import { FolderOpen, Globe, Library } from "lucide-react";
import { Badge, Button, Caption } from "@/components/ui";
import { cn } from "@/lib";
import type { FillerSourcesProps } from "./filler-sources.type";

// FillerSources — where the clip catalog comes from (§10, V28; the v2 mock's Filler → Sources).
//
// Reads a READ-MODEL, not a table: the server derives these rows from `filler.dir` plus the
// library connection and counts each row's clips from the catalog. That is why there is no
// add/remove affordance here — there is nothing to add a row TO. A source is configured by
// changing the setting it comes from, and V33 introduces the persisted registry when a remote
// source finally has state a setting cannot hold.
//
// ⚠ An UNCONFIGURED source renders as a row, not as an absence. "No drop-folder configured" is
// the answer to "why is my catalog empty", and hiding the row leaves that question unanswered —
// the operator sees two sources, concludes filler is working, and never finds the third.

const ICONS: Record<string, typeof FolderOpen> = {
  folder: FolderOpen,
  library: Library,
  remote: Globe,
};

const FillerSources = ({ sources, total, onFetch, fetching, error, className }: FillerSourcesProps) => (
  <div className={cn("flex flex-col gap-3", className)}>
    <p className="text-muted-foreground text-sm">
      {`${sources.length} ${sources.length === 1 ? "source" : "sources"} · ${total} ${
        total === 1 ? "clip" : "clips"
      }`}
    </p>

    {error && <p className="text-onair-300 text-sm">{error}</p>}

    <ul className="flex flex-col gap-2">
      {sources.map((s) => {
        const Icon = ICONS[s.kind] ?? Globe;
        return (
          <li
            key={s.kind}
            // ⚠ NOT dimmed with opacity. Fading an unconfigured row is the obvious way to say
            // "inactive", and it composites text-muted-foreground toward the background:
            // measured 2.95:1 against the required 4.5:1, i.e. an axe color-contrast failure on
            // the one row whose whole job is to explain why the catalog is empty. The `not
            // configured` badge already carries that meaning, legibly.
            className="flex flex-wrap items-center gap-3 rounded-lg border border-border px-4 py-3"
          >
            <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="truncate font-mono text-sm">{s.target}</span>
                {!s.configured && <Badge variant="caution">not configured</Badge>}
              </div>
              <p className="mt-0.5 text-muted-foreground text-xs">{s.detail}</p>
            </div>
            {/* The count is the honest signal that a source is doing anything. A configured
                source contributing 0 clips is a real and reportable state — usually an empty
                folder or a mount that did not come up. */}
            <span className="shrink-0 font-mono text-muted-foreground text-xs">
              {`${s.count} ${s.count === 1 ? "clip" : "clips"}`}
            </span>
            {s.fetchable && (
              <Button
                variant="ghost"
                onClick={() => onFetch(s.kind)}
                disabled={fetching != null}
                aria-label={`Fetch now from ${s.target}`}
              >
                {fetching === s.kind ? "Fetching…" : "Fetch now"}
              </Button>
            )}

            {/* The specific remotes an operator added, NESTED under this row rather than
                promoted to peers (V33, build plan §6.1). The rows above describe
                CONFIGURATION — including "you could set this up but have not" — and a flat
                list of things that EXIST cannot express that; merging the two models would
                show the drop-folder twice. */}
            {s.remotes && s.remotes.length > 0 && (
              <ul className="mt-1 flex w-full flex-col gap-1 border-border border-t pt-2 pl-7">
                {s.remotes.map((r) => (
                  <li key={r.id} className="flex flex-wrap items-baseline gap-2">
                    <span className="min-w-0 flex-1 truncate text-sm">{r.label}</span>
                    <Caption className="shrink-0">
                      {/* "never" rather than an epoch date nobody meant. */}
                      {r.lastFetchedAt ? `fetched ${formatRelative(r.lastFetchedAt)}` : "never fetched"}
                    </Caption>
                  </li>
                ))}
              </ul>
            )}
          </li>
        );
      })}
    </ul>
  </div>
);

export { FillerSources };
