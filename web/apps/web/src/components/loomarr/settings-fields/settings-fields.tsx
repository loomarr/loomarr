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
const SettingsFields = ({ entries, values, onChange, results, className }: SettingsFieldsProps) => {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const advancedCount = entries.filter((e) => e.advanced).length;
  const resultFor = (key: string) => results?.find((r) => r.key === key);

  return (
    <div className={cn("flex flex-col gap-5", className)}>
      {entries.map((entry) =>
        entry.advanced && !showAdvanced ? null : (
          <SettingField
            key={entry.key}
            entry={entry}
            value={values[entry.key] ?? entry.value ?? ""}
            onChange={(v) => onChange(entry.key, v)}
            result={resultFor(entry.key)}
          />
        ),
      )}

      {advancedCount > 0 && (
        <button
          type="button"
          onClick={() => setShowAdvanced((v) => !v)}
          className="w-fit text-muted-foreground text-sm underline-offset-4 hover:text-foreground hover:underline"
        >
          {showAdvanced ? "Hide advanced" : `Show advanced (${advancedCount})`}
        </button>
      )}
    </div>
  );
};

export { SettingsFields };
