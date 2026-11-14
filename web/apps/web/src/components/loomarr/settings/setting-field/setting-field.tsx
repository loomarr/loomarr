import { SettingEntryProvenance } from "@loomarr/api/models/settingEntryProvenance";
import { SettingKind } from "@loomarr/api/models/settingKind";
import { SettingPresentation } from "@loomarr/api/models/settingPresentation";
import type { SettingPreset } from "@loomarr/api/models/settingPreset";
import { SettingResultStatus } from "@loomarr/api/models/settingResultStatus";
import { SettingUnit } from "@loomarr/api/models/settingUnit";
import { formatAgo, humanizeSettingKey } from "@loomarr/core/format";
import {
  Action,
  CompactSelect,
  Field,
  FieldHint,
  Icon,
  icons,
  Stack,
  StatusLabel,
  Surface,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { NumberField } from "@loomarr/ui";
import { type ComponentProps, type ReactNode, useState } from "react";
import { languageName } from "@/lib/languages";
import { cn } from "@/lib/utils";
import { FieldHelp } from "../../feedback";
import { CronField } from "./cron-field";
import { DurationField, type DurationUnit } from "./duration-field";
import type { SettingFieldProps } from "./setting-field.type";

// SettingField — one registry key as a form control (config-design §2, §3, §6). The
// same field powers the wizard and Settings; there is no parallel wizard form system.
// Everything it renders is contract data: `kind` picks the control, `enum` fills the
// options, `doc` is the help text, `provenance: env` LOCKS the field with a "set via
// environment" chip (env > database > default, visible provenance), and a secret shows
// its masked tail with replace-only editing (§4 — a stored secret is never echoed).
const keyboardFor = (kind: SettingKind): ComponentProps<typeof Field>["keyboardType"] => {
  if (kind === SettingKind.int) return "number-pad";
  if (kind === SettingKind.url) return "url";
  return "default";
};

// A Go duration string ("6h0m0s", "90s", "1h30m") as seconds; anything unreadable is zero.
const durationSeconds = (raw: string): number => {
  const factors: Record<string, number> = { h: 3600, m: 60, s: 1, ms: 0.001 };
  let seconds = 0;
  for (const match of raw.matchAll(/(-?\d+(?:\.\d+)?)(ms|h|m|s)/g)) {
    const factor = factors[match[2] ?? ""];
    if (factor !== undefined) seconds += Number(match[1]) * factor;
  }
  return Number.isFinite(seconds) ? seconds : 0;
};

const durationRaw = (seconds: number): string => `${Number(seconds.toFixed(3))}s`;

// A whole number of time, per its registry-declared unit: what one stored step is worth in seconds
// and the units it can be edited in. Only this unit and larger ones, so every edit stays whole.
const TIME_UNITS: Partial<Record<SettingUnit, { seconds: number; units: readonly DurationUnit[] }>> = {
  [SettingUnit.seconds]: { seconds: 1, units: ["days", "hours", "minutes", "seconds"] },
  [SettingUnit.hours]: { seconds: 3600, units: ["days", "hours"] },
};

// The Custom choice offered after the registry's presets. It cannot collide with a preset: preset
// values are canonical setting values, and no setting accepts this word as one.
const CUSTOM_PRESET = "custom";

const byteParts = (raw: string): { amount: number; unit: "MiB" } => {
  const bytes = Number(raw);
  if (!Number.isFinite(bytes)) return { amount: 0, unit: "MiB" };
  return { amount: Number((bytes / 1024 ** 2).toFixed(2)), unit: "MiB" };
};

// A number box with its unit beside it: the bytes editor's MiB, or a registry-declared unit.
const WithUnit = ({ unit, children }: { unit: string; children: ReactNode }) => (
  <Stack direction="row" gap="inline">
    {children}
    <Stack
      align="center"
      className="border-input h-10 w-16 rounded-md border"
      flexShrink={0}
      justify="center"
      render="span"
    >
      <Text as="span" textRole="compact" tone="secondary">
        {unit}
      </Text>
    </Stack>
  </Stack>
);

const SettingField = ({
  entry,
  value,
  onChange,
  result,
  compact,
  labelledBy,
  onEnvOverride,
  disabledReason,
  descriptionId,
  className,
}: SettingFieldProps) => {
  const [replacing, setReplacing] = useState(false);
  const [editingCustomSchedule, setEditingCustomSchedule] = useState(false);
  const id = `setting-${entry.key}`;
  const label = entry.label || humanizeSettingKey(entry.key);
  // `pinned` is the LOCK STATE, which is not the same question as "is this env's key".
  // An unlocked key (§3.1) resolves as `db` while its variable is still set, so it is
  // editable — but still overriding, which the badge below has to say.
  const pinned = entry.provenance === SettingEntryProvenance.env;
  // ⚠ A pinned VALUE is real, so it is shown read-only at full strength, never in the dimmed
  // disabled look that reads as an empty placeholder (#2759): every control takes `readOnly`.
  const overriding = entry.envOverride === true;
  // Offer the affordance only where it can actually do something: the environment must set
  // the key, and the surface must have supplied a handler.
  const canUnlock = onEnvOverride !== undefined && entry.envPinnable === true;
  const invalid = result?.status === SettingResultStatus.invalid;
  const inlineSwitch = entry.kind === SettingKind.bool && entry.presentation === SettingPresentation.switch;
  const problem = invalid ? result?.problem : undefined;
  // Compact mode renders none of the explanatory elements, so pointing at one would be a dangling
  // reference. Full fields associate the ordinary help, any unavailable reason, and the server's
  // validation problem: `role="alert"` announces that once as it appears, and the description is
  // what a screen reader repeats when the operator comes back to the field to fix it.
  const describedBy = compact
    ? undefined
    : [
        entry.doc ? `${id}-doc` : undefined,
        disabledReason ? `${id}-unavailable` : undefined,
        problem ? `${id}-problem` : undefined,
        descriptionId,
      ]
        .filter(Boolean)
        .join(" ") || undefined;

  // A stored secret renders as its masked tail until the operator opts into replacing it.
  const secretLocked = entry.secret && entry.set && !replacing;

  const control = () => {
    if (entry.kind === SettingKind.bool) {
      if (entry.presentation === SettingPresentation.switch) {
        return (
          <Toggle
            kind="switch"
            id={id}
            label={label}
            labelHidden
            checked={value === "true"}
            disabled={pinned || disabledReason !== undefined}
            aria-describedby={describedBy}
            onCheckedChange={(checked) => onChange(String(checked))}
          />
        );
      }
      return (
        <Toggle
          id={id}
          label={label}
          labelHidden
          checked={value === "true"}
          disabled={pinned || disabledReason !== undefined}
          aria-describedby={describedBy}
          onCheckedChange={(checked) => onChange(String(checked))}
        />
      );
    }
    if (entry.kind === SettingKind.enum) {
      return (
        <CompactSelect
          aria-describedby={describedBy}
          aria-labelledby={labelledBy}
          disabled={disabledReason !== undefined}
          id={id}
          invalid={invalid}
          label={label}
          onValueChange={onChange}
          // Prefer the registry-owned {value,label} options (config-design §5) so the dropdown
          // shows "OpenAI"/"Emby", not the raw stored value. Fall back to the bare value list for
          // any enum that ships no labels.
          options={entry.enumOptions ?? (entry.enum ?? []).map((v) => ({ value: v, label: v }))}
          readOnly={pinned}
          value={value}
        />
      );
    }
    if (entry.presentation === SettingPresentation.filler_download_schedule) {
      // The registry publishes presets in canonical form, so a resolved value matches by string.
      const presets = entry.presets ?? [];
      const matched = presets.find((option) => option.value === value);
      const preset = editingCustomSchedule || !matched ? CUSTOM_PRESET : matched.value;
      return (
        <Stack gap="inline">
          <CompactSelect
            aria-describedby={describedBy}
            aria-labelledby={labelledBy}
            disabled={disabledReason !== undefined}
            id={id}
            invalid={invalid}
            label={label}
            onValueChange={(next) => {
              if (next === CUSTOM_PRESET) {
                setEditingCustomSchedule(true);
                return;
              }
              setEditingCustomSchedule(false);
              onChange(next);
            }}
            options={[...presets, { label: "Custom", value: CUSTOM_PRESET }]}
            readOnly={pinned}
            value={preset}
          />
          {preset === CUSTOM_PRESET && (
            <DurationField
              amountLabel="Custom check interval"
              disabled={pinned || disabledReason !== undefined}
              label="Custom check interval"
              onSeconds={(seconds) => onChange(durationRaw(seconds))}
              seconds={durationSeconds(value)}
              units={["days", "hours", "minutes"]}
            />
          )}
        </Stack>
      );
    }
    if (entry.kind === SettingKind.cron) {
      return (
        <CronField
          id={id}
          label={label}
          value={value}
          presets={entry.presets ?? []}
          onChange={onChange}
          disabled={disabledReason !== undefined}
          readOnly={pinned}
          invalid={invalid}
          describedBy={describedBy}
          labelledBy={labelledBy}
        />
      );
    }
    if (entry.kind === SettingKind.duration) {
      return (
        <DurationField
          describedBy={describedBy}
          disabled={pinned || disabledReason !== undefined}
          id={id}
          invalid={invalid}
          label={label}
          labelledBy={labelledBy}
          onSeconds={(seconds) => onChange(durationRaw(seconds))}
          seconds={durationSeconds(value)}
          units={["days", "hours", "minutes", "seconds"]}
        />
      );
    }
    const timeUnit = entry.kind === SettingKind.int && entry.unit ? TIME_UNITS[entry.unit] : undefined;
    if (timeUnit) {
      const stored = Number(value);
      return (
        <DurationField
          describedBy={describedBy}
          disabled={pinned || disabledReason !== undefined}
          id={id}
          invalid={invalid}
          label={label}
          labelledBy={labelledBy}
          onSeconds={(seconds) => onChange(String(Math.round(seconds / timeUnit.seconds)))}
          seconds={(Number.isFinite(stored) ? stored : 0) * timeUnit.seconds}
          units={timeUnit.units}
        />
      );
    }
    if (
      entry.kind === SettingKind.string &&
      (entry.presentation === SettingPresentation.timezone ||
        entry.presentation === SettingPresentation.language)
    ) {
      // The server's choices, never free text. A pinned or stored value outside them (a legacy zone
      // name, a rare language) is still shown as chosen rather than silently swapped for another.
      const presets = entry.presets ?? [];
      const choices = presets.some((option) => option.value === value)
        ? presets
        : [...presets, { value, label: value }];
      // Language names are interface copy, localized from CLDR; the registry's labels are codes.
      const optionLabel = (option: SettingPreset) =>
        entry.presentation === SettingPresentation.language && option.value
          ? languageName(option.value)
          : option.label;
      return (
        <CompactSelect
          aria-describedby={describedBy}
          aria-labelledby={labelledBy}
          disabled={pinned || disabledReason !== undefined}
          id={id}
          invalid={invalid}
          label={label}
          onValueChange={onChange}
          options={choices.map((option) => ({ value: option.value, label: optionLabel(option) }))}
          value={value}
        />
      );
    }
    if (entry.presentation === SettingPresentation.bytes) {
      const current = byteParts(value);
      return (
        <WithUnit unit={current.unit}>
          <NumberField
            amount={current.amount}
            aria-describedby={describedBy}
            aria-labelledby={labelledBy}
            disabled={disabledReason !== undefined}
            id={id}
            invalid={invalid}
            onAmount={(amount) => onChange(String(Math.round(amount * 1024 ** 2)))}
            readOnly={pinned}
          />
        </WithUnit>
      );
    }
    const input = (
      <Surface backgroundColor="transparent" borderWidth={0} flexGrow={1} flexShrink={1} minWidth={0}>
        <Field
          aria-describedby={describedBy}
          aria-labelledby={labelledBy}
          autoCapitalize="none"
          autoComplete={entry.secret ? "new-password" : "off"}
          autoCorrect={false}
          disabled={disabledReason !== undefined}
          id={id}
          invalid={invalid}
          keyboardType={keyboardFor(entry.kind)}
          onChangeText={onChange}
          placeholder={entry.secret && entry.set ? "Enter a new value to replace" : undefined}
          readOnly={pinned}
          secureTextEntry={entry.secret}
          value={value}
        />
      </Surface>
    );
    // The registry declares what a plain number counts; "50" alone does not say GiB.
    return entry.unit ? <WithUnit unit={entry.unit}>{input}</WithUnit> : input;
  };

  // Compact: the control alone. The secret replace-flow is kept — a stored secret must never
  // render its value, in a table cell least of all.
  if (compact) {
    return (
      <div className={cn("min-w-0", className)}>
        {secretLocked ? (
          <Text as="span" textRole="mono">
            {entry.preview ?? "stored"}
          </Text>
        ) : (
          control()
        )}
      </div>
    );
  }

  return (
    // `group` so the audit line can reveal on hover/focus of the whole field (below).
    <div
      className={cn(
        // `relative` anchors the absolutely positioned `sr-only` doc below to this field.
        "group relative",
        inlineSwitch
          ? "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1.5"
          : "flex flex-col gap-1.5",
        className,
      )}
    >
      <Stack align="center" direction="row" gap="inline">
        <Text as="label" htmlFor={id} textRole="label">
          {label}
        </Text>
        {/* The registry's one plain sentence sits under the field; the rest of the help (trade-offs,
            units, edge cases) waits behind this (i), so the form reads at a glance (#2741). */}
        {entry.detail && <FieldHelp label={label}>{entry.detail}</FieldHelp>}
        {/* THE LOCK IS THE CONTROL (config-design §3.1). The chip already says why the field
            is read-only, so hanging a separate "unlock" button beside it would put the
            explanation and the way out in two places — the operator reads "set via
            environment", and the thing to click is somewhere else. Clicking the lock opens
            it; clicking the open lock hands the key back.

            Still a plain label when no handler is supplied: a read-only surface keeps the
            pre-3.1 state exactly, rather than rendering a button that does nothing. */}
        {pinned &&
          (canUnlock ? (
            // The padlock is the DS's compact ghost icon action; the state beside it is the quiet
            // dot and word (#2784). The padlock's name carries the action, or a screen-reader user
            // hears "set via environment" and has to guess. The hover tell (the label warming to
            // `signal`, the padlock springing open) is not carried: the DS draws a pressed icon
            // action with its one focus outline and no colour swap.
            <Stack align="center" direction="row" gap="1.5">
              <Action compact icon="locked" layout="icon" onPress={() => onEnvOverride(true)} variant="ghost">
                {`Unlock ${label} to edit it here, currently set by ${entry.envVar ?? "the environment"}`}
              </Action>
              <StatusLabel tone="off">Set via environment</StatusLabel>
            </Stack>
          ) : (
            <StatusLabel tone="off">Set via environment</StatusLabel>
          ))}
        {/* Overriding is a DIFFERENT state from both locked and ordinary-db, and it names the
            variable: "set via environment" on an editable field would contradict itself, and a
            bare `db` chip would imply the environment never mentioned this key. */}
        {overriding &&
          (canUnlock ? (
            // The name starts with the visible word (WCAG 2.5.3), then says what the press does.
            <StatusLabel
              aria-label={`Overriding ${entry.envVar ?? "the environment"}: hand ${label} back, your saved value is kept`}
              onPress={() => onEnvOverride(false)}
              tone="warn"
            >
              Overriding {entry.envVar ?? "the environment"}
            </StatusLabel>
          ) : (
            <StatusLabel tone="warn">Overriding {entry.envVar ?? "the environment"}</StatusLabel>
          ))}
        {result?.status === SettingResultStatus.pinned && !pinned && (
          <StatusLabel tone="warn">Not saved: pinned</StatusLabel>
        )}
      </Stack>

      {secretLocked ? (
        <Stack align="center" direction="row" gap="inline">
          <Stack
            align="center"
            className="border-input text-muted-foreground h-9 rounded-md border px-3 font-mono text-sm"
            direction="row"
            flexBasis="0%"
            flexGrow={1}
            render="span"
          >
            {entry.preview ?? "stored"}
          </Stack>
          <Action
            onPress={() => {
              setReplacing(true);
              onChange("");
            }}
            variant="outline"
          >
            Replace
          </Action>
        </Stack>
      ) : (
        control()
      )}

      {/* The summary: one sentence, always visible, and the control's description. */}
      {entry.doc && (
        <FieldHint id={`${id}-doc`} className={cn(inlineSwitch && "col-span-2")}>
          {entry.doc}
        </FieldHint>
      )}

      {entry.caution && (
        <FieldHint tone="error" className={cn(inlineSwitch && "col-span-2")}>
          <Stack align="center" direction="row" gap="1" render="span">
            <Icon decorative glyph={icons.alert} size="compact" tone="inherit" />
            The stored value was invalid and has been reset to the default.
          </Stack>
        </FieldHint>
      )}

      {disabledReason && (
        <FieldHint
          id={`${id}-unavailable`}
          role="alert"
          tone="error"
          className={cn(inlineSwitch && "col-span-2")}
        >
          <Stack align="start" direction="row" gap="1" render="span">
            <Stack className="mt-0.5" direction="row" render="span">
              <Icon decorative glyph={icons.alert} size="compact" tone="inherit" />
            </Stack>
            {disabledReason}
          </Stack>
        </FieldHint>
      )}

      {/* "changed by … · when" (§5 field anatomy). Audit provenance is signal when you're
          auditing, noise at rest — so it's revealed on hover/focus of the field rather than
          costing a permanent line under every control (config-design §5). It stays in the DOM
          (opacity, not conditional render) so it's reachable by keyboard (focus-within) and
          screen readers; the transition is frozen under reduced-motion. Only for a value a
          PERSON set: an env pin or built-in default has no author to name. */}
      {entry.updatedAt && (
        <Text
          as="p"
          className={cn(
            "pointer-events-none opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100 motion-reduce:transition-none",
            inlineSwitch && "col-span-2",
          )}
          textRole="hint"
          tone="secondary"
        >
          {/* The server sends a name (never a raw id) and marks the caller's own change (#2736). */}
          {entry.updatedByYou
            ? `Changed by you, ${formatAgo(entry.updatedAt)}`
            : entry.updatedBy
              ? `Changed by ${entry.updatedBy}, ${formatAgo(entry.updatedAt)}`
              : `Changed ${formatAgo(entry.updatedAt)}`}
        </Text>
      )}

      {problem && (
        <FieldHint
          id={`${id}-problem`}
          role="alert"
          tone="error"
          className={cn(inlineSwitch && "col-span-2")}
        >
          {problem}
        </FieldHint>
      )}
    </div>
  );
};

export { SettingField };
