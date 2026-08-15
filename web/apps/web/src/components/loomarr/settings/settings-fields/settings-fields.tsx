import type { SettingEntry } from "@loomarr/api";
import { useState } from "react";
import { cn } from "@/lib";
import { SettingField } from "../setting-field";
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

const SettingsFields = ({
  entries,
  values,
  onChange,
  results,
  onEnvOverride,
  disabledReasons,
  className,
}: SettingsFieldsProps) => {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const resultFor = (key: string) => results?.find((r) => r.key === key);

  // The live value of a key, honoring an unsaved edit (values) over the resolved value —
  // so a conditional field reacts the instant you change its controller, before saving.
  const liveValue = (key: string): string => {
    const edited = values[key];
    if (edited !== undefined) return edited;
    return entries.find((e) => e.key === key)?.value ?? "";
  };

  // A field with `showWhen` is shown only when EVERY named controller currently holds one of
  // its listed values (config-design §5). No showWhen ⇒ always visible. This is generic — it
  // reads whatever the registry names, so it works for the AI provider case and any future one.
  const isVisible = (entry: SettingEntry): boolean => {
    const cond = entry.showWhen;
    if (!cond) return true;
    return Object.entries(cond).every(([ctrlKey, allowed]) => allowed?.includes(liveValue(ctrlKey)) ?? true);
  };

  const visible = entries.filter(isVisible);
  const advancedCount = visible.filter((e) => e.advanced).length;

  return (
    // A responsive 2-col field grid (was a single-column stack): compact controls pair up,
    // wide ones (secrets/urls/lists) span both columns. gap-x for columns, gap-y for rows.
    <div className={cn("grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2", className)}>
      {visible.map((entry) =>
        entry.advanced && !showAdvanced ? null : (
          <SettingField
            key={entry.key}
            entry={entry}
            value={values[entry.key] ?? entry.value ?? ""}
            onChange={(v) => onChange(entry.key, v)}
            result={resultFor(entry.key)}
            disabledReason={disabledReasons?.[entry.key]}
            onEnvOverride={onEnvOverride ? (enabled) => onEnvOverride(entry.key, enabled) : undefined}
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
