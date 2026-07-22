import { useState } from "react";
import { SettingField } from "@/components/loomarr/setting-field";
import { cn } from "@/lib";
import type { SettingsFieldsProps } from "./settings-fields.type";

// The fields of one settings group, and nothing else — no <form>, no save button.
//
// It exists because the save UNIT differs by context (config-design §5/§6): the wizard
// saves one group per step, whereas a Settings page has several blocks (Connections alone
// has media server, requester, Tunarr and TMDB) behind ONE sticky save bar, because
// connection settings change together and half-saved pairs mid-test are a footgun. A
// component that owned its own form could not serve both, so ownership moved out.
//
// Advanced keys stay behind a per-group toggle (§5) — that is a property of the group's
// presentation, so it does belong here.
// Kinds that need the FULL row width in the 2-col grid: secrets (the masked value + Replace
// button), urls (long addresses), and string_list. The compact controls (bool/int/enum) sit
// two-per-row.
const spansFullWidth = (kind: string, secret: boolean): boolean =>
  secret || kind === "url" || kind === "string_list";

const SettingsFields = ({ entries, values, onChange, results, className }: SettingsFieldsProps) => {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const advancedCount = entries.filter((e) => e.advanced).length;
  const resultFor = (key: string) => results?.find((r) => r.key === key);

  return (
    // A responsive 2-col field grid (was a single-column stack): compact controls pair up,
    // wide ones (secrets/urls/lists) span both columns. gap-x for columns, gap-y for rows.
    <div className={cn("grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2", className)}>
      {entries.map((entry) =>
        entry.advanced && !showAdvanced ? null : (
          <SettingField
            key={entry.key}
            entry={entry}
            value={values[entry.key] ?? entry.value ?? ""}
            onChange={(v) => onChange(entry.key, v)}
            result={resultFor(entry.key)}
            className={spansFullWidth(entry.kind, entry.secret) ? "sm:col-span-2" : undefined}
          />
        ),
      )}

      {advancedCount > 0 && (
        <button
          type="button"
          onClick={() => setShowAdvanced((v) => !v)}
          className="w-fit cursor-pointer text-muted-foreground text-sm underline-offset-4 hover:text-foreground hover:underline sm:col-span-2"
        >
          {showAdvanced ? "Hide advanced" : `Show advanced (${advancedCount})`}
        </button>
      )}
    </div>
  );
};

export { SettingsFields };
