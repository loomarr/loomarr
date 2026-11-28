import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { SettingKind } from "@loomarr/api/models/settingKind";
import { SettingPresentation } from "@loomarr/api/models/settingPresentation";
import { Action, Grid, GridItem } from "@loomarr/design-system";
import { useState } from "react";
import { SettingField } from "../setting-field";
import type { SettingsFieldsProps } from "./settings-fields.type";

// The fields of one settings group, and nothing else — no <form>, no save button.
//
// It exists because the save UNIT differs by context (config-design §5/§6): the wizard
// saves one group per step, whereas a Settings page has several blocks (Connections alone
// has media server, requester, Tunarr and TMDB) behind ONE sticky save bar, because
// connection settings change together and half-saved pairs mid-test are a footgun. A
// component that owned its own form could not serve both, so the caller owns the form.
//
// Advanced keys stay behind a per-group toggle (§5) — that is a property of the group's
// presentation, so it does belong here.
// Kinds that need the FULL row width in the 2-col grid: secrets (the masked value + Replace
// button), urls (long addresses), and string_list. The compact controls (bool/int/enum) sit
// two-per-row.
const spansFullWidth = (kind: SettingKind, secret: boolean, presentation?: SettingPresentation): boolean =>
  secret ||
  kind === SettingKind.url ||
  kind === SettingKind.string_list ||
  presentation === SettingPresentation.switch;

const SettingsFields = ({
  entries,
  values,
  onChange,
  results,
  onEnvOverride,
  disabledReasons,
  descriptionIds,
  initialAdvanced = false,
}: SettingsFieldsProps) => {
  const [showAdvanced, setShowAdvanced] = useState(initialAdvanced);
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
    <Grid columnGap="4" columns={{ base: 1, sm: 2 }} rowGap="5">
      {visible.map((entry) => {
        if (entry.advanced && !showAdvanced) return null;
        const field = (
          <SettingField
            key={entry.key}
            entry={entry}
            value={values[entry.key] ?? entry.value ?? ""}
            onChange={(v) => onChange(entry.key, v)}
            result={resultFor(entry.key)}
            disabledReason={disabledReasons?.[entry.key]}
            descriptionId={descriptionIds?.[entry.key]}
            onEnvOverride={onEnvOverride ? (enabled) => onEnvOverride(entry.key, enabled) : undefined}
          />
        );
        return spansFullWidth(entry.kind, entry.secret, entry.presentation) ? (
          <GridItem key={entry.key} span={{ base: 1, sm: "full" }}>
            {field}
          </GridItem>
        ) : (
          field
        );
      })}

      {advancedCount > 0 && (
        <GridItem span={{ base: 1, sm: "full" }}>
          <Action
            layout="text"
            onPress={() => setShowAdvanced((v) => !v)}
            alignSelf="flex-start"
            variant="ghost"
          >
            {showAdvanced ? "Hide advanced" : `Show advanced (${advancedCount})`}
          </Action>
        </GridItem>
      )}
    </Grid>
  );
};

export { SettingsFields };
