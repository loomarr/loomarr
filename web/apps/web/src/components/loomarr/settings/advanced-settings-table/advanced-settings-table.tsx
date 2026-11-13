import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { SettingEntryProvenance } from "@loomarr/api/models/settingEntryProvenance";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { Action, Stack, Text } from "@loomarr/design-system";
import { SearchField } from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
import { destinationForOwner } from "@/settings/settings-destinations";
import { SettingField } from "../setting-field";
import type { AdvancedSettingsTableProps } from "./advanced-settings-table.type";

// AdvancedSettingsTable — the raw registry escape hatch (config-design §5).
//
// A lookup surface that is ALSO editable. The lookup half is why keys are monospace and
// verbatim rather than humanized: someone arrives holding a literal `job.workers` from a
// compose file or a log line, and a row labelled "Job workers" does not match the string they
// are carrying.
//
// ⚠ The EDITING half is not a nicety — it is load-bearing. `GroupAdvanced` holds 19 keys (job
// schedules, TTLs, the reconcile interval) whose only editor is this table (V9: there is no
// separate "Advanced" page). A read-only table here — which is what the v2 mock draws — would
// leave all 19 uneditable, with nothing on screen saying so. Rows edit through the
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
// (`internal/platform/settings/secrets.go`) and are not settings entries at all — the API's provenance
// enum has exactly env/db/default. Rendering a fourth state would mean inventing a value the
// backend never sends. Those secrets live under Access and devices, where view/copy/
// regenerate belong.
const PROVENANCE: Record<
  string,
  { label: string; className: string; tone: "info" | "secondary" | "success" }
> = {
  [SettingEntryProvenance.env]: {
    label: "SET VIA ENV",
    // The only one that is not merely informational: an env-pinned key cannot be edited in the
    // UI at all (§3), so it reads as a lock rather than a note.
    className: "bg-tune-tint-15",
    tone: "info",
  },
  [SettingEntryProvenance.db]: { label: "DATABASE", className: "bg-lock-tint-15", tone: "success" },
  [SettingEntryProvenance.default]: { label: "DEFAULT", className: "bg-static-800", tone: "secondary" },
};

const AdvancedSettingsTable = ({
  entries,
  query,
  onQueryChange,
  values,
  onEdit,
  onEnvOverride,
  onClear,
  className,
}: AdvancedSettingsTableProps) => {
  const q = query.trim().toLowerCase();
  const rows = entries.filter((e) => matches(e, q));

  return (
    <Stack className={className} gap="control">
      <Stack align="center" direction="row" gap="control" wrap>
        <div className="relative min-w-64 flex-1">
          <SearchField
            label="Search settings"
            labelHidden
            onChangeText={onQueryChange}
            placeholder="Search by key, group, or value…"
            value={query}
          />
        </div>
        {/* The count is the feedback that a search DID something — without it, a query that
            matches nothing looks identical to a page that failed to load. */}
        <Text as="span" textRole="mono">
          {`${rows.length} of ${entries.length} shown`}
        </Text>
      </Stack>

      <div className="border-border overflow-x-auto rounded-lg border">
        <div className="min-w-[780px]">
          <div className="border-border grid grid-cols-[1.5fr_1.2fr_110px_128px_88px] gap-3 border-b px-4 py-2">
            {["Key", "Value", "Owner", "Provenance", "Action"].map((heading) => (
              <Text as="div" key={heading} textRole="label" tone="secondary">
                {heading}
              </Text>
            ))}
          </div>

          {rows.length === 0 && (
            <Text as="p" className="px-4 py-6 text-center" textRole="compact" tone="secondary">
              {`No setting matches “${query}”.`}
            </Text>
          )}

          {rows.map((entry) => {
            // A key this page owns has no other home: a link back to this page says nothing.
            const owner = destinationForOwner(entry.owner);
            const home = owner?.id === "advanced" ? undefined : owner;
            const prov = PROVENANCE[entry.provenance] ?? {
              label: entry.provenance.toUpperCase(),
              className: "bg-static-800",
              tone: "secondary" as const,
            };
            // llm.model is committed through the model-selection action, which also
            // verifies capability, persists provider metadata, and hot-swaps the client.
            // A generic text input here would bypass all three and accept Ollama tags
            // for OpenRouter, so keep the key searchable but route edits to its owner.
            const pickerOwned = entry.key === SettingKey.LLMModel;
            return (
              <div
                key={entry.key}
                className="border-border grid grid-cols-[1.5fr_1.2fr_110px_128px_88px] items-center gap-3 border-b px-4 py-2 last:border-b-0"
              >
                <Stack align="center" className="min-w-0" direction="row" gap="inline">
                  {/* The visible label for this row's control (see `labelledBy` below) — which is
                    why it carries an id. A table row already shows the name; a <label> beside
                    the input would duplicate it on screen for no one's benefit. */}
                  <Text
                    as="span"
                    className="truncate"
                    id={`${entry.key}-label`}
                    textRole="mono"
                    tone="primary"
                  >
                    {entry.key}
                  </Text>
                  {/* ADV marks a key that is hidden behind its home page's disclosure. It belongs
                    HERE especially: this is the surface someone reaches precisely because the
                    key was not where they looked, and "it's advanced" is the answer. */}
                  {entry.advanced && (
                    <span
                      title="advanced: also shown behind its home page's disclosure"
                      className="border-border shrink-0 rounded-sm border px-1"
                    >
                      <Text as="span" textRole="mono" tone="secondary">
                        ADV
                      </Text>
                    </span>
                  )}
                </Stack>
                {/* The control, not a rendering of the value. A secret still never shows its
                  value (§4) — SettingField's compact mode keeps the masked-preview flow. */}
                {pickerOwned ? (
                  <Text as="span" className="truncate" textRole="mono">
                    {(values[entry.key] ?? entry.value) || "Not selected"}
                  </Text>
                ) : (
                  <SettingField
                    entry={entry}
                    compact
                    labelledBy={`${entry.key}-label`}
                    value={values[entry.key] ?? entry.value ?? ""}
                    onChange={(v) => onEdit(entry.key, v)}
                    onEnvOverride={onEnvOverride ? (enabled) => onEnvOverride(entry.key, enabled) : undefined}
                  />
                )}
                {home ? (
                  <a
                    href={home.path}
                    className="text-muted-foreground hover:text-foreground truncate text-xs underline-offset-4 hover:underline"
                  >
                    {home.label}
                  </a>
                ) : (
                  // A key with no page but this one: say so in words, not the server's group id
                  // or a link to the page already open (#3411). The group stays searchable.
                  <Text as="span" className="truncate" textRole="hint" tone="secondary">
                    Only here
                  </Text>
                )}
                <span className={cn("w-fit rounded-sm px-1.5 py-0.5", prov.className)}>
                  <Text as="span" textRole="mono" tone={prov.tone}>
                    {prov.label}
                  </Text>
                </span>
                {pickerOwned ? (
                  <Link to="/settings/ai" className="text-signal w-fit text-xs underline underline-offset-4">
                    Choose model
                  </Link>
                ) : (
                  <Action
                    disabled={entry.provenance !== SettingEntryProvenance.db || !onClear}
                    layout="text"
                    onPress={() => onClear?.(entry)}
                    style={{ alignSelf: "flex-start" }}
                    variant="ghost"
                  >
                    Use default
                  </Action>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </Stack>
  );
};

export { AdvancedSettingsTable };
