import { type SettingEntry, SettingEntryProvenance } from "@loomarr/api";
import { Search } from "lucide-react";
import { Input } from "@/components/ui";
import { cn } from "@/lib";
import { SettingField } from "../setting-field";
import type { AllSettingsTableProps } from "./all-settings-table.type";

// AllSettingsTable — the escape hatch (config-design §5, V10).
//
// A lookup surface that is ALSO editable. The lookup half is why keys are monospace and
// verbatim rather than humanized: someone arrives holding a literal `job.workers` from a
// compose file or a log line, and a row labelled "Job workers" does not match the string they
// are carrying.
//
// ⚠ The EDITING half is not a nicety — it is load-bearing. `GroupAdvanced` holds 19 keys (job
// schedules, TTLs, the reconcile interval) whose only editor was the old "Advanced" page, and
// V9 folded that page into this one. A read-only table here — which is what the v2 mock draws —
// would have left all 19 uneditable, with nothing on screen saying so. Rows edit through the
// same `SettingField` the other pages use (in `compact` mode) and stage into the same cross-tab
// save bar, so env-pinned locking and the secret replace-flow behave identically.

// Search matches KEY + GROUP + VALUE (the phase gate). All three because each is a different
// way of already knowing what you want: the key when you read it in a config file, the group
// when you remember roughly where it lives, and the value when you are hunting for whichever
// setting is pointing at the wrong URL.
const matches = (entry: SettingEntry, q: string): boolean => {
  if (q === "") return true;
  const haystack = `${entry.key} ${entry.group} ${entry.value ?? ""}`.toLowerCase();
  return haystack.includes(q);
};

// Provenance chips. THREE, not the v2 mock's four: the mock adds a `generated` chip, but
// generated secrets (`playout_token`, the API token) are a separate registry
// (`internal/settings/secrets.go`) and are not settings entries at all — the API's provenance
// enum has exactly env/db/default. Rendering a fourth state would mean inventing a value the
// backend never sends. Those secrets have their own panel on Security, where view/copy/
// regenerate belong.
const PROVENANCE: Record<string, { label: string; className: string }> = {
  [SettingEntryProvenance.env]: {
    label: "SET VIA ENV",
    // The only one that is not merely informational: an env-pinned key cannot be edited in the
    // UI at all (§3), so it reads as a lock rather than a note.
    className: "bg-tune-tint-15 text-tune",
  },
  [SettingEntryProvenance.db]: { label: "DATABASE", className: "bg-lock-tint-15 text-lock" },
  [SettingEntryProvenance.default]: { label: "DEFAULT", className: "bg-static-800 text-static-400" },
};

const AllSettingsTable = ({
  entries,
  query,
  onQueryChange,
  values,
  onEdit,
  className,
}: AllSettingsTableProps) => {
  const q = query.trim().toLowerCase();
  const rows = entries.filter((e) => matches(e, q));

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-64 flex-1">
          <Search
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-static-400"
            aria-hidden
          />
          <Input
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Search by key, group, or value…"
            aria-label="Search settings"
            className="pl-8"
          />
        </div>
        {/* The count is the feedback that a search DID something — without it, a query that
            matches nothing looks identical to a page that failed to load. */}
        <span className="font-mono text-muted-foreground text-xs">
          {`${rows.length} of ${entries.length} shown`}
        </span>
      </div>

      <div className="overflow-hidden rounded-lg border border-border">
        <div className="grid grid-cols-[1.5fr_1.2fr_110px_128px] gap-3 border-border border-b px-4 py-2 font-mono text-2xs text-muted-foreground uppercase tracking-wide">
          <div>Key</div>
          <div>Value</div>
          <div>Group</div>
          <div>Provenance</div>
        </div>

        {rows.length === 0 && (
          <p className="px-4 py-6 text-center text-muted-foreground text-sm">
            {`No setting matches “${query}”.`}
          </p>
        )}

        {rows.map((entry) => {
          const prov = PROVENANCE[entry.provenance] ?? {
            label: entry.provenance.toUpperCase(),
            className: "bg-static-800 text-static-400",
          };
          return (
            <div
              key={entry.key}
              className="grid grid-cols-[1.5fr_1.2fr_110px_128px] items-center gap-3 border-border border-b px-4 py-2 last:border-b-0"
            >
              <div className="flex min-w-0 items-center gap-2">
                {/* The visible label for this row's control (see `labelledBy` below) — which is
                    why it carries an id. A table row already shows the name; a <label> beside
                    the input would duplicate it on screen for no one's benefit. */}
                <span id={`${entry.key}-label`} className="truncate font-mono text-xs">
                  {entry.key}
                </span>
                {/* ADV marks a key that is hidden behind its home page's disclosure. It belongs
                    HERE especially: this is the surface someone reaches precisely because the
                    key was not where they looked, and "it's advanced" is the answer. */}
                {entry.advanced && (
                  <span
                    title="advanced: also shown behind its home page's disclosure"
                    className="shrink-0 rounded-sm border border-border px-1 font-mono text-2xs text-muted-foreground"
                  >
                    ADV
                  </span>
                )}
              </div>
              {/* The control, not a rendering of the value. A secret still never shows its
                  value (§4) — SettingField's compact mode keeps the masked-preview flow. */}
              <SettingField
                entry={entry}
                compact
                labelledBy={`${entry.key}-label`}
                value={values[entry.key] ?? entry.value ?? ""}
                onChange={(v) => onEdit(entry.key, v)}
              />
              <span className="truncate text-muted-foreground text-xs">{entry.group}</span>
              <span
                className={cn(
                  "w-fit rounded-sm px-1.5 py-0.5 font-mono text-2xs uppercase tracking-wide",
                  prov.className,
                )}
              >
                {prov.label}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
};

export { AllSettingsTable };
